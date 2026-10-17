package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/kms"
	_ "github.com/lib/pq"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provider"
	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provision"
)

func main() {
	profile, err := provider.ProfileForProviderID(getEnv("PROVIDER_ID", ""))
	if err != nil {
		log.Fatal(err)
	}
	db := buildDatabase()
	defer db.Close()
	var manager provider.LifecycleManager
	switch profile.ProviderID {
	case "runtime-target.lifecycle.kubernetes":
		cipher, err := kms.NewAESGCMFromHex(os.Getenv("HNB_MASTER_KEY"))
		if err != nil {
			log.Fatalf("HNB_MASTER_KEY: %v", err)
		}
		store := provider.NewProvisionStore(db, cipher)
		opts := []provision.Option{
			provision.WithBootstrapper(&provision.NodeBootstrapper{
				SSH:   &provision.RealSSH{},
				Key:   store,
				Store: store,
			}),
		}
		if mgmtKubeconfig := os.Getenv("MANAGEMENT_KUBECONFIG"); mgmtKubeconfig != "" {
			log.Fatal("MANAGEMENT_KUBECONFIG cannot be used by the SSH provisioner: CAPI lifecycle provider is not yet implemented")
		}
		provisioner := provision.New(opts...)
		manager = provider.NewKubernetesManagerWithProvisionerAndLoader(profile, provisioner,
			func(ctx context.Context, tenantID string, ref provision.SecretReference) ([]byte, error) {
				return provider.NewPGSecretResolver(db, cipher).Resolve(ctx, tenantID, profile.ProviderID, "kubeconfig", provider.SecretReference{
					Provider: ref.Provider, Scope: ref.Scope, Name: ref.Name, Version: ref.Version,
				})
			})
		log.Printf("using real Kubernetes lifecycle manager (self-hosted provisioning enabled)")
	case "runtime-target.lifecycle.edge":
		manager = provider.NewEdgeManager(profile)
		log.Printf("using real Edge lifecycle manager")
	default:
		manager = provider.NewMemoryManager(profile)
	}
	server := provider.NewServer(profile, manager, buildSecretResolver(db), provider.NewPGObserverRegistry(db))
	addr := getEnv("LISTEN_ADDRESS", ":18082")
	// Only /healthz is unauthenticated; every execution request must carry a
	// platform service token for this exact audience.
	authenticator, err := loadServiceAuthenticator(provider.Audience)
	if err != nil {
		log.Fatalf("service identity: %v", err)
	}
	handler := iam.RequireServiceIdentity(authenticator, iam.ActionExecute, iam.RuntimeExecutionScope(provider.ExecuteBodyLimit), "/healthz")(server.Handler())
	httpServer := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	log.Printf("runtime-target lifecycle provider %s listening on %s", profile.ProviderID, addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// buildSecretResolver returns a database-backed secret resolver. Lifecycle
// providers must fail fast without the platform DB because target credentials
// are security-bound SecretReference objects, not inline kubeconfig content.
func buildDatabase() *sql.DB {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN is required for runtime-target lifecycle provider")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}
	log.Printf("using PG database (connected)")
	return db
}

func buildSecretResolver(db *sql.DB) provider.SecretResolver {
	keyHex := os.Getenv("HNB_MASTER_KEY")
	cipher, err := kms.NewAESGCMFromHex(keyHex)
	if err != nil {
		log.Fatalf("HNB_MASTER_KEY: %v", err)
	}
	return provider.NewPGSecretResolver(db, cipher)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// loadServiceAuthenticator builds the platform service-identity verifier for the
// step execution endpoint. The lifecycle provider provisions real clusters, so an
// unauthenticated caller must never reach it; the pattern mirrors
// cmd/kubernetes-provider so every HNB provider enforces the same contract.
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
	interval, err := iam.ParseKeyReloadInterval(getEnv("API_TOKEN_KEY_RELOAD_INTERVAL", "5s"))
	if err != nil {
		return nil, err
	}
	keys, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{
		ManifestPath: manifestPath,
		Issuer:       issuer,
		OnSuccess: func(stats iam.KeyReloadStats) {
			log.Printf("identity key manifest generation %d loaded", stats.Generation)
		},
	})
	if err != nil {
		return nil, err
	}
	if err := keys.StartPolling(context.Background(), interval, func(err error) {
		log.Printf("identity key manifest reload failed: %v", err)
	}); err != nil {
		return nil, err
	}
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{Issuer: issuer, Audience: audience, AccessTTL: iam.MaxAccessTokenTTL}, keys)
	if err != nil {
		return nil, err
	}
	return iam.NewServiceAuthenticator(verifier)
}
