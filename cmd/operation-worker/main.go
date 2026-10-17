package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/operation-worker/internal/config"
	"github.com/F31/hnb/cmd/operation-worker/internal/driver"
	"github.com/F31/hnb/cmd/operation-worker/internal/nats"
	workstore "github.com/F31/hnb/cmd/operation-worker/internal/store"
	corestore "github.com/F31/hnb/pkg/core/store"
	dbpkg "github.com/F31/hnb/pkg/db"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/messaging"

	"github.com/F31/hnb/pkg/core"
)

const (
	// A step whose request was lost must not be re-driven before JetStream had a
	// chance to redeliver it, so the idle threshold stays above the consumer AckWait.
	orphanedStepIdleThreshold = 2 * time.Minute
	redriveSweepInterval      = 60 * time.Second
	redriveSweepLimit         = 20
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

	if err := dbpkg.WaitForReady(context.Background(), db); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	sourceCtx, sourceCancel := context.WithCancel(context.Background())
	defer sourceCancel()
	identitySources := make(map[string]*iam.TenantRenewingTokenSource)
	if cfg.ServiceIdentitiesEnabled() {
		keys, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{
			ManifestPath: cfg.ServiceIdentityKeyManifest,
			Issuer:       cfg.ServiceIdentityIssuer,
			OnSuccess: func(stats iam.KeyReloadStats) {
				log.Printf("service identity key manifest generation %d loaded", stats.Generation)
			},
		})
		if err != nil {
			log.Fatalf("service identity keys: %v", err)
		}
		if err := keys.StartPolling(sourceCtx, cfg.ServiceIdentityKeyReloadInterval, func(err error) {
			log.Printf("service identity key manifest reload failed: %v", err)
		}); err != nil {
			log.Fatalf("service identity key polling: %v", err)
		}
		for providerID, provider := range cfg.RuntimeProviders {
			if len(provider.ServiceIdentities) == 0 {
				continue
			}
			verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
				Issuer: cfg.ServiceIdentityIssuer, Audience: provider.Audience, AccessTTL: iam.MaxAccessTokenTTL,
			}, keys)
			if err != nil {
				log.Fatalf("service identity verifier for %s: %v", providerID, err)
			}
			bindings := make(map[string]iam.ServiceIdentityBinding, len(provider.ServiceIdentities))
			for tenantID, identity := range provider.ServiceIdentities {
				bindings[tenantID] = iam.ServiceIdentityBinding{
					IdentityID: identity.IdentityID, SubjectID: identity.SubjectID, TokenFile: identity.TokenFile,
				}
			}
			source, err := iam.NewHTTPTenantTokenSource(sourceCtx, verifier, cfg.ServiceIdentityURL, providerID, provider.Audience, bindings, time.Now)
			if err != nil {
				log.Fatalf("service identity source for %s: %v", providerID, err)
			}
			identitySources[providerID] = source
		}
	}
	providers := make(map[string]driver.ProviderConfig, len(cfg.RuntimeProviders))
	for providerID, provider := range cfg.RuntimeProviders {
		var tokenSource iam.TokenSource = iam.FileTokenSource{Path: provider.TokenFile}
		if source, ok := identitySources[providerID]; ok {
			tokenSource = source
		}
		providers[providerID] = driver.ProviderConfig{
			Endpoint:         provider.Endpoint,
			Audience:         provider.Audience,
			TokenSource:      tokenSource,
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
	healthServer := &http.Server{Addr: ":8081", ReadHeaderTimeout: 5 * time.Second,
		Handler: dbpkg.WithHealth(http.NotFoundHandler(), db, nc.IsConnected)}
	defer healthServer.Close()
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("health server: %v", err)
		}
	}()
	errCh := make(chan error, 4)
	go func() { errCh <- relay.Start(ctx) }()
	go func() { errCh <- w.Start(ctx) }()
	go func() { reconciler.Start(ctx) }()
	go func() {
		sweeper := workstore.NewOrphanedStepSweeper(
			workstore.NewOperationStore(db), orphanedStepIdleThreshold, redriveSweepInterval, redriveSweepLimit)
		sweeper.Start(ctx)
		errCh <- nil
	}()
	if err := <-errCh; err != nil {
		cancel()
		log.Fatalf("operation-worker failed: %v", err)
	}
	cancel()
	log.Println("operation-worker stopped")
}
