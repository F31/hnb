package provider

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

type NetworkPolicy struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Spec        map[string]any    `json:"spec"`
}

type EnvoyConfig struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Spec        map[string]any    `json:"spec"`
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