package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TargetState mirrors the target partition of the observation contract.
type TargetState struct {
	LifecycleState        string    `json:"lifecycleState"`
	HealthState           string    `json:"healthState"`
	ConnectivityState     string    `json:"connectivityState"`
	LastKnownStateAt      time.Time `json:"lastKnownStateAt"`
	StaleThresholdSeconds int64     `json:"staleThresholdSeconds"`
	RuntimeVersion        string    `json:"runtimeVersion,omitempty"`
}

// Capability mirrors the capability partition of the observation contract.
type Capability struct {
	SnapshotID        string   `json:"snapshotId"`
	Digest            string   `json:"digest"`
	KubernetesVersion string   `json:"kubernetesVersion,omitempty"`
	RuntimeVersion    string   `json:"runtimeVersion"`
	Architectures     []string `json:"architectures"`
	Resources         struct {
		CpuMillis   int64 `json:"cpuMillis"`
		MemoryBytes int64 `json:"memoryBytes"`
		GpuCount    int64 `json:"gpuCount,omitempty"`
		NpuCount    int64 `json:"npuCount,omitempty"`
	} `json:"resources"`
	CniPlugins []string `json:"cniPlugins,omitempty"`
	CsiDrivers []string `json:"csiDrivers,omitempty"`
}

// Node mirrors the node partition of the observation contract.
type Node struct {
	NodeID            string            `json:"nodeId"`
	Name              string            `json:"name,omitempty"`
	LifecycleState    string            `json:"lifecycleState"`
	HealthState       string            `json:"healthState"`
	ConnectivityState string            `json:"connectivityState"`
	Freshness         string            `json:"freshness"`
	ObservedAt        time.Time         `json:"observedAt"`
	LastKnownStateAt  time.Time         `json:"lastKnownStateAt"`
	Deleted           bool              `json:"deleted,omitempty"`
	RuntimeVersion    string            `json:"runtimeVersion,omitempty"`
	KubeletVersion    string            `json:"kubeletVersion,omitempty"`
	Architecture      string            `json:"architecture,omitempty"`
	Resources         map[string]int64  `json:"resources"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// KubeDiscovery reads target capability and node inventory from the local
// Kubernetes API using an in-cluster token.
type KubeDiscovery struct {
	baseURL          string
	token            string
	httpClient       *http.Client
	pageLimit        int
	maxPages         int
	maxResponseBytes int64
	requestTimeout   time.Duration
}

type kubernetesHTTPError struct {
	Path       string
	StatusCode int
	Body       string
}

func (e *kubernetesHTTPError) Error() string {
	return fmt.Sprintf("kubernetes %s returned %d: %s", e.Path, e.StatusCode, e.Body)
}

func NewKubeDiscovery(baseURL, token string) *KubeDiscovery {
	return NewKubeDiscoveryWithClient(baseURL, token, nil)
}

func NewKubeDiscoveryWithClient(baseURL, token string, client *http.Client) *KubeDiscovery {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &KubeDiscovery{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		token:            token,
		httpClient:       client,
		pageLimit:        250,
		maxPages:         40,
		maxResponseBytes: 8 << 20,
		requestTimeout:   10 * time.Second,
	}
}

type nodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			UID    string            `json:"uid"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			NodeInfo struct {
				KubeletVersion string `json:"kubeletVersion"`
				Architecture   string `json:"architecture"`
				OSImage        string `json:"osImage"`
			} `json:"nodeInfo"`
			Allocatable struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"allocatable"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	} `json:"items"`
}

// DiscoverNodes returns the current node inventory. nodeId is the target-stable
// node name; uid is preserved as a secondary identifier when available.
func (d *KubeDiscovery) DiscoverNodes(ctx context.Context, observedAt time.Time) ([]Node, error) {
	var list nodeList
	if err := d.getJSON(ctx, "/api/v1/nodes", &list); err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(list.Items))
	for _, item := range list.Items {
		id := item.Metadata.Name
		if item.Metadata.UID != "" {
			id = item.Metadata.UID
		}
		node := Node{
			NodeID:            id,
			Name:              item.Metadata.Name,
			LifecycleState:    "ACTIVE",
			HealthState:       nodeHealth(item.Status.Conditions),
			ConnectivityState: nodeConnectivity(item.Status.Conditions),
			Freshness:         "FRESH",
			ObservedAt:        observedAt,
			LastKnownStateAt:  observedAt,
			KubeletVersion:    item.Status.NodeInfo.KubeletVersion,
			Architecture:      item.Status.NodeInfo.Architecture,
			Labels:            item.Metadata.Labels,
			Resources: map[string]int64{
				"cpuMillis":   parseCPUToMillis(item.Status.Allocatable.CPU),
				"memoryBytes": parseBytes(item.Status.Allocatable.Memory),
			},
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// DiscoverCapability builds the capability snapshot from the local API server
// version and aggregated node resources.
func (d *KubeDiscovery) DiscoverCapability(ctx context.Context, nodes []Node, observedAt time.Time) (*Capability, error) {
	var version struct {
		GitVersion string `json:"gitVersion"`
	}
	if err := d.getJSON(ctx, "/version", &version); err != nil {
		return nil, err
	}
	var cpuMillis, memoryBytes int64
	archs := map[string]bool{}
	for _, node := range nodes {
		cpuMillis += node.Resources["cpuMillis"]
		memoryBytes += node.Resources["memoryBytes"]
		if node.Architecture != "" {
			archs[node.Architecture] = true
		}
	}
	cap := &Capability{
		SnapshotID:        newSnapshotID(),
		KubernetesVersion: version.GitVersion,
		RuntimeVersion:    version.GitVersion,
		Architectures:     keys(archs),
	}
	cap.Resources.CpuMillis = cpuMillis
	cap.Resources.MemoryBytes = memoryBytes
	cap.Digest = capContentDigest(cap)
	return cap, nil
}

func (d *KubeDiscovery) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+path, nil)
	if err != nil {
		return err
	}
	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kubernetes %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &kubernetesHTTPError{Path: path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	limited := io.LimitReader(resp.Body, d.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if int64(len(body)) > d.maxResponseBytes {
		return fmt.Errorf("kubernetes %s response exceeds %d bytes", path, d.maxResponseBytes)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func nodeHealth(conditions []struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}) string {
	for _, c := range conditions {
		if c.Type == "Ready" {
			if c.Status == "True" {
				return "HEALTHY"
			}
			return "UNHEALTHY"
		}
	}
	return "UNKNOWN"
}

func nodeConnectivity(conditions []struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}) string {
	for _, c := range conditions {
		if c.Type == "Ready" {
			if c.Status == "True" {
				return "CONNECTED"
			}
			return "DISCONNECTED"
		}
	}
	return "UNKNOWN"
}

func parseCPUToMillis(value string) int64 {
	if value == "" {
		return 0
	}
	if strings.HasSuffix(value, "m") {
		var millis int64
		if _, err := fmt.Sscanf(strings.TrimSuffix(value, "m"), "%d", &millis); err == nil {
			return millis
		}
	}
	var cores float64
	if _, err := fmt.Sscanf(value, "%f", &cores); err == nil {
		return int64(cores * 1000)
	}
	return 0
}

func parseBytes(value string) int64 {
	if value == "" {
		return 0
	}
	suffixes := map[string]int64{"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40}
	for suffix, mult := range suffixes {
		if strings.HasSuffix(value, suffix) {
			var n float64
			if _, err := fmt.Sscanf(strings.TrimSuffix(value, suffix), "%f", &n); err == nil {
				return int64(n * float64(mult))
			}
		}
	}
	var n int64
	if _, err := fmt.Sscanf(value, "%d", &n); err == nil {
		return n
	}
	return 0
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
