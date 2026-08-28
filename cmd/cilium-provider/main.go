package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/F31/hnb/pkg/messaging"
	"github.com/F31/hnb/pkg/network"
)

func main() {
	nc, err := messaging.ConnectNATSFromEnv(getEnv("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	providers := map[string]network.NetworkProvider{
		"cilium": network.NewCiliumProvider(getEnv("HELM_PATH", "helm")),
	}

	w, err := network.NewWorker(nc, providers, "hnb.network.cilium.>")
	if err != nil {
		log.Fatalf("worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	log.Println("cilium-provider starting")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("cilium provider failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
