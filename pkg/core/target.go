package core

import (
	"fmt"
	"time"
)

type TargetType string

const (
	TargetKubernetes      TargetType = "kubernetes"
	TargetContainerEngine TargetType = "container_engine"
	TargetEdgeRuntime     TargetType = "edge_runtime"
	TargetExternalService TargetType = "external_service"
)

type Distribution string

const (
	DistStandard Distribution = "standard"
	DistK3s      Distribution = "k3s"
	DistKubeEdge Distribution = "kubeedge"
	DistOther    Distribution = "other"
)

func ParseDistribution(s string) Distribution {
	switch s {
	case "k3s":
		return DistK3s
	case "kubeedge":
		return DistKubeEdge
	case "other":
		return DistOther
	default:
		return DistStandard
	}
}

type ConnectionType string

const (
	ConnAgent    ConnectionType = "agent"
	ConnDirect   ConnectionType = "direct"
	ConnCloudHub ConnectionType = "cloudhub"
)

type ProviderPhase string

const (
	PhasePending      ProviderPhase = "pending"
	PhaseInstalling   ProviderPhase = "installing"
	PhaseReady        ProviderPhase = "ready"
	PhaseDegraded     ProviderPhase = "degraded"
	PhaseUninstalling ProviderPhase = "uninstalling"
)

type ProviderBinding struct {
	ID             string        `json:"id"`
	ClusterID      string        `json:"cluster_id"`
	Provider       string        `json:"provider"`
	Version        string        `json:"version"`
	Phase          ProviderPhase `json:"phase"`
	RefCount       int           `json:"ref_count"`
	HealthFailures int           `json:"health_failures"`
	LastHealthAt   *time.Time    `json:"last_health_at,omitempty"`
	LastError      string        `json:"last_error,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type Quota struct {
	CPU     int64 `json:"cpu,omitempty"`
	Memory  int64 `json:"memory,omitempty"`
	Storage int64 `json:"storage,omitempty"`
	VGPU    int64 `json:"vgpu,omitempty"`
	VRAM    int64 `json:"vram,omitempty"`
	GPU     int64 `json:"gpu,omitempty"`
}

type Workspace struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	TenantID    string            `json:"tenant_id"`
	Labels      map[string]string `json:"labels,omitempty"`
	Quota       Quota             `json:"quota,omitempty"`
	IsActive    bool              `json:"is_active"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Namespace struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	ClusterID   string            `json:"cluster_id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Quota       Quota             `json:"quota,omitempty"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type ExtensionPhase string

const (
	ExtPending      ExtensionPhase = "pending"
	ExtInstalling   ExtensionPhase = "installing"
	ExtReady        ExtensionPhase = "ready"
	ExtDegraded     ExtensionPhase = "degraded"
	ExtUninstalling ExtensionPhase = "uninstalling"
)

type Extension struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	ProviderType   ProviderType      `json:"provider_type"`
	WorkspaceID    string            `json:"workspace_id,omitempty"`
	TargetID       string            `json:"target_id,omitempty"`
	Phase          ExtensionPhase    `json:"phase"`
	Manifest       ExtensionManifest `json:"manifest"`
	Labels         map[string]string `json:"labels,omitempty"`
	HealthFailures int               `json:"health_failures"`
	LastError      string            `json:"last_error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type ExtensionManifest struct {
	Name                 string                `json:"name"`
	Version              string                `json:"version"`
	Provider             string                `json:"provider"`
	Description          string                `json:"description,omitempty"`
	Dependencies         []string              `json:"dependencies,omitempty"`
	Capabilities         []string              `json:"capabilities,omitempty"`
	Config               map[string]any        `json:"config,omitempty"`
	StorageDriverPackage *StorageDriverPackage `json:"storageDriverPackage,omitempty"`
}

func (m *ExtensionManifest) Validate() error {
	if m.StorageDriverPackage == nil {
		return nil
	}
	if m.Name == "" || m.Provider == "" || !semverPattern.MatchString(m.Version) {
		return fmt.Errorf("extension manifest name, provider, and semantic version are required")
	}
	if err := m.StorageDriverPackage.Validate(m.Version, time.Now()); err != nil {
		return fmt.Errorf("storageDriverPackage: %w", err)
	}
	return nil
}

type EdgeType string

const (
	EdgeKubeEdge  EdgeType = "kubeedge"
	EdgeOpenYurt  EdgeType = "openyurt"
	EdgeSuperEdge EdgeType = "superedge"
	EdgeCustom    EdgeType = "custom"
)

type TargetStatus string

const (
	StatusOnline         TargetStatus = "online"
	StatusOffline        TargetStatus = "offline"
	StatusUnknown        TargetStatus = "unknown"
	StatusDegraded       TargetStatus = "degraded"
	StatusDecommissioned TargetStatus = "decommissioned"
)

type RuntimeTarget struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id,omitempty"`
	WorkspaceID        string            `json:"workspace_id,omitempty"`
	Name               string            `json:"name"`
	DisplayName        string            `json:"display_name,omitempty"`
	TargetType         TargetType        `json:"target_type"`
	Distribution       Distribution      `json:"distribution"`
	EdgeType           EdgeType          `json:"edge_type,omitempty"`
	EdgeConfig         map[string]any    `json:"edge_config,omitempty"`
	ConnectionType     ConnectionType    `json:"connection_type"`
	ConnectionEndpoint string            `json:"connection_endpoint,omitempty"`
	AgentVersion       string            `json:"agent_version,omitempty"`
	Status             TargetStatus      `json:"status"`
	Labels             map[string]string `json:"labels"`
	ObservedAt         *time.Time        `json:"observed_at,omitempty"`
	StaleThresholdSec  int               `json:"stale_threshold_seconds"`
	IsActive           bool              `json:"is_active"`
	CreatedAt          time.Time         `json:"created_at"`
	Shares             []ClusterShare    `json:"shares,omitempty"`
}

type ClusterShare struct {
	ID                     string   `json:"id"`
	ClusterID              string   `json:"cluster_id"`
	GranteeTenantID        string   `json:"grantee_tenant_id"`
	GranteeWorkspaceID     string   `json:"grantee_workspace_id,omitempty"`
	Permissions            []string `json:"permissions"`
	K8sNamespacePrefix     string   `json:"k8s_namespace_prefix"`
	TenantIsolationEnabled bool     `json:"tenant_isolation_enabled"`
	CreatedAt              string   `json:"created_at,omitempty"`
	CreatedBySubjectID     string   `json:"created_by_subject_id,omitempty"`
}

// TenantClusterAllocation is the hard, cluster-local quota granted to a
// tenant in a shared cluster. A tenant's platform quota is the sum of its
// active allocations; admission must also enforce the selected allocation.
// WorkspaceID is deliberately absent: workspaces are optional organisation
// metadata, not the tenancy or capacity boundary.
type TenantClusterAllocation struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ClusterID        string    `json:"cluster_id"`
	Quota            Quota     `json:"quota"`
	Status           string    `json:"status"`
	NamespacePrefix  string    `json:"namespace_prefix"`
	IsolationEnabled bool      `json:"isolation_enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TenantIsolationPolicy struct {
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	TenantID    string         `json:"tenant_id"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	Namespace   string         `json:"namespace"`
	Spec        map[string]any `json:"spec"`
}

func (t *RuntimeTarget) IsDeployable() bool {
	if !t.IsActive {
		return false
	}
	switch t.TargetType {
	case TargetKubernetes, TargetContainerEngine, TargetEdgeRuntime:
		return true
	case TargetExternalService:
		return false
	}
	return false
}

func (t *RuntimeTarget) IsStale() bool {
	if t.ObservedAt == nil {
		return true
	}
	threshold := time.Duration(t.StaleThresholdSec) * time.Second
	return time.Since(*t.ObservedAt) > threshold
}

type CapabilitySnapshot struct {
	ID                string          `json:"id"`
	TargetID          string          `json:"target_id"`
	KubeVersion       string          `json:"kube_version,omitempty"`
	Arch              string          `json:"arch,omitempty"`
	CPUCores          int             `json:"cpu_cores"`
	CPUModel          string          `json:"cpu_model,omitempty"`
	MemoryMB          int64           `json:"memory_mb"`
	StorageGB         int64           `json:"storage_gb"`
	CNIPlugins        []string        `json:"cni_plugins,omitempty"`
	CNIDetails        []CNICapability `json:"cni_details,omitempty"`
	CSIDrivers        []string        `json:"csi_drivers,omitempty"`
	GatewayAPIVersion string          `json:"gateway_api_version,omitempty"`
	GPUModel          string          `json:"gpu_model,omitempty"`
	GPUMemoryMB       int64           `json:"gpu_memory_mb"`
	GPUCount          int             `json:"gpu_count"`
	Features          map[string]bool `json:"features"`
	ObservedAt        time.Time       `json:"observed_at"`
}

type CNICapability struct {
	Plugin            string `json:"plugin"`
	Version           string `json:"version,omitempty"`
	SupportsPolicy    bool   `json:"supports_policy"`
	SupportsTrace     bool   `json:"supports_trace"`
	SupportsHubble    bool   `json:"supports_hubble"`
	SupportsIngress   bool   `json:"supports_ingress"`
	SupportsDualStack bool   `json:"supports_dual_stack"`
}

type ResourceRequirement struct {
	MinMemoryMB    int64           `json:"min_memory_mb"`
	MinStorageGB   int64           `json:"min_storage_gb"`
	MinCPUCores    int             `json:"min_cpu_cores"`
	RequiresGPU    bool            `json:"requires_gpu"`
	CNIRequired    string          `json:"cni_required,omitempty"`
	CNIRequirement *CNIRequirement `json:"cni_requirement,omitempty"`
	GatewayAPI     string          `json:"gateway_api,omitempty"`
	KubeMinVersion string          `json:"kube_min_version,omitempty"`
	Features       []string        `json:"features,omitempty"`
}

type CNIRequirement struct {
	Plugin        string `json:"plugin"`
	VersionRange  string `json:"version_range,omitempty"`
	NeedPolicy    bool   `json:"need_policy,omitempty"`
	NeedTrace     bool   `json:"need_trace,omitempty"`
	NeedHubble    bool   `json:"need_hubble,omitempty"`
	NeedDualStack bool   `json:"need_dual_stack,omitempty"`
}

type DistributionNotes struct {
	Distribution Distribution      `json:"distribution"`
	Label        string            `json:"label"`
	KnownLimits  []string          `json:"known_limits"`
	BuiltIn      map[string]bool   `json:"built_in"`
	Capabilities map[string]string `json:"capabilities"`
}

func GetDistributionNotes(dist Distribution) *DistributionNotes {
	switch dist {
	case DistK3s:
		return &DistributionNotes{
			Distribution: dist,
			Label:        "K3s - Lightweight Kubernetes",
			KnownLimits: []string{
				"Uses SQLite/embedded etcd instead of standard etcd (no multi-node HA by default)",
				"Built-in Traefik Ingress Controller (optional)",
				"Built-in local-path-provisioner StorageClass",
				"Limited to single-node control plane in default mode",
				"Some advanced Kubernetes features may not be available (e.g. certain Admission Webhooks)",
			},
			BuiltIn: map[string]bool{
				"traefik_ingress":    true,
				"local_path_storage": true,
				"metrics_server":     true,
				"servicelb":          true,
			},
			Capabilities: map[string]string{
				"etcd":         "sqlite (default) / embedded etcd (HA mode)",
				"ingress":      "traefik (built-in, optional)",
				"storage":      "local-path-provisioner (built-in, optional)",
				"loadbalancer": "servicelb (built-in, optional)",
				"cni":          "flannel (built-in) / cilium (replaceable)",
			},
		}
	case DistKubeEdge:
		return &DistributionNotes{
			Distribution: dist,
			Label:        "KubeEdge - Edge Computing Platform",
			KnownLimits: []string{
				"Edge nodes do not run standard kubelet",
				"Edge nodes run EdgeCore connected via CloudHub-EdgeHub tunnel",
				"Standard Kubernetes API available through CloudCore",
				"EdgeApplication CRD required for edge deployments",
				"Offline autonomy supported via EdgeStore (Lite Data Store)",
				"Device management via DeviceModel/Device CRDs",
			},
			BuiltIn: map[string]bool{
				"edge_autonomy":     true,
				"device_management": true,
				"cloudhub_tunnel":   true,
				"edge_mesh":         true,
			},
			Capabilities: map[string]string{
				"node_management":    "cloudcore (central) + edgecore (edge)",
				"offline_mode":       "supported via edged/edgestore",
				"device_integration": "device mapper framework",
				"service_mesh":       "edgemesh (optional)",
				"cni":                "flannel / cilium (optional)",
			},
		}
	default:
		return &DistributionNotes{
			Distribution: dist,
			Label:        "Standard Kubernetes",
			KnownLimits:  nil,
			BuiltIn: map[string]bool{
				"cilium_support": true,
			},
			Capabilities: map[string]string{
				"cni": "cilium (recommended) / calico / flannel / weave",
			},
		}
	}
}
