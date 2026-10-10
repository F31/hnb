package provision

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// capiWaitTimeout bounds how long Provision waits for the CAPI control plane
// to report Ready after applying manifests to a management cluster.

// SecretStore mirrors the provider's secret persistence surface so the
// orchestrator can store the resulting admin kubeconfig for the new target.
type SecretStore interface {
	// StoreKubeconfig persists an encrypted kubeconfig (purpose=kubeconfig,
	// allowed provider = runtime-target.lifecycle.kubernetes) and returns the
	// resulting SecretReference.
	StoreKubeconfig(ctx context.Context, tenantID, scope, name string, kubeconfig []byte) (SecretReference, error)
}

// KeyResolver resolves the encrypted SSH private key SecretReference for a
// tenant into plaintext PEM bytes.
type KeyResolver interface {
	ResolveSSHKey(ctx context.Context, tenantID string, ref SecretReference) ([]byte, error)
	ValidateVersion(ctx context.Context, version string) error
}

// NodeBootstrapper owns the SSH-side bootstrap sequence plus the secret
// resolution/store bridges. Split behind interfaces so the orchestrator is
// testable with fake runners/resolvers.
type NodeBootstrapper struct {
	SSH   SSHRunner
	Key   KeyResolver
	Store SecretStore
}

// Option composes a provisioner.
type Option func(*Provisioner)

// WithBootstrapper overrides the default node bootstrap driver (used in tests).
func WithBootstrapper(b *NodeBootstrapper) Option {
	return func(p *Provisioner) { p.bootstrap = b }
}

// WithManagementClient attaches a CAPI management cluster client. When set,
// Provision applies the rendered CAPI manifests and waits for the control
// plane before finishing; without it, manifests are only generated/audited.
func WithManagementClient(client ManagementClient) Option {
	return func(p *Provisioner) { p.management = client }
}

// Provisioner drives a self-hosted kubeadm/CAPI-aligned cluster bootstrap from
// user-supplied SSH nodes.
type Provisioner struct {
	bootstrap *NodeBootstrapper
	// management is the optional CAPI management-cluster client.
	management ManagementClient
	// cniManifest overrides the default pod-network manifest (optional).
	cniManifest string
	// clusterName is the CAPI object name prefix (default: shortID(target)).
	clusterName string
}

// New provisioner with sensible defaults.
func New(opts ...Option) *Provisioner {
	p := &Provisioner{
		bootstrap: &NodeBootstrapper{SSH: &RealSSH{}},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Plan returns the ordered bootstrap stages for a spec (used by callers that
// want to render a plan/approval before executing).
func (p *Provisioner) Plan(spec ProvisionSpec) []string {
	stages := []string{"prereq"}
	cpCount := len(spec.ControlPlaneNodes())
	if cpCount == 0 {
		stages = append(stages, "kubeadm-init(failed: no control-plane node)", "join")
	} else {
		stages = append(stages, "kubeadm-init", "cni")
		if len(spec.WorkerNodes()) > 0 {
			stages = append(stages, "join")
		}
	}
	return stages
}

// Provision runs the whole self-hosted bootstrap and returns the resulting
// control-plane endpoint, node count, and stored kubeconfig reference.
func (p *Provisioner) Provision(ctx context.Context, spec ProvisionSpec) (Result, error) {
	// Direct SSH bootstrap must never be followed by a second lifecycle owner.
	if p.management != nil {
		return Result{}, stepErr("strategy", fmt.Errorf("CAPI provisioning is not implemented; cannot combine CAPI reconciliation with SSH bootstrap"))
	}
	if p.bootstrap == nil || p.bootstrap.SSH == nil || p.bootstrap.Store == nil {
		return Result{}, stepErr("configuration", fmt.Errorf("SSH runner and secret store are required"))
	}
	cpNodes := spec.ControlPlaneNodes()
	if len(cpNodes) == 0 {
		return Result{}, stepErr("plan", fmt.Errorf("provision requires at least one control-plane node"))
	}
	if len(cpNodes) != 1 {
		return Result{}, stepErr("plan", fmt.Errorf("SSH bootstrap currently supports exactly one control-plane node; HA is not accepted"))
	}
	if err := validateSSHSpec(spec); err != nil {
		return Result{}, stepErr("plan", err)
	}
	key, err := p.sshKey(ctx, spec)
	if err != nil {
		return Result{}, err
	}

	// 1. prereq + kubeadm init on the first control-plane node.
	firstCP := cpNodes[0]
	if _, err := p.run(ctx, firstCP, key, PrereqScript(spec)); err != nil {
		return Result{}, stepErr("prereq", err)
	}
	initOut, err := p.run(ctx, firstCP, key, InitScript(spec, firstCP))
	if err != nil {
		return Result{}, stepErr("kubeadm-init", fmt.Errorf("%w: %s", err, tail(initOut)))
	}
	token, caHash, err := ParseJoinDetails(initOut)
	if err != nil {
		return Result{}, stepErr("parse-join", err)
	}

	// 2. additional control-plane nodes join (HA path; v1 documents single
	// control-plane, extra CP nodes join with --control-plane).
	for _, n := range cpNodes[1:] {
		if _, err := p.run(ctx, n, key, JoinScript(spec, n, token, caHash)); err != nil {
			return Result{}, stepErr("join-control-plane:"+n.Host, err)
		}
	}

	// 3. worker nodes join.
	for _, n := range spec.WorkerNodes() {
		if _, err := p.run(ctx, n, key, PrereqScript(spec)); err != nil {
			return Result{}, stepErr("prereq-worker:"+n.Host, err)
		}
		if _, err := p.run(ctx, n, key, JoinScript(spec, n, token, caHash)); err != nil {
			return Result{}, stepErr("join-worker:"+n.Host, err)
		}
	}

	// 4. install pod network (best-effort, on control plane).
	if _, err := p.run(ctx, firstCP, key, CNIScript(p.cniManifest)); err != nil {
		return Result{}, stepErr("cni", err)
	}

	// Observe the real workload cluster before publishing credentials/success.
	accepted, err := p.run(ctx, firstCP, key, AcceptanceScript(spec))
	if err != nil {
		return Result{}, stepErr("cluster-acceptance", err)
	}
	if !strings.Contains(accepted, "__HNB_CLUSTER_ACCEPTED__") {
		return Result{}, stepErr("cluster-acceptance", fmt.Errorf("readiness evidence missing"))
	}

	// 5. read the admin kubeconfig from the control-plane node.
	kubeconfig, err := p.readKubeconfig(ctx, firstCP, key, spec.Endpoint())
	if err != nil {
		return Result{}, err
	}

	// 6. store it as the target credential.
	scope := "tenant"
	name := "provision-" + shortID(spec.TargetID)
	ref, err := p.bootstrap.Store.StoreKubeconfig(ctx, spec.TenantID, scope, name, kubeconfig)
	if err != nil {
		return Result{}, stepErr("store-kubeconfig", err)
	}

	return Result{
		ControlPlaneEndpoint: spec.Endpoint(),
		NodeCount:            len(spec.Nodes),
		KubeconfigRef:        ref,
	}, nil
}

func validateSSHSpec(spec ProvisionSpec) error {
	if !regexp.MustCompile(`^v?1\.[0-9]+\.[0-9]+$`).MatchString(spec.DesiredVersion) {
		return fmt.Errorf("an exact Kubernetes patch version is required")
	}
	seen := map[string]bool{}
	for _, node := range spec.Nodes {
		if node.EffectiveUser() != "root" {
			return fmt.Errorf("SSH bootstrap currently requires root; sudo execution is not implemented")
		}
		if net.ParseIP(node.Host) == nil || seen[node.Host] {
			return fmt.Errorf("node host must be a unique IP address")
		}
		seen[node.Host] = true
		if node.Role != RoleControlPlane && node.Role != RoleWorker {
			return fmt.Errorf("unknown node role")
		}
		if !regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`).MatchString(node.EffectiveUser()) || node.EffectivePort() < 1 || node.EffectivePort() > 65535 {
			return fmt.Errorf("invalid SSH user or port")
		}
		if node.ID != "" && !regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`).MatchString(node.ID) {
			return fmt.Errorf("invalid node ID")
		}
	}
	for _, cidr := range []string{spec.PodCIDR, spec.ServiceCIDR} {
		if cidr != "" {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid network CIDR")
			}
		}
	}
	if spec.ClusterDomain != "" && !regexp.MustCompile(`^[a-z0-9.-]+$`).MatchString(spec.ClusterDomain) {
		return fmt.Errorf("invalid cluster domain")
	}
	return nil
}

func (p *Provisioner) sshKey(ctx context.Context, spec ProvisionSpec) ([]byte, error) {
	if p.bootstrap.Key == nil {
		return nil, stepErr("ssh-key", fmt.Errorf("ssh key resolver not configured"))
	}
	if err := p.bootstrap.Key.ValidateVersion(ctx, spec.DesiredVersion); err != nil {
		return nil, stepErr("version-evidence", err)
	}
	key, err := p.bootstrap.Key.ResolveSSHKey(ctx, spec.TenantID, spec.SSHCredential)
	if err != nil {
		return nil, stepErr("ssh-key", err)
	}
	return key, nil
}

func (p *Provisioner) run(ctx context.Context, node NodeSpec, key []byte, script string) (string, error) {
	return p.bootstrap.SSH.Run(ctx, node, key, script)
}

func (p *Provisioner) readKubeconfig(ctx context.Context, cp NodeSpec, key []byte, endpoint string) ([]byte, error) {
	out, err := p.run(ctx, cp, key, AdminKubeconfigScript)
	if err != nil {
		return nil, stepErr("read-kubeconfig", err)
	}
	if strings.Contains(out, "__NO_ADMIN_CONF__") {
		return nil, stepErr("read-kubeconfig", fmt.Errorf("admin.conf not present on control-plane node"))
	}
	// Rewrite the embedded server to the provider-routable endpoint so
	// downloads/agents connect to the reachable apiserver.
	return rewriteKubeconfigServer([]byte(out), endpoint)
}

// rewriteKubeconfigServer replaces the server target so the stored kubeconfig
// points at the exposed control-plane endpoint.
func rewriteKubeconfigServer(kubeconfig []byte, endpoint string) ([]byte, error) {
	lines := strings.Split(string(kubeconfig), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "server:") {
			indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " ")))
			lines[i] = fmt.Sprintf("%sserver: https://%s", indent, endpoint)
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}
