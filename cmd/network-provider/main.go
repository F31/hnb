package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/cmd/network-provider/internal/config"
	"github.com/F31/hnb/cmd/network-provider/internal/nats"
	"github.com/F31/hnb/cmd/network-provider/internal/provider"
	"github.com/F31/hnb/cmd/network-provider/internal/provider/network"
	"github.com/F31/hnb/pkg/messaging"
)

func main() {
	cfg := config.Load()

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	providers := registerProviders(cfg)

	w, err := nats.NewWorker(nc, providers)
	if err != nil {
		log.Fatalf("worker: %v", err)
	}

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

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		log.Printf("metrics server on :8080")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		metricsServer.Shutdown(context.Background())
	}()

	log.Println("network-provider starting")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("network provider failed: %v", err)
	}
	log.Println("network-provider stopped")
}

func registerProviders(cfg *config.Config) map[string]provider.NetworkProvider {
	providers := make(map[string]provider.NetworkProvider)

	cilium := network.NewCiliumProvider(cfg.HelmPath)
	providers[cilium.Name()] = cilium
	log.Printf("registered provider: %s", cilium.Name())

	calico := network.NewCalicoProvider(cfg.HelmPath)
	providers[calico.Name()] = calico
	log.Printf("registered provider: %s", calico.Name())

	kubeovn := network.NewKubeOVNProvider(cfg.HelmPath)
	providers[kubeovn.Name()] = kubeovn
	log.Printf("registered provider: %s", kubeovn.Name())

	return providers
}
