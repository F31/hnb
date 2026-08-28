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

	"github.com/F31/hnb/cmd/extension-controller/internal/config"
	"github.com/F31/hnb/cmd/extension-controller/internal/metrics"
	"github.com/F31/hnb/cmd/extension-controller/internal/nats"
	"github.com/F31/hnb/pkg/extension"
	"github.com/F31/hnb/pkg/messaging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := sql.Open("postgres", cfg.DBDSN)
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

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	extStore := extension.NewExtensionDBStore(db)
	extManager := extension.NewExtensionManager(extStore, nc, js)
	extWorker := nats.NewWorker(extManager, nc, js)
	extReconciler := extension.NewExtensionReconciler(extManager, extStore, time.Duration(cfg.ReconcileInterval)*time.Second)

	metrics.Serve(cfg.ListenAddr)

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

	log.Println("extension-controller starting")
	errCh := make(chan error, 2)
	go func() { errCh <- extWorker.Start(ctx) }()
	go func() { extReconciler.Start(ctx) }()
	if err := <-errCh; err != nil {
		cancel()
		log.Fatalf("extension-controller failed: %v", err)
	}
	cancel()
	log.Println("extension-controller stopped")
}
