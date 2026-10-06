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
	// 服务级健康检查模板下放（迁移 094，GSLB 四期）：
	// Path/Timeout/FailureThreshold 为 0 值时回退引擎全局默认。
	// 注意：探测周期保持引擎全局 interval（服务级 interval 不下放，
	// 避免每集群独立 ticker 的复杂度）；同一集群被多服务引用时由上层
	// 预先聚合为最严格配置（最短 timeout、最高 threshold）。
	Path             string
	Timeout          time.Duration
	FailureThreshold int
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
			cfg := DefaultFailoverConfig()
			// 服务级 failureThreshold 接入 failover 状态机判定
			if t.FailureThreshold > 0 {
				cfg.ConsecutiveFailures = t.FailureThreshold
			}
			e.trackers[t.Name] = NewFailoverTracker(cfg)
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
	timeout := e.timeout
	if target.Timeout > 0 {
		timeout = target.Timeout
	}
	path := target.Path
	if path == "" {
		path = "/healthz"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", target.Endpoint+path, nil)
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