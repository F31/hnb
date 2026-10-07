// Package provision implements platform-side self-hosted cluster creation
// ("真实装机") for Kubernetes targets, following the Rancher/KubeSphere model.
//
// The bootstrap is CAPI-aligned: a control-plane node runs kubeadm init (with a
// generated kubeadm ClusterConfiguration), worker nodes kubeadm join, and the
// resulting admin kubeconfig is stored into the platform secret store for the
// registered RuntimeTarget. For environments where a CAPI management cluster is
// reachable (MANAGEMENT_KUBECONFIG), CAPI v1beta1 manifests (Cluster,
// KubeadmControlPlane, KubeadmConfigTemplate, MachineDeployment + SSHMachine
// infra objects) are also rendered/applied so cluster state is reconcilable the
// CAPI way.
package provision

import "fmt"

const (
	// DefaultSSHPort is used when a node does not specify one.
	DefaultSSHPort = 22
	// DefaultSSHUser is used when a node does not specify one.
	DefaultSSHUser = "root"
	// kubeadmAPIVersion is the ClusterConfiguration API version this provider
	// renders (kubeadm v1beta4).
	kubeadmAPIVersion = "kubeadm.k8s.io/v1beta4"
	// adminKubeconfigPath is where kubeadm writes the admin kubeconfig on the
	// control-plane node.
	adminKubeconfigPath = "/etc/kubernetes/admin.conf"
	// bindPort is the kube-apiserver secure port we target for joins.
	bindPort = 6443
)

// NodeRole is the bootstrap role of a node.
type NodeRole string

const (
	RoleControlPlane NodeRole = "control-plane"
	RoleWorker       NodeRole = "worker"
)

// NodeSpec describes a user-supplied node the platform bootstraps over SSH.
type NodeSpec struct {
	// ID is a stable unique identifier within the request (used for ordering
	// and CLI/table display); not a Kubernetes object name.
	ID string `json:"id"`
	// Host is the IP or resolvable hostname reachable from the provider.
	Host string `json:"host"`
	// Port is the SSH port; 0 means 22.
	Port int `json:"port,omitempty"`
	// User is the SSH login user; empty means root.
	User string `json:"user,omitempty"`
	// Role must be "control-plane" or "worker".
	Role NodeRole `json:"role"`
	// Labels are applied to the kubelet node (optional).
	Labels map[string]string `json:"labels,omitempty"`
}

// EffectivePort returns the SSH port with defaults applied.
func (n NodeSpec) EffectivePort() int {
	if n.Port <= 0 {
		return DefaultSSHPort
	}
	return n.Port
}

// EffectiveUser returns the SSH user with defaults applied.
func (n NodeSpec) EffectiveUser() string {
	if n.User == "" {
		return DefaultSSHUser
	}
	return n.User
}

// SecretReference identifies an encrypted value in the platform secret store,
// matching the provider-level SecretReference contract.
type SecretReference struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

func (s SecretReference) String() string {
	return fmt.Sprintf("%s/%s/%s@%s", s.Provider, s.Scope, s.Name, s.Version)
}

// ProvisionSpec is the resolved input for a self-hosted cluster creation.
type ProvisionSpec struct {
	TenantID       string
	TargetID       string
	DisplayName    string
	DesiredVersion string
	ControlPlaneEndpoint string // optional override; defaults to the first CP node
	PodCIDR        string
	ServiceCIDR    string
	ClusterDomain  string
	IdempotencyKey string
	Nodes          []NodeSpec
	// SSHCredential is the encrypted SSH private key SecretReference.
	SSHCredential SecretReference
}

// ControlPlaneNodes returns nodes with role control-plane, in declaration order.
func (p ProvisionSpec) ControlPlaneNodes() []NodeSpec {
	var out []NodeSpec
	for _, n := range p.Nodes {
		if n.Role == RoleControlPlane {
			out = append(out, n)
		}
	}
	return out
}

// WorkerNodes returns nodes with role worker, in declaration order.
func (p ProvisionSpec) WorkerNodes() []NodeSpec {
	var out []NodeSpec
	for _, n := range p.Nodes {
		if n.Role == RoleWorker {
			out = append(out, n)
		}
	}
	return out
}

// Endpoint returns the load-balancer/API endpoint used for joins and kubeconfig.
func (p ProvisionSpec) Endpoint() string {
	if p.ControlPlaneEndpoint != "" {
		return p.ControlPlaneEndpoint
	}
	if cp := p.ControlPlaneNodes(); len(cp) > 0 {
		return fmt.Sprintf("%s:%d", cp[0].Host, bindPort)
	}
	return ""
}

// Result describes a completed bootstrap.
type Result struct {
	ControlPlaneEndpoint string
	NodeCount            int
	// KubeconfigRef is where the admin kubeconfig was stored.
	KubeconfigRef SecretReference
}

// StepError wraps a step name with the underlying failure so the provider can
// surface which stage of provisioning failed.
type StepError struct {
	Step string
	Err  error
}

func (e *StepError) Error() string {
	if e == nil || e.Err == nil {
		return "provision step failed"
	}
	return fmt.Sprintf("%s: %v", e.Step, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }

func stepErr(step string, err error) error {
	return &StepError{Step: step, Err: err}
}