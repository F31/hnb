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

	"github.com/F31/hnb/cmd/edge-provider/internal/provider"
	"github.com/F31/hnb/pkg/iam"
)

func main() {
	cfg, err := provider.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	k8sClient, dynamicClient, restMapper, err := provider.NewCloudCoreClient(cfg.CloudCoreEndpoint, cfg.Kubeconfig)
	if err != nil {
		log.Fatalf("cloudcore client: %v", err)
	}

	version, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		log.Fatalf("cloudcore connection: %v", err)
	}
	log.Printf("connected to CloudCore %s", version.GitVersion)

	executor := provider.NewExecutor(k8sClient, dynamicClient, restMapper, cfg.AllowedNamespaces, cfg.MaxReplicas)
	authenticator, err := loadServiceAuthenticator("hnb-edge-provider")
	if err != nil {
		log.Fatalf("service identity: %v", err)
	}
	handler := iam.RequireServiceIdentity(authenticator, iam.ActionExecute, iam.RuntimeExecutionScope(64<<10), "/healthz")(provider.NewHandler(executor))
	server := &http.Server{Addr: cfg.ListenAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	log.Printf("edge-provider listening on %s (CloudCore: %s)", cfg.ListenAddress, cfg.CloudCoreEndpoint)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func loadServiceAuthenticator(expectedAudience string) (*iam.ServiceAuthenticator, error) {
	issuer := os.Getenv("API_TOKEN_ISSUER")
	audience := os.Getenv("API_TOKEN_AUDIENCE")
	if issuer == "" || audience != expectedAudience {
		return nil, errors.New("API_TOKEN_ISSUER and the exact provider API_TOKEN_AUDIENCE are required")
	}
	manifestPath := os.Getenv("API_TOKEN_KEY_MANIFEST_FILE")
	if manifestPath == "" {
		return nil, errors.New("API_TOKEN_KEY_MANIFEST_FILE is required")
	}
	interval, err := iam.ParseKeyReloadInterval(envOrDefault("API_TOKEN_KEY_RELOAD_INTERVAL", "5s"))
	if err != nil {
		return nil, err
	}
	keys, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{ManifestPath: manifestPath, Issuer: issuer, OnSuccess: func(stats iam.KeyReloadStats) {
		log.Printf("identity key manifest generation %d loaded", stats.Generation)
	}})
	if err != nil {
		return nil, err
	}
	if err := keys.StartPolling(context.Background(), interval, func(err error) { log.Printf("identity key manifest reload failed: %v", err) }); err != nil {
		return nil, err
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{Issuer: issuer, Audience: audience, AccessTTL: iam.MaxAccessTokenTTL}, keys)
	if err != nil {
		return nil, err
	}
	return iam.NewServiceAuthenticator(verifier)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
