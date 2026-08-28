package healthsource

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

type HealthManager struct {
	sources     []HealthSource
	mergePolicy MergePolicy
	lastResults map[string]map[string]HealthResult
	mu          sync.RWMutex
}

func NewHealthManager(sources []HealthSource, policy MergePolicy) *HealthManager {
	return &HealthManager{
		sources:     sources,
		mergePolicy: policy,
		lastResults: make(map[string]map[string]HealthResult),
	}
}

func (m *HealthManager) ProbeAll(ctx context.Context, targets []ClusterTarget) map[string]HealthResult {
	allResults := make(map[string]map[string]HealthResult)
	for _, t := range targets {
		allResults[t.Name] = make(map[string]HealthResult)
	}

	var wg sync.WaitGroup
	for _, src := range m.sources {
		wg.Add(1)
		go func(s HealthSource) {
			defer wg.Done()
			results, err := s.Probe(ctx, targets)
			if err != nil {
				log.Printf("[healthsource] %s probe error: %v", s.Name(), err)
				for _, t := range targets {
					allResults[t.Name][s.Name()] = HealthResult{
						Status: "unknown",
						Source: s.Name(),
						Details: map[string]string{
							"error": err.Error(),
						},
					}
				}
				return
			}
			m.mu.Lock()
			for name, result := range results {
				allResults[name][s.Name()] = result
			}
			m.mu.Unlock()
		}(src)
	}
	wg.Wait()

	merged := MergeResults(allResults, m.mergePolicy)

	m.mu.Lock()
	m.lastResults = allResults
	m.mu.Unlock()

	return merged
}

func (m *HealthManager) GetMergedStatus(clusterName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results, ok := m.lastResults[clusterName]
	if !ok {
		return "unknown"
	}
	merged := MergeResults(map[string]map[string]HealthResult{clusterName: results}, m.mergePolicy)
	if r, ok := merged[clusterName]; ok {
		return r.Status
	}
	return "unknown"
}

func (m *HealthManager) GetAllMergedStatuses() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	merged := MergeResults(m.lastResults, m.mergePolicy)
	result := make(map[string]string, len(merged))
	for k, v := range merged {
		result[k] = v.Status
	}
	return result
}

func (m *HealthManager) GetSourceStatus(clusterName, sourceName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if results, ok := m.lastResults[clusterName]; ok {
		if r, ok := results[sourceName]; ok {
			return r.Status
		}
	}
	return "unknown"
}

func (m *HealthManager) GetAllStatuses() map[string]map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]map[string]string)
	for clusterName, sources := range m.lastResults {
		entry := make(map[string]string)
		for srcName, hr := range sources {
			entry[srcName] = hr.Status
		}
		result[clusterName] = entry
	}
	return result
}

func (m *HealthManager) HealthyTargets(targets []ClusterTarget) []ClusterTarget {
	merged := m.GetAllMergedStatuses()
	var healthy []ClusterTarget
	for _, t := range targets {
		if merged[t.Name] == "healthy" {
			healthy = append(healthy, t)
		}
	}
	return healthy
}

func (m *HealthManager) Sources() []HealthSource {
	return m.sources
}

func ParseSources(s string, defaultInterval, defaultTimeout time.Duration) ([]HealthSource, error) {
	if s == "" {
		s = "http"
	}
	parts := strings.Split(s, ",")
	var sources []HealthSource
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "http":
			sources = append(sources, NewHTTPSource(defaultInterval, defaultTimeout))
		case "karmada":
			ks, err := NewKarmadaSource("")
			if err != nil {
				log.Printf("[healthsource] karmada source unavailable: %v (skipping)", err)
				continue
			}
			sources = append(sources, ks)
		default:
			log.Printf("[healthsource] unknown source %q (skipping)", p)
		}
	}
	if len(sources) == 0 {
		sources = append(sources, NewHTTPSource(defaultInterval, defaultTimeout))
	}
	return sources, nil
}

func ParseSourcesWithKarmada(s, karmadaKubeconfig string, defaultInterval, defaultTimeout time.Duration) ([]HealthSource, error) {
	if s == "" {
		s = "http"
	}
	parts := strings.Split(s, ",")
	var sources []HealthSource
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "http":
			sources = append(sources, NewHTTPSource(defaultInterval, defaultTimeout))
		case "karmada":
			ks, err := NewKarmadaSource(karmadaKubeconfig)
			if err != nil {
				log.Printf("[healthsource] karmada source unavailable: %v (skipping)", err)
				continue
			}
			sources = append(sources, ks)
		default:
			log.Printf("[healthsource] unknown source %q (skipping)", p)
		}
	}
	if len(sources) == 0 {
		sources = append(sources, NewHTTPSource(defaultInterval, defaultTimeout))
	}
	return sources, nil
}