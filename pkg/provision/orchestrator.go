package provision

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	observer  ProgressObserver
	// management is the optional CAPI management-cluster client.
	management ManagementClient
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
	if spec.NetworkPlugin == "cilium" {
		if _, err := p.run(ctx, firstCP, key, "cni-prereq", "command -v helm >/dev/null 2>&1 || { echo 'Cilium bootstrap requires a preinstalled Helm CLI' >&2; exit 1; }"); err != nil {
			return Result{}, stepErr("cni-prereq", err)
		}
	}
	if _, err := p.run(ctx, firstCP, key, "prereq", PrereqScript(spec)); err != nil {
		return Result{}, stepErr("prereq", err)
	}
	if _, err := p.run(ctx, firstCP, key, "containerd-config", RuntimeConfigScript(spec)); err != nil {
		return Result{}, stepErr("containerd-config", err)
	}
	initOut, err := p.run(ctx, firstCP, key, "kubeadm-init", InitScript(spec, firstCP))
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
		if _, err := p.run(ctx, n, key, "join-control-plane", JoinScript(spec, n, token, caHash)); err != nil {
			return Result{}, stepErr("join-control-plane:"+n.Host, err)
		}
	}

	// 3. worker nodes join.
	for _, n := range spec.WorkerNodes() {
		if _, err := p.run(ctx, n, key, "prereq-worker", PrereqScript(spec)); err != nil {
			return Result{}, stepErr("prereq-worker:"+n.Host, err)
		}
		if _, err := p.run(ctx, n, key, "containerd-config-worker", RuntimeConfigScript(spec)); err != nil {
			return Result{}, stepErr("containerd-config-worker:"+n.Host, err)
		}
		if _, err := p.run(ctx, n, key, "join-worker", JoinScript(spec, n, token, caHash)); err != nil {
			return Result{}, stepErr("join-worker:"+n.Host, err)
		}
	}

	// 4. install the selected pod network on the control plane.
	if _, err := p.run(ctx, firstCP, key, "cni", NetworkScript(spec)); err != nil {
		return Result{}, stepErr("cni", err)
	}

	// Observe the real workload cluster before publishing credentials/success.
	accepted, err := p.run(ctx, firstCP, key, "cluster-acceptance", AcceptanceScript(spec))
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

// AddNodes joins new worker nodes to an existing SSH-provisioned cluster. It
// never repeats kubeadm init: the control-plane script only verifies API
// readiness and mints a fresh join token. Nodes that are already cluster
// members are counted as replay and left untouched.
func (p *Provisioner) AddNodes(ctx context.Context, spec AddNodesSpec) (AddNodesResult, error) {
	result := AddNodesResult{Endpoint: spec.ControlPlaneEndpoint, Requested: len(spec.Nodes)}
	if p.management != nil {
		return result, stepErr("strategy", fmt.Errorf("CAPI provisioning is not implemented; cannot combine CAPI reconciliation with SSH bootstrap"))
	}
	if p.bootstrap == nil || p.bootstrap.SSH == nil {
		return result, stepErr("configuration", fmt.Errorf("SSH runner is required"))
	}
	if err := validateAddNodesSpec(spec); err != nil {
		return result, stepErr("plan", err)
	}
	// Scripts are generated from a ProvisionSpec carrying only the
	// join-relevant fields: endpoint, version, credential, and the new nodes.
	inner := ProvisionSpec{
		TenantID: spec.TenantID, TargetID: spec.TargetID, DesiredVersion: spec.DesiredVersion,
		ControlPlaneEndpoint: spec.ControlPlaneEndpoint, IdempotencyKey: spec.IdempotencyKey,
		Nodes: spec.Nodes, SSHCredential: spec.SSHCredential,
	}
	key, err := p.sshKey(ctx, inner)
	if err != nil {
		return result, err
	}
	cpHost, _, splitErr := net.SplitHostPort(spec.ControlPlaneEndpoint)
	if splitErr != nil {
		return result, stepErr("plan", fmt.Errorf("controlPlaneEndpoint must be an address with port 6443"))
	}
	cp := NodeSpec{ID: "control-plane", Host: cpHost, Role: RoleControlPlane}

	// Mint a join token on the initialized control plane. InitScript's
	// already-initialized branch verifies readiness and prints a fresh join
	// command; kubeadm init itself is never repeated.
	initOut, err := p.run(ctx, cp, key, "join-token", InitScript(inner, cp))
	if err != nil {
		return result, stepErr("join-token", fmt.Errorf("%w: %s", err, tail(initOut)))
	}
	token, caHash, err := ParseJoinDetails(initOut)
	if err != nil {
		return result, stepErr("parse-join", err)
	}

	for _, n := range spec.Nodes {
		if _, err := p.run(ctx, n, key, "prereq-node", PrereqScript(inner)); err != nil {
			return result, stepErr("prereq-node:"+n.Host, err)
		}
		if _, err := p.run(ctx, n, key, "containerd-config-node", RuntimeConfigScript(inner)); err != nil {
			return result, stepErr("containerd-config-node:"+n.Host, err)
		}
		joinOut, err := p.run(ctx, n, key, "join-node", JoinScript(inner, n, token, caHash))
		if err != nil {
			return result, stepErr("join-node:"+n.Host, fmt.Errorf("%w: %s", err, tail(joinOut)))
		}
		if strings.Contains(joinOut, "__KUBEADM_ALREADY_JOINED__") {
			result.AlreadyJoined++
		} else {
			result.Joined++
		}
	}

	// Observe the new nodes as cluster members before reporting success.
	names := make([]string, 0, len(spec.Nodes))
	for _, n := range spec.Nodes {
		names = append(names, n.ID)
	}
	accepted, err := p.run(ctx, cp, key, "nodes-acceptance", NodesReadyScript(names))
	if err != nil {
		return result, stepErr("nodes-acceptance", err)
	}
	if !strings.Contains(accepted, "__HNB_NODES_READY__") {
		return result, stepErr("nodes-acceptance", fmt.Errorf("readiness evidence missing"))
	}
	return result, nil
}

func validateAddNodesSpec(spec AddNodesSpec) error {
	if len(spec.Nodes) == 0 {
		return fmt.Errorf("node add requires at least one node")
	}
	if spec.ControlPlaneEndpoint == "" {
		return fmt.Errorf("control-plane endpoint is required")
	}
	if _, port, err := net.SplitHostPort(spec.ControlPlaneEndpoint); err != nil || port != strconv.Itoa(bindPort) {
		return fmt.Errorf("controlPlaneEndpoint must be an address with port %d", bindPort)
	}
	if !regexp.MustCompile(`^v?1\.[0-9]+\.[0-9]+$`).MatchString(spec.DesiredVersion) {
		return fmt.Errorf("an exact Kubernetes patch version is required")
	}
	seen := map[string]bool{}
	for _, node := range spec.Nodes {
		if node.Role != RoleWorker {
			return fmt.Errorf("node add joins worker nodes only")
		}
		if node.ID == "" || node.Host == "" || node.ID != node.Host {
			return fmt.Errorf("node name must equal its SSH address")
		}
		if seen[node.Host] {
			return fmt.Errorf("duplicate node address %q", node.Host)
		}
		seen[node.Host] = true
		if !regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`).MatchString(node.Host) {
			return fmt.Errorf("invalid node name or address %q", node.Host)
		}
		if !regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`).MatchString(node.EffectiveUser()) || node.EffectivePort() < 1 || node.EffectivePort() > 65535 {
			return fmt.Errorf("invalid SSH user or port")
		}
	}
	return nil
}

func validateSSHSpec(spec ProvisionSpec) error {
	if spec.NetworkPlugin != "" && spec.NetworkPlugin != "flannel" && spec.NetworkPlugin != "cilium" {
		return fmt.Errorf("unsupported network plugin %q", spec.NetworkPlugin)
	}
	if spec.NetworkPlugin == "cilium" && !regexp.MustCompile(`^v?1\.[0-9]+\.[0-9]+$`).MatchString(spec.NetworkVersion) {
		return fmt.Errorf("Cilium requires an exact networkVersion")
	}
	if spec.AdvertiseAddress != "" && net.ParseIP(spec.AdvertiseAddress) == nil {
		return fmt.Errorf("advertiseAddress must be an IP address")
	}
	if spec.ImageRepository != "" && !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9./:_-]*$`).MatchString(spec.ImageRepository) {
		return fmt.Errorf("invalid image repository")
	}
	if spec.SandboxImage != "" && !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9./:@_-]*$`).MatchString(spec.SandboxImage) {
		return fmt.Errorf("invalid sandbox image")
	}
	if spec.CNIImageRepository != "" && !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9./:_-]*$`).MatchString(spec.CNIImageRepository) {
		return fmt.Errorf("invalid CNI image repository")
	}
	if spec.HelmChartRepository != "" && !regexp.MustCompile(`^https?://[a-zA-Z0-9.-]+(:[0-9]+)?(/[a-zA-Z0-9._/-]*)?$`).MatchString(spec.HelmChartRepository) {
		return fmt.Errorf("Helm chart repository must be an explicit registry origin")
	}
	// Bootstrap must resolve every node image through the platform repository.
	// Falling back to a hardcoded public registry would break air-gapped
	// installs and split the supply-chain trust boundary.
	if spec.NetworkPlugin == "cilium" {
		if spec.CNIImageRepository == "" && spec.ImageRepository == "" {
			return fmt.Errorf("cilium bootstrap requires an explicit CNI image repository or image repository")
		}
		if spec.HelmChartRepository == "" {
			return fmt.Errorf("cilium bootstrap requires an explicit Helm chart repository")
		}
	}
	if spec.ControlPlaneEndpoint != "" {
		host, port, err := net.SplitHostPort(spec.ControlPlaneEndpoint)
		if err != nil || net.ParseIP(host) == nil || port != "6443" {
			return fmt.Errorf("controlPlaneEndpoint must be an IP address with port 6443")
		}
	}
	if !regexp.MustCompile(`^v?1\.[0-9]+\.[0-9]+$`).MatchString(spec.DesiredVersion) {
		return fmt.Errorf("an exact Kubernetes patch version is required")
	}
	seen := map[string]bool{}
	for _, node := range spec.Nodes {
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

func (p *Provisioner) run(ctx context.Context, node NodeSpec, key []byte, stage, script string) (string, error) {
	event := StageEvent{Stage: stage, NodeID: node.ID, StartedAt: time.Now().UTC(), State: "running"}
	if p.observer != nil {
		p.observer.Observe(event)
	}
	out, err := p.bootstrap.SSH.Run(ctx, node, key, script)
	event.FinishedAt = time.Now().UTC()
	event.State = "succeeded"
	if err != nil {
		event.State = "failed"
	}
	if p.observer != nil {
		p.observer.Observe(event)
	}
	return out, err
}

func (p *Provisioner) readKubeconfig(ctx context.Context, cp NodeSpec, key []byte, endpoint string) ([]byte, error) {
	out, err := p.run(ctx, cp, key, "read-kubeconfig", AdminKubeconfigScript)
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
