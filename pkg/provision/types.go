// Package provision implements platform-side self-hosted cluster creation
// ("真实装机") for Kubernetes targets, following the Rancher/KubeSphere model.
//
// The SSH bootstrap is the lifecycle owner: a control-plane node runs kubeadm init (with a
// generated kubeadm ClusterConfiguration), worker nodes kubeadm join, and the
// resulting admin kubeconfig is stored into the platform secret store for the
// registered RuntimeTarget. CAPI manifest rendering is separate; this SSH
// provisioner does not apply them or delegate ownership to CAPI controllers.
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
	TenantID             string
	TargetID             string
	DisplayName          string
	DesiredVersion       string
	ControlPlaneEndpoint string // optional override; defaults to the first CP node
	PodCIDR              string
	ServiceCIDR          string
	ClusterDomain        string
	// NetworkPlugin defaults to flannel. Cilium uses a preinstalled Helm CLI.
	NetworkPlugin   string
	NetworkVersion  string
	ImageRepository string
	SandboxImage    string
	// CNIImageRepository overrides the CNI data-path/operator image registry.
	// Bootstrap must not reach an upstream public registry implicitly: the
	// platform resolves every node image through this repository, which is
	// backed by the platform OCI registry (Harbor).
	CNIImageRepository string
	// HelmChartRepository overrides the Cilium chart source. It must point at
	// the platform-controlled chart mirror for air-gapped installs; the public
	// upstream is never assumed.
	HelmChartRepository string
	AdvertiseAddress    string // optional node-local IP behind a public SSH address
	IdempotencyKey      string
	Nodes               []NodeSpec
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

// AddNodesSpec is the resolved input for joining new worker nodes to an
// existing toolkit-provisioned cluster. The node identity doubles as the SSH
// address and the kubelet node name; the control plane is reached at
// ControlPlaneEndpoint with the same SSH credential.
type AddNodesSpec struct {
	TenantID             string
	TargetID             string
	ControlPlaneEndpoint string // host:6443 of the provisioned control plane
	DesiredVersion       string // exact Kubernetes version new nodes must match
	IdempotencyKey       string
	Nodes                []NodeSpec // new worker nodes only (ID == Host == node name)
	SSHCredential        SecretReference
}

// AddNodesResult reports how the requested nodes were joined. An already
// joined node is replay evidence, not a failure.
type AddNodesResult struct {
	Endpoint string
	// Requested is the number of nodes in the request.
	Requested int
	// Joined is the number of nodes that performed a first-time join.
	Joined int
	// AlreadyJoined counts nodes that were already cluster members (replay).
	AlreadyJoined int
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
