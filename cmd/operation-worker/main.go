package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/operation-worker/internal/config"
	"github.com/F31/hnb/cmd/operation-worker/internal/driver"
	"github.com/F31/hnb/cmd/operation-worker/internal/nats"
	corestore "github.com/F31/hnb/pkg/core/store"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/messaging"

	"github.com/F31/hnb/pkg/core"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	providers := make(map[string]driver.ProviderConfig, len(cfg.RuntimeProviders))
	for providerID, provider := range cfg.RuntimeProviders {
		providers[providerID] = driver.ProviderConfig{
			Endpoint:         provider.Endpoint,
			Audience:         provider.Audience,
			TokenSource:      iam.FileTokenSource{Path: provider.TokenFile},
			ProtocolVersion:  provider.ProtocolVersion,
			ProviderVersion:  provider.ProviderVersion,
			ProviderDigest:   provider.ProviderDigest,
			RequiredProvider: provider.RequiredProvider,
		}
	}
	runner, err := driver.NewHTTPRunner(providers, nil)
	if err != nil {
		log.Fatalf("runtime providers: %v", err)
	}
	w, err := nats.NewWorker(db, nc, cfg.LeaseDuration, runner)
	if err != nil {
		log.Fatalf("worker: %v", err)
	}
	relay, err := nats.NewOutboxRelay(db, nc, time.Second)
	if err != nil {
		log.Fatalf("outbox relay: %v", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}
	bindingStore := corestore.NewProviderBindingStore(db)
	reconciler := core.NewProviderReconciler(nc, js, bindingStore, 60*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		cancel()
	}()

	log.Println("operation-worker starting")
	errCh := make(chan error, 3)
	go func() { errCh <- relay.Start(ctx) }()
	go func() { errCh <- w.Start(ctx) }()
	go func() { reconciler.Start(ctx) }()
	if err := <-errCh; err != nil {
		cancel()
		log.Fatalf("operation-worker failed: %v", err)
	}
	cancel()
	log.Println("operation-worker stopped")
}
