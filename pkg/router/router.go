package router

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/F31/hnb/pkg/tunnel"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	routerRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_router_requests_total",
		Help: "Total requests routed through cluster router",
	}, []string{"cluster_id", "status"})

	routerRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hnb_router_request_duration_seconds",
		Help:    "Request duration through cluster router",
		Buckets: prometheus.DefBuckets,
	}, []string{"cluster_id"})

	routerCircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_router_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
	}, []string{"cluster_id"})

	routerActiveConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hnb_router_active_connections",
		Help: "Active connections per cluster",
	}, []string{"cluster_id"})

	routerPoolSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_router_pool_size",
		Help: "Connection pool size",
	})
)

type ClusterRouter struct {
	mu          sync.RWMutex
	routes      map[string]*ClusterRoute
	registry    *tunnel.AgentRegistry
	balancer    Balancer
	pool        *ConnectionPool
	healthCheck time.Duration
}

func NewClusterRouter(registry *tunnel.AgentRegistry, balancer Balancer, pool *ConnectionPool, healthCheckInterval time.Duration) *ClusterRouter {
	cr := &ClusterRouter{
		routes:      make(map[string]*ClusterRoute),
		registry:    registry,
		balancer:    balancer,
		pool:        pool,
		healthCheck: healthCheckInterval,
	}
	go cr.healthLoop()
	return cr
}

func (cr *ClusterRouter) healthLoop() {
	ticker := time.NewTicker(cr.healthCheck)
	defer ticker.Stop()

	for range ticker.C {
		cr.syncRoutes()
		cr.checkHealth()
	}
}

func (cr *ClusterRouter) syncRoutes() {
	agents := cr.registry.List()

	cr.mu.Lock()
	defer cr.mu.Unlock()

	active := make(map[string]bool)
	for _, agent := range agents {
		active[agent.ClusterID] = true
		if _, exists := cr.routes[agent.ClusterID]; !exists {
			cr.routes[agent.ClusterID] = NewClusterRoute(agent.ClusterID)
			log.Printf("[router] added route for cluster %s", agent.ClusterID)
		}
	}

	for id, route := range cr.routes {
		if !active[id] {
			route.Backend.IsHealthy = false
			log.Printf("[router] cluster %s agent disconnected", id)
		}
	}
}

func (cr *ClusterRouter) checkHealth() {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	for _, route := range cr.routes {
		routerCircuitBreakerState.WithLabelValues(route.ClusterID).Set(float64(route.Breaker.State()))
	}
}

func (cr *ClusterRouter) Route(clusterID string) (*ClusterRoute, error) {
	cr.mu.RLock()
	route, exists := cr.routes[clusterID]
	cr.mu.RUnlock()

	if !exists {
		cr.mu.Lock()
		route = NewClusterRoute(clusterID)
		cr.routes[clusterID] = route
		cr.mu.Unlock()
	}

	if !route.Breaker.Allow() {
		routerRequestsTotal.WithLabelValues(clusterID, "circuit_open").Inc()
		return nil, fmt.Errorf("circuit breaker open for cluster %s", clusterID)
	}

	_, err := cr.pool.Acquire(clusterID)
	if err != nil {
		return nil, fmt.Errorf("pool acquire: %w", err)
	}
	routerActiveConnections.WithLabelValues(clusterID).Set(float64(cr.pool.Size()))
	routerPoolSize.Set(float64(cr.pool.Size()))

	return route, nil
}

func (cr *ClusterRouter) ReportSuccess(clusterID string) {
	cr.mu.RLock()
	route, exists := cr.routes[clusterID]
	cr.mu.RUnlock()

	if exists {
		route.Breaker.Success()
		route.Backend.IsHealthy = true
		route.LastSuccessAt = time.Now()
		route.Backend.LastUsed = time.Now()
		routerRequestsTotal.WithLabelValues(clusterID, "success").Inc()
	}

	cr.pool.Release(clusterID)
}

func (cr *ClusterRouter) ReportFailure(clusterID string, err error) {
	cr.mu.RLock()
	route, exists := cr.routes[clusterID]
	cr.mu.RUnlock()

	if exists {
		route.Breaker.Failure()
		route.Backend.Failures++
		route.LastError = err.Error()
		route.LastFailureAt = time.Now()

		atomicLoad := route.Backend.Failures
		if atomicLoad >= 3 {
			route.Backend.IsHealthy = false
		}
		routerRequestsTotal.WithLabelValues(clusterID, "failure").Inc()
		routerCircuitBreakerState.WithLabelValues(clusterID).Set(float64(route.Breaker.State()))
	}

	cr.pool.Release(clusterID)
}

func (cr *ClusterRouter) SelectBackend(clusterID string) *Backend {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	backends := make([]*Backend, 0, len(cr.routes))
	for _, route := range cr.routes {
		if route.Backend.IsHealthy {
			backends = append(backends, route.Backend)
		}
	}

	if len(backends) == 0 {
		return nil
	}

	if clusterID != "" {
		for _, be := range backends {
			if be.ClusterID == clusterID {
				return be
			}
		}
		return nil
	}

	return cr.balancer.Select(backends)
}

func (cr *ClusterRouter) RouteViaTunnel(clusterID string, agentConn *tunnel.AgentConnection, req *tunnel.RequestPayload) (*tunnel.ResponsePayload, error) {
	start := time.Now()

	_, err := cr.Route(clusterID)
	if err != nil {
		return nil, err
	}

	resp, err := agentConn.SendRequest(req)
	if err != nil {
		cr.ReportFailure(clusterID, err)
		return nil, err
	}

	cr.ReportSuccess(clusterID)
	routerRequestDuration.WithLabelValues(clusterID).Observe(time.Since(start).Seconds())

	return resp, nil
}

func (cr *ClusterRouter) Routes() []*ClusterRoute {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	routes := make([]*ClusterRoute, 0, len(cr.routes))
	for _, route := range cr.routes {
		routes = append(routes, route)
	}
	return routes
}

func (cr *ClusterRouter) GetRoute(clusterID string) *ClusterRoute {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.routes[clusterID]
}

func (cr *ClusterRouter) Stats() map[string]any {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	stats := make(map[string]any)
	for id, route := range cr.routes {
		stats[id] = map[string]any{
			"breaker":       route.Breaker.Stats(),
			"healthy":       route.Backend.IsHealthy,
			"failures":      route.Backend.Failures,
			"last_error":    route.LastError,
			"last_success":  route.LastSuccessAt,
			"last_failure":  route.LastFailureAt,
		}
	}
	return stats
}

func (cr *ClusterRouter) ResetBreaker(clusterID string) {
	cr.mu.RLock()
	route, exists := cr.routes[clusterID]
	cr.mu.RUnlock()

	if exists {
		route.Breaker.Reset()
		route.Backend.IsHealthy = true
		route.Backend.Failures = 0
		log.Printf("[router] reset circuit breaker for cluster %s", clusterID)
	}
}

func (cr *ClusterRouter) BalancerName() string {
	return cr.balancer.Name()
}

func (cr *ClusterRouter) PoolStats() map[string]any {
	return cr.pool.Stats()
}