package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/cmd/tunnel-server/internal/config"
	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/tunnel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	keys, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{ManifestPath: cfg.TokenKeyManifestPath, Issuer: cfg.TokenIssuer, OnSuccess: func(stats iam.KeyReloadStats) {
		log.Printf("identity key manifest generation %d loaded", stats.Generation)
	}})
	if err != nil {
		log.Fatalf("identity public keys: %v", err)
	}
	if err := keys.StartPolling(context.Background(), cfg.TokenKeyReloadInterval, func(err error) { log.Printf("identity key manifest reload failed: %v", err) }); err != nil {
		log.Fatalf("identity key polling: %v", err)
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{Issuer: cfg.TokenIssuer, Audience: cfg.TokenAudience, AccessTTL: iam.MaxAccessTokenTTL}, keys)
	if err != nil {
		log.Fatalf("tunnel token verifier: %v", err)
	}
	authenticator, err := iam.NewServiceAuthenticator(verifier)
	if err != nil {
		log.Fatalf("tunnel service identity: %v", err)
	}
	verifyToken := func(ctx context.Context, token, tenantID, clusterID string) (time.Time, error) {
		trusted, err := authenticator.Authenticate(ctx, token, iam.ActionExecute, tenantID, "cluster", clusterID)
		return trusted.ExpiresAt, err
	}

	server := tunnel.NewTunnelServer(verifyToken)

	mux := http.NewServeMux()
	mux.Handle("/tunnel", server)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/agents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		agents := server.Registry().List()
		data, _ := json.Marshal(agents)
		w.Write(data)
	})

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("tunnel-server listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("tunnel-server stopped")
}
