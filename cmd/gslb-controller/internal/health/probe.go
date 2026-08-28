package health

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type ClusterProbe struct {
	mu       sync.RWMutex
	statuses map[string]string
	interval time.Duration
	timeout  time.Duration

	trackers map[string]*FailoverTracker
}

type ProbeTarget struct {
	Name     string
	Endpoint string
}

func NewProbeEngine(interval, timeout time.Duration) *ClusterProbe {
	return &ClusterProbe{
		statuses: make(map[string]string),
		interval: interval,
		timeout:  timeout,
		trackers: make(map[string]*FailoverTracker),
	}
}

func (e *ClusterProbe) Start(ctx context.Context, targets []ProbeTarget) {
	e.ensureTrackers(targets)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	e.probeAll(targets)
	for {
		select {
		case <-ticker.C:
			e.probeAll(targets)
		case <-ctx.Done():
			return
		}
	}
}

func (e *ClusterProbe) ProbeOnce(ctx context.Context, targets []ProbeTarget) {
	e.ensureTrackers(targets)
	e.probeAll(targets)
}

func (e *ClusterProbe) ensureTrackers(targets []ProbeTarget) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range targets {
		if _, ok := e.trackers[t.Name]; !ok {
			e.trackers[t.Name] = NewFailoverTracker(DefaultFailoverConfig())
		}
	}
}

func (e *ClusterProbe) probeAll(targets []ProbeTarget) {
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(target ProbeTarget) {
			defer wg.Done()
			status := e.probe(target)
			e.mu.Lock()
			e.statuses[target.Name] = status
			tracker, ok := e.trackers[target.Name]
			e.mu.Unlock()

			if ok {
				tracker.RecordProbe(ProbeResult{Status: status})
			}
		}(t)
	}
	wg.Wait()
}

func (e *ClusterProbe) probe(target ProbeTarget) string {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", target.Endpoint+"/healthz", nil)
	if err != nil {
		return "unreachable"
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return "healthy"
	}
	return "degraded"
}

func (e *ClusterProbe) GetStatus(name string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.statuses[name]
}

func (e *ClusterProbe) GetAllStatuses() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]string, len(e.statuses))
	for k, v := range e.statuses {
		result[k] = v
	}
	return result
}

func (e *ClusterProbe) GetFailoverState(name string) ClusterState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if t, ok := e.trackers[name]; ok {
		return t.State()
	}
	return StateHealthy
}

func (e *ClusterProbe) GetAllFailoverStates() map[string]ClusterState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]ClusterState, len(e.trackers))
	for k, v := range e.trackers {
		result[k] = v.State()
	}
	return result
}

func (e *ClusterProbe) HealthyTargets(targets []ProbeTarget) []ProbeTarget {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var healthy []ProbeTarget
	for _, t := range targets {
		tracker, ok := e.trackers[t.Name]
		if !ok {
			continue
		}
		if tracker.State() == StateHealthy {
			healthy = append(healthy, t)
		}
	}
	return healthy
}