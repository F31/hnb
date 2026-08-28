package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gnats "github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/cmd/gateway-provider/internal/config"
	"github.com/F31/hnb/cmd/gateway-provider/internal/metrics"
	"github.com/F31/hnb/cmd/gateway-provider/internal/nats"
	"github.com/F31/hnb/pkg/messaging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	opts := []gnats.Option{
		gnats.MaxReconnects(-1),
		gnats.ReconnectWait(2 * time.Second),
		gnats.ReconnectHandler(func(_ *gnats.Conn) {
			log.Println("nats reconnected")
		}),
		gnats.ClosedHandler(func(_ *gnats.Conn) {
			log.Println("nats connection closed")
		}),
		gnats.DisconnectErrHandler(func(_ *gnats.Conn, err error) {
			log.Printf("nats disconnected: %v", err)
		}),
	}
	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL, opts...)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	w, err := nats.NewWorker(nc, cfg.Kubeconfig, cfg.GatewayAdapter)
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
	mux.HandleFunc("/healthz", func(wr http.ResponseWriter, _ *http.Request) {
		wr.WriteHeader(http.StatusOK)
		wr.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(wr http.ResponseWriter, _ *http.Request) {
		if nc.IsConnected() {
			wr.WriteHeader(http.StatusOK)
			wr.Write([]byte("ready"))
		} else {
			wr.WriteHeader(http.StatusServiceUnavailable)
			wr.Write([]byte("not ready"))
		}
	})
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

	metrics.HealthCheckDuration.Set(0)

	if w.Applier() != nil {
		reconciler := nats.NewGatewayReconciler(w.Applier(), 5*time.Minute)
		go reconciler.Start(ctx)
	}

	log.Println("gateway-provider starting")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("gateway provider failed: %v", err)
	}
	log.Println("gateway-provider stopped")
}
