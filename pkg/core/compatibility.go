package core

import (
	"fmt"
	"sort"
	"strings"
)

type CompatibilityIssue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type CompatibilityResult struct {
	Passed bool                 `json:"passed"`
	Issues []CompatibilityIssue `json:"issues,omitempty"`
}

type CompatibilityChecker struct{}

func NewCompatibilityChecker() *CompatibilityChecker {
	return &CompatibilityChecker{}
}

func (cc *CompatibilityChecker) Check(
	req *ResourceRequirement,
	cap *CapabilitySnapshot,
) *CompatibilityResult {
	result := &CompatibilityResult{Passed: true}

	if cap.MemoryMB > 0 && req.MinMemoryMB > 0 && cap.MemoryMB < req.MinMemoryMB {
		result.Passed = false
		result.Issues = append(result.Issues, CompatibilityIssue{
			Severity: "error",
			Category: "memory",
			Message:  fmt.Sprintf("requires %dMB memory, target has %dMB", req.MinMemoryMB, cap.MemoryMB),
		})
	}

	if cap.StorageGB > 0 && req.MinStorageGB > 0 && cap.StorageGB < req.MinStorageGB {
		result.Passed = false
		result.Issues = append(result.Issues, CompatibilityIssue{
			Severity: "error",
			Category: "storage",
			Message:  fmt.Sprintf("requires %dGB storage, target has %dGB", req.MinStorageGB, cap.StorageGB),
		})
	}

	if cap.CPUCores > 0 && req.MinCPUCores > 0 && cap.CPUCores < req.MinCPUCores {
		result.Passed = false
		result.Issues = append(result.Issues, CompatibilityIssue{
			Severity: "error",
			Category: "cpu",
			Message:  fmt.Sprintf("requires %d CPU cores, target has %d", req.MinCPUCores, cap.CPUCores),
		})
	}

	if req.RequiresGPU {
		if cap.GPUCount == 0 {
			result.Passed = false
			result.Issues = append(result.Issues, CompatibilityIssue{
				Severity: "error",
				Category: "gpu",
				Message:  "requires GPU, target has none",
			})
		}
	}

	if req.CNIRequired != "" {
		found := false
		for _, plugin := range cap.CNIPlugins {
			if strings.EqualFold(plugin, req.CNIRequired) {
				found = true
				break
			}
		}
		if !found {
			result.Passed = false
			result.Issues = append(result.Issues, CompatibilityIssue{
				Severity: "error",
				Category: "cni",
				Message:  fmt.Sprintf("requires CNI %q, target has: %v", req.CNIRequired, cap.CNIPlugins),
			})
		}
	}

	if req.CNIRequirement != nil {
		matched := false
		for _, detail := range cap.CNIDetails {
			if !strings.EqualFold(detail.Plugin, req.CNIRequirement.Plugin) {
				continue
			}
			if req.CNIRequirement.VersionRange != "" && detail.Version != "" {
				if !matchVersionRange(detail.Version, req.CNIRequirement.VersionRange) {
					continue
				}
			}
			if req.CNIRequirement.NeedPolicy && !detail.SupportsPolicy {
				continue
			}
			if req.CNIRequirement.NeedTrace && !detail.SupportsTrace {
				continue
			}
			if req.CNIRequirement.NeedHubble && !detail.SupportsHubble {
				continue
			}
			if req.CNIRequirement.NeedDualStack && !detail.SupportsDualStack {
				continue
			}
			matched = true
			break
		}
		if !matched {
			result.Passed = false
			result.Issues = append(result.Issues, CompatibilityIssue{
				Severity: "error",
				Category: "cni",
				Message:  fmt.Sprintf("no CNI matches requirement: plugin=%s version=%s policy=%v trace=%v hubble=%v dualstack=%v",
					req.CNIRequirement.Plugin, req.CNIRequirement.VersionRange,
					req.CNIRequirement.NeedPolicy, req.CNIRequirement.NeedTrace,
					req.CNIRequirement.NeedHubble, req.CNIRequirement.NeedDualStack),
			})
		}
	}

	if req.KubeMinVersion != "" && cap.KubeVersion != "" {
		if compareVersions(cap.KubeVersion, req.KubeMinVersion) < 0 {
			result.Passed = false
			result.Issues = append(result.Issues, CompatibilityIssue{
				Severity: "error",
				Category: "kube_version",
				Message:  fmt.Sprintf("requires kube >= %s, target has %s", req.KubeMinVersion, cap.KubeVersion),
			})
		}
	}

	for _, reqFeature := range req.Features {
		if !cap.Features[reqFeature] {
			result.Passed = false
			result.Issues = append(result.Issues, CompatibilityIssue{
				Severity: "error",
				Category: "feature",
				Message:  fmt.Sprintf("requires feature %q, target does not support", reqFeature),
			})
		}
	}

	if len(result.Issues) > 0 {
		sort.Slice(result.Issues, func(i, j int) bool {
			return result.Issues[i].Category < result.Issues[j].Category
		})
	}

	return result
}

func compareVersions(a, b string) int {
	parse := func(v string) []int {
		var parts []int
		for _, s := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			var n int
			fmt.Sscanf(s, "%d", &n)
			parts = append(parts, n)
		}
		for len(parts) < 3 {
			parts = append(parts, 0)
		}
		return parts
	}

	va, vb := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if va[i] != vb[i] {
			return va[i] - vb[i]
		}
	}
	return 0
}

func matchVersionRange(version, rangeStr string) bool {
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return true
	}

	parts := strings.Fields(rangeStr)
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, ">="):
			if compareVersions(version, strings.TrimPrefix(part, ">=")) < 0 {
				return false
			}
		case strings.HasPrefix(part, "<="):
			if compareVersions(version, strings.TrimPrefix(part, "<=")) > 0 {
				return false
			}
		case strings.HasPrefix(part, ">"):
			if compareVersions(version, strings.TrimPrefix(part, ">")) <= 0 {
				return false
			}
		case strings.HasPrefix(part, "<"):
			if compareVersions(version, strings.TrimPrefix(part, "<")) >= 0 {
				return false
			}
		default:
			if compareVersions(version, part) != 0 {
				return false
			}
		}
	}
	return true
}