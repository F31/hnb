package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type CheckFunc func(ctx context.Context) error

type HealthServer struct {
	addr        string
	mu          sync.RWMutex
	checks      map[string]CheckFunc
	lastCheck   time.Time
	overallHealth string
}

func NewHealthServer(addr string) *HealthServer {
	return &HealthServer{
		addr:          addr,
		checks:        make(map[string]CheckFunc),
		overallHealth: "unknown",
	}
}

func (h *HealthServer) RegisterCheck(name string, fn CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = fn
}

func (h *HealthServer) Start(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/readyz", h.handleReady)

	server := &http.Server{
		Addr:    h.addr,
		Handler: mux,
	}

	go func() {
		klog := &logWriter{}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Write([]byte(fmt.Sprintf("Health server error: %v", err)))
		}
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				server.Shutdown(context.Background())
				return
			case <-ticker.C:
				h.runChecks(ctx)
			}
		}
	}()
}

func (h *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	status := h.overallHealth
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	h.mu.RLock()
	checkResults := make(map[string]string)
	for name := range h.checks {
		checkResults[name] = "ok"
	}
	h.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"checks":  checkResults,
		"updated": h.lastCheck,
	})
}

func (h *HealthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	status := h.overallHealth
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if status == "healthy" || status == "degraded" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
	})
}

func (h *HealthServer) runChecks(ctx context.Context) {
	h.mu.RLock()
	checks := make(map[string]CheckFunc)
	for k, v := range h.checks {
		checks[k] = v
	}
	h.mu.RUnlock()

	allOK := true
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for _, fn := range checks {
		if err := fn(checkCtx); err != nil {
			allOK = false
		}
	}

	h.mu.Lock()
	h.lastCheck = time.Now()
	if allOK {
		h.overallHealth = "healthy"
	} else {
		h.overallHealth = "degraded"
	}
	h.mu.Unlock()
}

type logWriter struct{}

func (l *logWriter) Write(p []byte) (n int, err error) {
	fmt.Print(string(p))
	return len(p), nil
}
