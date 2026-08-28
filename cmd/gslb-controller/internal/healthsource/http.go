package healthsource

import (
	"context"
	"sync"
	"time"

	"github.com/F31/hnb/cmd/gslb-controller/internal/health"
)

type HTTPSource struct {
	probeEngine *health.ClusterProbe
	interval    time.Duration
	timeout     time.Duration
	mu          sync.Mutex
}

func NewHTTPSource(interval, timeout time.Duration) *HTTPSource {
	return &HTTPSource{
		probeEngine: health.NewProbeEngine(interval, timeout),
		interval:    interval,
		timeout:     timeout,
	}
}

func (s *HTTPSource) Name() string {
	return "http"
}

func (s *HTTPSource) Probe(ctx context.Context, targets []ClusterTarget) (map[string]HealthResult, error) {
	probeTargets := convertTargets(targets)
	s.probeEngine.ProbeOnce(ctx, probeTargets)

	results := make(map[string]HealthResult, len(targets))
	for _, t := range targets {
		status := s.probeEngine.GetStatus(t.Name)
		state := s.probeEngine.GetFailoverState(t.Name)

		details := map[string]string{
			"raw_status":    status,
			"failover_state": string(state),
		}

		mergedStatus := status
		if state == health.StateUnhealthy {
			mergedStatus = "unreachable"
		} else if state == health.StateDegraded {
			mergedStatus = "degraded"
		}

		results[t.Name] = HealthResult{
			Status:    mergedStatus,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Details:   details,
		}
	}
	return results, nil
}

func (s *HTTPSource) GetFailoverStates() map[string]health.ClusterState {
	return s.probeEngine.GetAllFailoverStates()
}

func (s *HTTPSource) GetStatus(name string) string {
	return s.probeEngine.GetStatus(name)
}

func (s *HTTPSource) GetAllStatuses() map[string]string {
	return s.probeEngine.GetAllStatuses()
}

func convertTargets(targets []ClusterTarget) []health.ProbeTarget {
	pt := make([]health.ProbeTarget, len(targets))
	for i, t := range targets {
		pt[i] = health.ProbeTarget{Name: t.Name, Endpoint: t.Endpoint}
	}
	return pt
}