package healthsource

import (
	"context"
	"time"
)

type ClusterTarget struct {
	Name     string
	Endpoint string
	Labels   map[string]string
}

type HealthResult struct {
	Status    string
	Source    string
	Timestamp time.Time
	Details   map[string]string
}

type HealthSource interface {
	Name() string
	Probe(ctx context.Context, targets []ClusterTarget) (map[string]HealthResult, error)
}

type MergePolicy string

const (
	MergeAllHealthy      MergePolicy = "all-healthy"
	MergeAnyHealthy      MergePolicy = "any-healthy"
	MergePrimaryFallback MergePolicy = "primary-fallback"
)

func ParseMergePolicy(s string) MergePolicy {
	switch s {
	case "any-healthy":
		return MergeAnyHealthy
	case "primary-fallback":
		return MergePrimaryFallback
	default:
		return MergeAllHealthy
	}
}

func MergeResults(results map[string]map[string]HealthResult, policy MergePolicy) map[string]HealthResult {
	merged := make(map[string]HealthResult)

	for clusterName, sourceResults := range results {
		var best HealthResult
		best.Status = "unknown"

		switch policy {
		case MergeAnyHealthy:
			for _, r := range sourceResults {
				if r.Status == "healthy" {
					best = r
					break
				}
				if r.Status != "unknown" {
					best = r
				}
			}
			if best.Status == "" {
				best.Status = "unknown"
			}

		case MergePrimaryFallback:
			primary, hasPrimary := sourceResults["http"]
			if hasPrimary && primary.Status == "healthy" {
				best = primary
			} else if hasPrimary {
				best = primary
				for _, r := range sourceResults {
					if r.Source != "http" && r.Status == "healthy" {
						best = r
						break
					}
				}
			} else {
				for _, r := range sourceResults {
					best = r
					break
				}
			}

		default:
			// 取最差状态（all-healthy 合并）；用 found 标志选择首个源，
			// 避免 map 迭代顺序影响结果（GSLB-011 多源聚合）。
			worst := -1
			found := false
			rank := map[string]int{
				"unreachable": 0,
				"degraded":    1,
				"unknown":     2,
				"healthy":     3,
			}
			for _, r := range sourceResults {
				rk := rank[r.Status]
				if !found || rk < worst {
					found = true
					worst = rk
					best = r
				}
			}
			if best.Status == "" {
				best.Status = "unknown"
			}
		}

		best.Timestamp = time.Now()
		merged[clusterName] = best
	}

	return merged
}