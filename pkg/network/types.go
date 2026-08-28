package network

import "context"

type NetworkProvider interface {
	Name() string
	Install(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error
	Uninstall(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error
	Upgrade(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget, version string) error
	Capability() NetworkCapability
	Health(ctx context.Context, target *RuntimeTarget) error
}

type NetworkPolicyManager interface {
	ApplyNetworkPolicy(ctx context.Context, policy *NetworkPolicy, target *RuntimeTarget) error
	DeleteNetworkPolicy(ctx context.Context, name, namespace string, target *RuntimeTarget) error
}

type ClusterwidePolicyManager interface {
	ApplyClusterwidePolicy(ctx context.Context, policy *NetworkPolicy, target *RuntimeTarget) error
	DeleteClusterwidePolicy(ctx context.Context, name string, target *RuntimeTarget) error
}

type EnvoyConfigManager interface {
	ApplyEnvoyConfig(ctx context.Context, config *EnvoyConfig, target *RuntimeTarget) error
	DeleteEnvoyConfig(ctx context.Context, name, namespace string, target *RuntimeTarget) error
}

type PolicyTracer interface {
	PolicyTrace(ctx context.Context, trace *PolicyTraceRequest, target *RuntimeTarget) (*PolicyTraceResult, error)
}

type TenantIsolationManager interface {
	ApplyTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace, tenantID, workspaceID string) error
	RemoveTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace string) error
	ApplyCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error
	RemoveCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error
}

type NetworkPolicy struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Spec      map[string]any    `json:"spec"`
}

type EnvoyConfig struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Spec      map[string]any    `json:"spec"`
}

type PolicyTraceRequest struct {
	Source      map[string]string `json:"source,omitempty"`
	Destination map[string]string `json:"destination,omitempty"`
	Direction   string            `json:"direction"`
	Verbose     bool              `json:"verbose"`
}

type PolicyTraceResult struct {
	Verdict string `json:"verdict"`
	Log     string `json:"log,omitempty"`
}

type NetworkCapability struct {
	ProviderName       string   `json:"provider_name"`
	SupportsPolicy     bool     `json:"supports_policy"`
	SupportsEncryption bool     `json:"supports_encryption"`
	EncryptionType     string   `json:"encryption_type,omitempty"`
	SupportsDualStack  bool     `json:"supports_dual_stack"`
	SupportsEgress     bool     `json:"supports_egress"`
	SupportsIngress    bool     `json:"supports_ingress"`
	SupportsHubble     bool     `json:"supports_hubble"`
	SupportedModes     []string `json:"supported_modes,omitempty"`
	SupportedIPAMModes []string `json:"supported_ipam_modes,omitempty"`
}

type RuntimeTarget struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Name         string `json:"name"`
	Kubeconfig   string `json:"kubeconfig,omitempty"`
	APIServerURL string `json:"api_server_url,omitempty"`
	Distribution string `json:"distribution,omitempty"`
	KubeVersion  string `json:"kube_version,omitempty"`
}

type NetworkProfile struct {
	ID                   string           `json:"id"`
	TenantID             string           `json:"tenant_id"`
	TargetID             string           `json:"target_id"`
	Provider             string           `json:"provider"`
	IPVersion            string           `json:"ip_version"`
	PodCIDR              string           `json:"pod_cidr"`
	ServiceCIDR          string           `json:"service_cidr"`
	IPv6PodCIDR          string           `json:"ipv6_pod_cidr,omitempty"`
	IPv6ServiceCIDR      string           `json:"ipv6_service_cidr,omitempty"`
	EncapMode            string           `json:"encap_mode"`
	RoutingMode          string           `json:"routing_mode"`
	MTU                  int              `json:"mtu"`
	EnablePolicy         bool             `json:"enable_policy"`
	EnableHubble         bool             `json:"enable_hubble"`
	HubbleRelay          bool             `json:"hubble_relay,omitempty"`
	HubbleUI             bool             `json:"hubble_ui,omitempty"`
	HubbleMetrics        []string         `json:"hubble_metrics,omitempty"`
	EnableOTel           bool             `json:"enable_otel,omitempty"`
	OTelTarget           string           `json:"otel_target,omitempty"`
	EnableClusterMesh    bool             `json:"enable_clustermesh,omitempty"`
	ClusterMeshID        int              `json:"clustermesh_id,omitempty"`
	ClusterMeshName      string           `json:"clustermesh_name,omitempty"`
	ClusterMeshPeers     []ClusterMeshPeer `json:"clustermesh_peers,omitempty"`
	KubeProxyReplacement string           `json:"kube_proxy_replacement"`
	IPAMMode             string           `json:"ipam_mode"`
	Version              string           `json:"version"`
	ExtraConfig          map[string]any   `json:"extra_config,omitempty"`
}

type ClusterMeshPeer struct {
	ClusterID   int    `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Endpoint    string `json:"endpoint"`
}

type NetworkRequestMessage struct {
	OperationID string `json:"operation_id"`
	Action      string `json:"action"`
	Provider    string `json:"provider"`
	ProfileJSON string `json:"profile_json,omitempty"`
	PolicyJSON  string `json:"policy_json,omitempty"`
	PolicyName  string `json:"policy_name,omitempty"`
	PolicyNS    string `json:"policy_namespace,omitempty"`
	TraceJSON   string `json:"trace_json,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	Version     string `json:"version,omitempty"`
}

type NetworkResultMessage struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Provider    string `json:"provider"`
	Error       string `json:"error,omitempty"`
	Message     string `json:"message,omitempty"`
	TraceResult string `json:"trace_result,omitempty"`
}