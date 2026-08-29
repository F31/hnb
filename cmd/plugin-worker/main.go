package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/cmd/plugin-worker/internal/config"
	"github.com/F31/hnb/cmd/plugin-worker/internal/worker"
	"github.com/F31/hnb/pkg/kms"
	"github.com/F31/hnb/pkg/messaging"
)

func main() {
	cfg := config.Load()

	db, err := sql.Open("postgres", cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	var cipher kms.Decrypter
	if key, kerr := kms.MasterKeyFromHex(cfg.MasterKeyHex); kerr == nil {
		cipher, err = kms.NewAESGCM(key)
		if err != nil {
			log.Fatalf("kms: %v", err)
		}
	} else if cfg.MasterKeyHex != "" {
		log.Fatalf("kms: %v", kerr)
	}

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	if cfg.ListenAddr != "" {
		metricsServer := &http.Server{Addr: cfg.ListenAddr, Handler: mux}
		go func() {
			log.Printf("[plugin-worker] metrics on %s", cfg.ListenAddr)
			if serr := metricsServer.ListenAndServe(); serr != nil && serr != http.ErrServerClosed {
				log.Printf("[plugin-worker] metrics server: %v", serr)
			}
		}()
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

	w := worker.New(db, cipher, nc, cfg.HelmPath, cfg.KubeconfigDir)
	log.Println("[plugin-worker] starting")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("plugin-worker failed: %v", err)
	}
	log.Println("[plugin-worker] stopped")
}
