package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/F31/hnb/cmd/cluster-router/internal/config"
	"github.com/F31/hnb/cmd/cluster-router/internal/metrics"
	"github.com/F31/hnb/pkg/router"
	"github.com/F31/hnb/pkg/tunnel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	registry := tunnel.NewAgentRegistry()

	var balancer router.Balancer
	switch cfg.BalancerType {
	case "least_connections":
		balancer = router.NewLeastConnBalancer()
	case "random":
		balancer = router.NewRandomBalancer()
	default:
		balancer = router.NewRoundRobinBalancer()
	}

	pool := router.NewConnectionPool(cfg.PoolMaxSize, cfg.PoolTTL)
	clusterRouter := router.NewClusterRouter(registry, balancer, pool, cfg.HealthCheckInt)

	metrics.Serve(cfg.MetricsAddr)

	go pollAgents(cfg.TunnelAPIURL, registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"balancer":  clusterRouter.BalancerName(),
			"pool_size": pool.Size(),
			"routes":    len(clusterRouter.Routes()),
		})
	})

	mux.HandleFunc("GET /routes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(clusterRouter.Routes())
	})

	mux.HandleFunc("GET /routes/{cluster_id}", func(w http.ResponseWriter, r *http.Request) {
		clusterID := r.PathValue("cluster_id")
		route := clusterRouter.GetRoute(clusterID)
		if route == nil {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(route)
	})

	mux.HandleFunc("POST /routes/{cluster_id}/reset", func(w http.ResponseWriter, r *http.Request) {
		clusterID := r.PathValue("cluster_id")
		clusterRouter.ResetBreaker(clusterID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{
			"routes":   clusterRouter.Stats(),
			"pool":     clusterRouter.PoolStats(),
			"balancer": clusterRouter.BalancerName(),
		}
		json.NewEncoder(w).Encode(data)
	})

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println()
		log.Println("shutting down...")
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("cluster-router listening on %s (balancer=%s)", cfg.ListenAddr, cfg.BalancerType)
	log.Printf("  pool: max=%d ttl=%s circuit: threshold=%d reset=%s health=%s",
		cfg.PoolMaxSize, cfg.PoolTTL, cfg.CircuitThreshold, cfg.CircuitReset, cfg.HealthCheckInt)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("cluster-router stopped")
}

func pollAgents(tunnelAPIURL string, registry *tunnel.AgentRegistry) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		agents, err := fetchAgents(tunnelAPIURL)
		if err != nil {
			log.Printf("[router] fetch agents: %v", err)
			continue
		}

		// Register all agents from tunnel server
		for _, agent := range agents {
			if _, ok := registry.Get(agent.ClusterID); !ok {
				log.Printf("[router] discovered agent %s", agent.ClusterID)
			}
		}
	}
}

func fetchAgents(tunnelAPIURL string) ([]tunnel.AgentInfo, error) {
	resp, err := http.Get(fmt.Sprintf("%s/agents", tunnelAPIURL))
	if err != nil {
		return nil, fmt.Errorf("get agents: %w", err)
	}
	defer resp.Body.Close()

	var agents []tunnel.AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("decode agents: %w", err)
	}
	return agents, nil
}