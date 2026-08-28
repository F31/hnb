package health

import (
	"sync"
	"time"
)

type ClusterState string

const (
	StateHealthy   ClusterState = "healthy"
	StateDegraded  ClusterState = "degraded"
	StateUnhealthy ClusterState = "unhealthy"
)

type FailoverConfig struct {
	ConsecutiveSuccesses int
	ConsecutiveFailures  int
	DebounceDuration     time.Duration
}

func DefaultFailoverConfig() FailoverConfig {
	return FailoverConfig{
		ConsecutiveSuccesses: 2,
		ConsecutiveFailures:  3,
		DebounceDuration:     10 * time.Second,
	}
}

type FailoverTracker struct {
	mu sync.RWMutex

	state                 ClusterState
	consecutiveSuccesses  int
	consecutiveFailures   int
	lastTransitionAt      time.Time
	lastProbeResult       string
	cfg                   FailoverConfig
}

func NewFailoverTracker(cfg FailoverConfig) *FailoverTracker {
	return &FailoverTracker{
		state:            StateHealthy,
		lastTransitionAt: time.Now(),
		cfg:              cfg,
	}
}

type ProbeResult struct {
	Status string
	Error  string
}

func (t *FailoverTracker) RecordProbe(result ProbeResult) (ClusterState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastProbeResult = result.Status

	wasHealthy := t.state == StateHealthy

	switch result.Status {
	case "healthy":
		t.consecutiveFailures = 0
		t.consecutiveSuccesses++

		if t.state != StateHealthy && t.consecutiveSuccesses >= t.cfg.ConsecutiveSuccesses {
			if time.Since(t.lastTransitionAt) >= t.cfg.DebounceDuration {
				t.state = StateHealthy
				t.lastTransitionAt = time.Now()
				t.consecutiveSuccesses = 0
				return t.state, wasHealthy
			}
		}

	case "unreachable", "degraded":
		t.consecutiveSuccesses = 0
		t.consecutiveFailures++

		if t.state == StateHealthy && t.consecutiveFailures >= t.cfg.ConsecutiveFailures {
			if time.Since(t.lastTransitionAt) >= t.cfg.DebounceDuration {
				t.state = StateUnhealthy
				t.lastTransitionAt = time.Now()
				t.consecutiveFailures = 0
				return t.state, wasHealthy
			}
		}
	}

	return t.state, wasHealthy
}

func (t *FailoverTracker) State() ClusterState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

func (t *FailoverTracker) LastTransitionAt() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastTransitionAt
}

func (t *FailoverTracker) LastProbeResult() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastProbeResult
}

func (t *FailoverTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = StateHealthy
	t.consecutiveSuccesses = 0
	t.consecutiveFailures = 0
	t.lastTransitionAt = time.Now()
	t.lastProbeResult = ""
}