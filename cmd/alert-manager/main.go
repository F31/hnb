package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"
	gnats "github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/F31/hnb/cmd/alert-manager/internal/config"
	"github.com/F31/hnb/pkg/alert"
	"github.com/F31/hnb/pkg/messaging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := sql.Open("postgres", cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	nc, err := messaging.ConnectNATSFromEnv(cfg.NATSURL)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	store := alert.NewAlertDBStore(db)
	if err := store.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Canonical notification channels are tenant-scoped and hold only SecretReference metadata.
	// Delivery workers resolve those references; this process never loads channel secrets.
	notifier := alert.NewNotifier(nil)
	manager := alert.NewAlertManager(store, notifier)
	reconciler := alert.NewAlertReconciler(manager, cfg.EvalInterval)

	// Subscribe to NATS alert events
	subscribeNATS(nc, manager)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/alerts/rules", func(w http.ResponseWriter, r *http.Request) {
		rules, _ := manager.ListRules()
		json.NewEncoder(w).Encode(rules)
	})
	mux.HandleFunc("GET /api/v1/alerts/events", func(w http.ResponseWriter, r *http.Request) {
		events, _ := manager.ListEvents("", "", 100)
		json.NewEncoder(w).Encode(events)
	})
	mux.HandleFunc("POST /api/v1/alerts/events/{id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		userID := r.URL.Query().Get("user_id")
		if err := manager.Acknowledge(id, userID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
	})

	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		httpServer.Shutdown(context.Background())
		cancel()
	}()

	go reconciler.Start(ctx)

	log.Printf("alert-manager listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("alert-manager stopped")
}

func subscribeNATS(nc *gnats.Conn, manager *alert.AlertManager) {
	nc.Subscribe("hnb.alert.fire", func(msg *gnats.Msg) {
		var event alert.AlertEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("[alert-nats] unmarshal: %v", err)
			return
		}
		manager.FireEvent(&event)
	})

	nc.Subscribe("hnb.alert.resolve", func(msg *gnats.Msg) {
		var payload struct {
			RuleID string `json:"rule_id"`
		}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			log.Printf("[alert-nats] unmarshal: %v", err)
			return
		}
		manager.ResolveEvent(payload.RuleID)
	})

	log.Println("[alert-nats] subscribed to hnb.alert.>")
}
