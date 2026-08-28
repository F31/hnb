package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type EdgeNodeStatus string

const (
	EdgeNodeOnline  EdgeNodeStatus = "online"
	EdgeNodeOffline EdgeNodeStatus = "offline"
	EdgeNodeUnknown EdgeNodeStatus = "unknown"
)

type EdgeNodeInfo struct {
	Name              string         `json:"name"`
	Status            EdgeNodeStatus `json:"status"`
	Architecture      string         `json:"architecture,omitempty"`
	CPUCores          int            `json:"cpu_cores,omitempty"`
	MemoryMB          int64          `json:"memory_mb,omitempty"`
	KubeletVersion    string         `json:"kubelet_version,omitempty"`
	EdgeCoreVersion   string         `json:"edgecore_version,omitempty"`
	OperatingSystem   string         `json:"operating_system,omitempty"`
	LastHeartbeatTime *time.Time     `json:"last_heartbeat_time,omitempty"`
}

type KubeEdgeVersionInfo struct {
	CloudCoreVersion string `json:"cloudcore_version"`
	EdgeCoreVersion  string `json:"edgecore_version,omitempty"`
	KubeVersion      string `json:"kube_version"`
	Platform         string `json:"platform,omitempty"`
}

type KubeEdgeDiscoveryResult struct {
	Dist        Distribution         `json:"distribution"`
	Version     *KubeEdgeVersionInfo `json:"version,omitempty"`
	Nodes       []EdgeNodeInfo       `json:"nodes,omitempty"`
	OfflineCount int                  `json:"offline_count"`
	TotalNodes   int                  `json:"total_nodes"`
	DetectedAt   time.Time            `json:"detected_at"`
}

type nodeListResponse struct {
	Items []struct {
		Metadata struct {
			Name        string            `json:"name"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Status struct {
			NodeInfo struct {
				Architecture    string `json:"architecture"`
				KubeletVersion  string `json:"kubeletVersion"`
				OperatingSystem string `json:"operatingSystem"`
			} `json:"nodeInfo"`
			Capacity map[string]string `json:"capacity"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

func DiscoverKubeEdge(serverURL string, timeout time.Duration) *KubeEdgeDiscoveryResult {
	client := &http.Client{Timeout: timeout}
	now := time.Now()

	result := &KubeEdgeDiscoveryResult{
		Dist:      DistKubeEdge,
		DetectedAt: now,
	}

	version, err := fetchK8sVersion(client, serverURL)
	if err == nil {
		result.Version = &KubeEdgeVersionInfo{
			KubeVersion: version.GitVersion,
			Platform:    version.Platform,
		}
		result.Version.CloudCoreVersion = extractCloudCoreVersion(version.GitVersion)
	}

	nodes, err := fetchEdgeNodes(client, serverURL)
	if err == nil {
		result.Nodes = nodes
		result.TotalNodes = len(nodes)
		for _, n := range nodes {
			if n.Status == EdgeNodeOffline {
				result.OfflineCount++
			}
		}
		if len(nodes) > 0 {
			ecVersion := nodes[0].EdgeCoreVersion
			for _, n := range nodes {
				if n.EdgeCoreVersion != "" && n.EdgeCoreVersion != ecVersion {
					ecVersion = ""
					break
				}
			}
			if ecVersion != "" && result.Version != nil {
				result.Version.EdgeCoreVersion = ecVersion
			}
		}
	}

	return result
}

func extractCloudCoreVersion(gitVersion string) string {
	lower := strings.ToLower(gitVersion)
	if idx := strings.Index(lower, "kubeedge"); idx >= 0 {
		rest := gitVersion[idx+8:]
		rest = strings.TrimLeft(rest, "-v")
		end := strings.IndexAny(rest, " \t\n")
		if end > 0 {
			return rest[:end]
		}
		if rest != "" {
			return rest
		}
	}
	return gitVersion
}

func fetchEdgeNodes(client *http.Client, serverURL string) ([]EdgeNodeInfo, error) {
	url := fmt.Sprintf("%s/api/v1/nodes", strings.TrimRight(serverURL, "/"))
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch nodes: %w", err)
	}
	defer resp.Body.Close()

	var nodeList nodeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&nodeList); err != nil {
		return nil, fmt.Errorf("decode nodes: %w", err)
	}

	var nodes []EdgeNodeInfo
	for _, item := range nodeList.Items {
		if !isEdgeNode(item.Metadata.Labels) {
			continue
		}

		info := EdgeNodeInfo{
			Name:            item.Metadata.Name,
			Architecture:    item.Status.NodeInfo.Architecture,
			KubeletVersion:  item.Status.NodeInfo.KubeletVersion,
			OperatingSystem: item.Status.NodeInfo.OperatingSystem,
		}

		if cpuStr, ok := item.Status.Capacity["cpu"]; ok {
			fmt.Sscanf(cpuStr, "%d", &info.CPUCores)
		}
		if memStr, ok := item.Status.Capacity["memory"]; ok {
			var memKB int64
			if n, _ := fmt.Sscanf(memStr, "%dKi", &memKB); n == 1 {
				info.MemoryMB = memKB / 1024
			}
		}

		info.Status = getNodeCondition(item.Status.Conditions)
		info.EdgeCoreVersion = extractEdgeCoreVersion(item.Metadata.Annotations)

		nodes = append(nodes, info)
	}

	return nodes, nil
}

func isEdgeNode(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	if _, ok := labels["node-role.kubernetes.io/edge"]; ok {
		return true
	}
	return false
}

func getNodeCondition(conditions []struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}) EdgeNodeStatus {
	for _, c := range conditions {
		if c.Type == "Ready" {
			switch c.Status {
			case "True":
				return EdgeNodeOnline
			case "False":
				return EdgeNodeOffline
			default:
				return EdgeNodeUnknown
			}
		}
	}
	return EdgeNodeUnknown
}

func extractEdgeCoreVersion(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	for k, v := range annotations {
		if strings.Contains(k, "kubeedge") || strings.Contains(k, "edgecore") {
			return v
		}
	}
	return ""
}