package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"

	"github.com/F31/hnb/pkg/kms"
	"github.com/F31/hnb/pkg/messaging"
	"github.com/F31/hnb/pkg/network"
)

func main() {
	db, err := sql.Open("postgres", getEnv("DB_DSN", ""))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	var cipher kms.Decrypter
	if key, kerr := kms.MasterKeyFromHex(getEnv("HNB_MASTER_KEY", "")); kerr == nil {
		cipher, err = kms.NewAESGCM(key)
		if err != nil {
			log.Fatalf("kms: %v", err)
		}
	} else if getEnv("HNB_MASTER_KEY", "") != "" {
		log.Fatalf("kms: %v", kerr)
	} else {
		log.Printf("WARNING: no HNB_MASTER_KEY configured; target credential resolution will fail closed")
	}

	nc, err := messaging.ConnectNATSFromEnv(getEnv("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	providers := map[string]network.NetworkProvider{
		"kube-ovn": network.NewKubeOVNProvider(getEnv("HELM_PATH", "helm")),
	}

	resolver := network.NewPGCredentialResolver(db, cipher, getEnv("KUBECONFIG_DIR", "/tmp/hnb-kube-ovn-provider"))
	w, err := network.NewWorker(nc, providers, "hnb.network.kube-ovn.>", resolver)
	if err != nil {
		log.Fatalf("worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	log.Println("kube-ovn-provider starting")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("kube-ovn provider failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
