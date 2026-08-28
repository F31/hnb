package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/F31/hnb/pkg/kms"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provider"
)

func main() {
	profile, err := provider.ProfileForProviderID(getEnv("PROVIDER_ID", ""))
	if err != nil {
		log.Fatal(err)
	}
	var manager provider.LifecycleManager
	switch profile.ProviderID {
	case "runtime-target.lifecycle.kubernetes":
		manager = provider.NewKubernetesManager(profile)
		log.Printf("using real Kubernetes lifecycle manager")
	case "runtime-target.lifecycle.edge":
		manager = provider.NewEdgeManager(profile)
		log.Printf("using real Edge lifecycle manager")
	default:
		manager = provider.NewMemoryManager(profile)
	}
	server := provider.NewServer(profile, manager, buildSecretResolver(), provider.NewMemoryObserverRegistry())
	addr := getEnv("LISTEN_ADDRESS", ":18082")
	log.Printf("runtime-target lifecycle provider %s listening on %s", profile.ProviderID, addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatal(err)
	}
}

// buildSecretResolver returns a database-backed secret resolver when both the
// platform DB DSN and the HNB_MASTER_KEY are configured; otherwise it falls
// back to the metadata-only resolver (which returns a placeholder) so the
// service still boots in minimal/test environments.
func buildSecretResolver() provider.SecretResolver {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Printf("DB_DSN not set; using metadata-only secret resolver")
		return provider.MetadataOnlySecretResolver{}
	}
	keyHex := os.Getenv("HNB_MASTER_KEY")
	cipher, err := kms.NewAESGCMFromHex(keyHex)
	if err != nil {
		log.Fatalf("HNB_MASTER_KEY: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Printf("database ping failed (%v); using metadata-only secret resolver", err)
		return provider.MetadataOnlySecretResolver{}
	}
	log.Printf("using PG secret resolver (db connected)")
	return provider.NewPGSecretResolver(db, cipher)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
