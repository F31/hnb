package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

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
		manager = provider.NewKubernetesManagerWithProvisioner(profile, provisioner)
		log.Printf("using real Kubernetes lifecycle manager (self-hosted provisioning enabled)")
	case "runtime-target.lifecycle.edge":
		manager = provider.NewEdgeManager(profile)
		log.Printf("using real Edge lifecycle manager")
	default:
		manager = provider.NewMemoryManager(profile)
	}
	server := provider.NewServer(profile, manager, buildSecretResolver(db), provider.NewPGObserverRegistry(db))
	addr := getEnv("LISTEN_ADDRESS", ":18082")
	log.Printf("runtime-target lifecycle provider %s listening on %s", profile.ProviderID, addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
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
