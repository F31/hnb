package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/F31/hnb/cmd/platform-api/internal/api"
	"github.com/F31/hnb/cmd/platform-api/internal/config"
	"github.com/F31/hnb/cmd/platform-api/internal/observer"
	stalepolicy "github.com/F31/hnb/cmd/platform-api/internal/stale"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/kms"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	keySet, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{ManifestPath: cfg.TokenKeyManifestPath, Issuer: cfg.TokenIssuer, OnSuccess: func(stats iam.KeyReloadStats) {
		log.Printf("identity key manifest generation %d loaded", stats.Generation)
	}})
	if err != nil {
		log.Fatalf("identity public keys: %v", err)
	}
	if err := keySet.StartPolling(context.Background(), cfg.TokenKeyReloadInterval, func(err error) { log.Printf("identity key manifest reload failed: %v", err) }); err != nil {
		log.Fatalf("identity key polling: %v", err)
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, AccessTTL: iam.MaxAccessTokenTTL,
	}, keySet)
	if err != nil {
		log.Fatalf("identity verifier: %v", err)
	}
	delegationVerifier, err := iam.NewDelegationVerifier(iam.DelegationConfig{
		Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second,
	}, keySet)
	if err != nil {
		log.Fatalf("delegation verifier: %v", err)
	}

	dbc := cfg.DBConfig()
	if cfg.DBDriver == "mysql" && (cfg.Environment != "development" || !cfg.AllowUnimplementedDBBackend) {
		log.Fatalf("mysql backend is unavailable in this release (APP_ENV=%s, ALLOW_UNIMPLEMENTED_DB_BACKEND=%v)",
			cfg.Environment, cfg.AllowUnimplementedDBBackend)
	}
	db, err := dbc.Open()
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	st, err := store.New(db, store.Driver(cfg.DBDriver))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	permissionResolver := iam.NewIAMDBStore(db, cfg.TokenIssuer)
	challengeKey, err := os.ReadFile(cfg.StaleChallengeKeyFile)
	if err != nil {
		log.Fatalf("stale challenge key: %v", err)
	}
	challengeSigner, err := stalepolicy.NewSigner(challengeKey, cfg.StaleChallengeTTL)
	if err != nil {
		log.Fatalf("stale challenge signer: %v", err)
	}
	stalePolicy, err := stalepolicy.NewPolicy(cfg.StaleUpgradePolicy, cfg.StaleUnmanagePolicy)
	if err != nil {
		log.Fatalf("stale policy: %v", err)
	}
	apiHandler := api.NewServer(st, verifier, permissionResolver)
	apiHandler.ConfigureIntentDelegation(delegationVerifier)
	apiHandler.ConfigureStaleAdmission(challengeSigner, stalePolicy)
	if keyHex := os.Getenv("HNB_MASTER_KEY"); keyHex != "" {
		cipher, err := kms.NewAESGCMFromHex(keyHex)
		if err != nil {
			log.Fatalf("kms: %v", err)
		}
		apiHandler.ConfigureKMS(cipher)
	}

	observerVerifier, err := iam.NewObserverTokenVerifier(iam.ObserverTokenConfig{
		Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, TTL: iam.MaxObserverTokenTTL,
	}, keySet)
	if err != nil {
		log.Fatalf("observer identity verifier: %v", err)
	}
	ingest := observer.NewIngestHandler(
		observer.NewProjector(observer.NewPGCursorStore(db)),
		observer.NewObserverTokenIdentityVerifier(observerVerifier),
	)
	apiHandler.ConfigureObserverIngest(ingest)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("platform-api listening on %s", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("graceful shutdown: %v", err)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}
	log.Println("platform-api stopped")
}
