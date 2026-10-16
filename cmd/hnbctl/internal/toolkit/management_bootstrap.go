package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/provision"
)

// BootstrapSources is part of the immutable run identity, not a per-retry
// override. This candidate adapter is internal; install remains gated by 4.10.
type BootstrapSources struct {
	ImageRepository     string `json:"imageRepository"`
	SandboxImage        string `json:"sandboxImage"`
	CNIImageRepository  string `json:"cniImageRepository"`
	HelmChartRepository string `json:"helmChartRepository"`
}

// ManagementBootstrap adapts shared node scripts to the local Day-0 run store.
// It never creates platform Operations or writes a platform Secret/Read Model.
// The first adapter supports one control-plane; worker joins remain unsupported.
type ManagementBootstrap struct {
	cfg          *DeploymentConfig
	lock         *ReleaseLock
	spec         provision.ProvisionSpec
	store        RunStore
	runner       NodeProbeRunner
	runID        string
	configDigest string
	imageCache   string
}

type ManagementBootstrapResult struct {
	RunID                string `json:"runId"`
	ManagementClusterUID string `json:"managementClusterUID"`
	KubeconfigFile       string `json:"kubeconfigFile"`
	ClusterReady         bool   `json:"clusterReady"`
	PlatformVerified     bool   `json:"platformVerified"`
}

var machineIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var bootstrapClaimPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// NewManagementBootstrap validates and snapshots all inputs before any SSH or
// state creation. Unknown target/platform references are rejected by DecodeConfig.
func NewManagementBootstrap(cfg *DeploymentConfig, lock *ReleaseLock, sources BootstrapSources, runID, runDir string, runner NodeProbeRunner) (*ManagementBootstrap, error) {
	if cfg == nil || lock == nil || runner == nil || !machineIDPattern.MatchString(runID) || runDir == "" {
		return nil, fmt.Errorf("BOOTSTRAP_INPUT_INVALID: config, lock, runner, opaque run id and run directory required")
	}
	if cfg.Spec.Mode != ModeBootstrap || cfg.Spec.Cluster.Strategy != StrategyProvision || len(cfg.Validate(CurrentEvidence)) > 0 {
		return nil, fmt.Errorf("BOOTSTRAP_BOUNDARY_REFUSED: dedicated bootstrap configuration required")
	}
	if len(lock.Validate()) > 0 || len(lock.ValidateCompatibility(cfg, CurrentEvidence)) > 0 {
		return nil, fmt.Errorf("BOOTSTRAP_LOCK_INVALID: incompatible release lock")
	}
	if cfg.Spec.Artifacts.Mode != ArtifactsOnline || len(cfg.Spec.Cluster.Nodes) != 1 || cfg.Spec.Cluster.Nodes[0].Roles[0] != RoleControlPlane {
		return nil, fmt.Errorf("BOOTSTRAP_ROUTE_UNSUPPORTED: candidate adapter supports online single-control-plane only")
	}
	if !validRepositoryPrefix(sources.ImageRepository) || !validRepositoryPrefix(sources.CNIImageRepository) || !validProbeURL(sources.HelmChartRepository) {
		return nil, fmt.Errorf("BOOTSTRAP_SOURCE_INVALID: explicit image and chart repositories required")
	}
	// Every node image locator must be included in the selected release. This
	// candidate adapter does not certify channel-resolved packages or the release.
	if len(lock.Compatibility.OS) != 1 {
		return nil, fmt.Errorf("BOOTSTRAP_LOCK_INVALID: select one node OS recipe")
	}
	recipe, err := ResolveNodeRecipe(lock.Compatibility.OS[0], "amd64", cfg.Spec.Cluster.Network.Plugin, cfg.Spec.Cluster.Network.Version,
		cfg.Spec.Cluster.KubernetesVersion, sources.SandboxImage, sources.ImageRepository, sources.CNIImageRepository)
	if err != nil {
		return nil, fmt.Errorf("BOOTSTRAP_SOURCE_INVALID: node recipe unavailable")
	}
	for _, required := range recipe.Images {
		found := false
		for _, image := range lock.Images {
			if image.Ref == required.Ref {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("BOOTSTRAP_LOCK_INVALID: node image absent from release")
		}
	}
	// JSON snapshots avoid accepting later mutations of caller-owned slices/maps.
	data, err := json.Marshal(struct {
		Config  *DeploymentConfig
		Sources BootstrapSources
	}{cfg, sources})
	if err != nil {
		return nil, fmt.Errorf("BOOTSTRAP_INPUT_INVALID: cannot bind configuration")
	}
	var snapshot struct {
		Config  *DeploymentConfig
		Sources BootstrapSources
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	lockBytes, _ := json.Marshal(lock)
	var locked ReleaseLock
	if err := json.Unmarshal(lockBytes, &locked); err != nil {
		return nil, err
	}
	node := snapshot.Config.Spec.Cluster.Nodes[0]
	return &ManagementBootstrap{
		cfg: snapshot.Config, lock: &locked, runner: runner, runID: runID, store: RunStore{Dir: runDir},
		configDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
		spec: provision.ProvisionSpec{DesiredVersion: cfg.Spec.Cluster.KubernetesVersion, ControlPlaneEndpoint: cfg.Spec.Cluster.APIEndpoint,
			PodCIDR: cfg.Spec.Cluster.Network.PodCIDR, ServiceCIDR: cfg.Spec.Cluster.Network.ServiceCIDR,
			NetworkPlugin: cfg.Spec.Cluster.Network.Plugin, NetworkVersion: cfg.Spec.Cluster.Network.Version,
			ImageRepository: sources.ImageRepository, SandboxImage: sources.SandboxImage,
			CNIImageRepository: sources.CNIImageRepository, HelmChartRepository: sources.HelmChartRepository,
			AdvertiseAddress: node.InternalAddress, Nodes: []provision.NodeSpec{{ID: node.Name, Host: node.SSHAddress, User: node.User, Role: provision.RoleControlPlane}}},
	}, nil
}

const bootstrapIdentityScript = `set -eu
printf 'machineId=%s\n' "$(cat /etc/machine-id)"
version=none
if command -v kubeadm >/dev/null 2>&1; then version=$(kubeadm version -o short); fi
printf 'version=%s\n' "$version"
claim=none
if [ -f /etc/hnb/bootstrap/owner ]; then claim=$(cat /etc/hnb/bootstrap/owner); fi
printf 'claim=%s\n' "$claim"
foreign=no
for p in /etc/rancher/rke2 /var/lib/rancher/rke2 /etc/rancher/k3s /var/lib/rancher/k3s; do
  if [ -e "$p" ]; then foreign=yes; fi
done
printf 'foreign=%s\n' "$foreign"
uid=none
if [ -s /etc/kubernetes/admin.conf ]; then
  uid=$(kubectl --kubeconfig /etc/kubernetes/admin.conf get namespace kube-system -o jsonpath='{.metadata.uid}')
fi
printf 'clusterUID=%s\n' "$uid"
`

type bootstrapIdentity struct{ machineID, version, claim, foreign, clusterUID string }

func (b *ManagementBootstrap) identity(ctx context.Context) (bootstrapIdentity, error) {
	var out bootstrapIdentity
	raw, err := b.runner.Run(ctx, b.cfg.Spec.Cluster.Nodes[0], bootstrapIdentityScript)
	if err != nil {
		return out, fmt.Errorf("BOOTSTRAP_IDENTITY_UNAVAILABLE")
	}
	values := map[string]*string{"machineId": &out.machineID, "version": &out.version, "claim": &out.claim, "foreign": &out.foreign, "clusterUID": &out.clusterUID}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		field := values[key]
		if !ok || field == nil || seen[key] || len(value) > 128 {
			return out, fmt.Errorf("BOOTSTRAP_IDENTITY_INVALID")
		}
		seen[key], *field = true, value
	}
	if len(seen) != 5 || !machineIDPattern.MatchString(out.machineID) || out.foreign != "no" ||
		(out.claim != "none" && !bootstrapClaimPattern.MatchString(out.claim)) ||
		(out.version != "none" && out.version != "v"+b.spec.DesiredVersion) ||
		(out.clusterUID != "none" && !validName(out.clusterUID)) {
		return out, fmt.Errorf("BOOTSTRAP_IDENTITY_CONFLICT: machine, version or lifecycle owner differs")
	}
	return out, nil
}

func (b *ManagementBootstrap) claim(machineID string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(b.runID+"\n"+b.configDigest+"\n"+lockContentDigest(b.lock)+"\n"+machineID)))
}

// Execute is the internal candidate adapter. Successful node/cluster work keeps
// the overall install run incomplete; platform installation has separate gates.
func (b *ManagementBootstrap) Execute(ctx context.Context) (ManagementBootstrapResult, error) {
	result := ManagementBootstrapResult{RunID: b.runID}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("BOOTSTRAP_CANCELED")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	identity, err := b.identity(probeCtx)
	cancel()
	if err != nil {
		return result, err
	}
	claim := b.claim(identity.machineID)
	var previous *InstallRun
	if _, err := os.Lstat(b.store.Dir); err == nil {
		previous, err = LoadRunState(b.store.Dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("BOOTSTRAP_STATE_UNSAFE: existing run cannot be read")
		}
	} else if !os.IsNotExist(err) {
		return result, fmt.Errorf("BOOTSTRAP_STATE_UNSAFE")
	}
	if identity.claim != "none" && (previous == nil || identity.claim != claim) {
		return result, fmt.Errorf("BOOTSTRAP_OWNER_PRESENT: use the owning lifecycle entry")
	}
	if identity.clusterUID != "none" && (previous == nil || previous.ManagementClusterUID != identity.clusterUID || identity.claim != claim) {
		return result, fmt.Errorf("BOOTSTRAP_CLUSTER_CHANGED: refusing to adopt or reset a cluster")
	}
	checks := ProbeLockedNodes(ctx, b.cfg, b.lock, b.runner)
	if len(checks) == 0 {
		return result, fmt.Errorf("BOOTSTRAP_PREFLIGHT_FAILED: no node observations")
	}
	for _, check := range checks {
		ownedResume := check.CheckID == "node.owner-markers" && check.Actual == "hnb" && previous != nil && identity.claim == claim
		if check.Status != StatusPass && !ownedResume {
			return result, fmt.Errorf("BOOTSTRAP_PREFLIGHT_FAILED: %s", check.ReasonCode)
		}
	}
	ownership := []string{"management-node/" + b.cfg.Spec.Cluster.Nodes[0].SSHAddress, "machine-id/" + identity.machineID, "claim/" + claim}
	run, err := b.store.Create(b.runID, b.configDigest, lockContentDigest(b.lock), string(ModeBootstrap), ownership)
	if err != nil {
		return result, err
	}
	writer, err := NewRunID()
	if err != nil {
		return result, err
	}
	ttl := 3*b.cfg.Spec.Timeouts.Download + 4*b.cfg.Spec.Timeouts.Readiness + time.Minute
	run, err = b.store.AcquireWriter(writer, ttl)
	if err != nil {
		return result, err
	}
	defer b.store.ReleaseWriter(writer, run.Epoch)
	node := b.spec.Nodes[0]
	stages := []struct {
		phase        PhaseName
		script, post string
	}{
		{PhaseNodePrereq, provision.PrereqScript(b.spec), "test \"$(kubeadm version -o short)\" = 'v" + b.spec.DesiredVersion + "'\ncommand -v helm >/dev/null"},
		{PhaseRuntime, provision.RuntimeConfigScript(b.spec), "systemctl is-active --quiet containerd"},
		{PhaseInit, provision.InitScript(b.spec, node), "test -s /etc/kubernetes/admin.conf\nkubectl --kubeconfig /etc/kubernetes/admin.conf get namespace kube-system -o jsonpath='{.metadata.uid}'"},
		{PhaseNetwork, provision.NetworkScript(b.spec), "kubectl --kubeconfig /etc/kubernetes/admin.conf -n kube-system rollout status daemonset/cilium --timeout=120s"},
		{PhaseClusterVerified, provision.AcceptanceScript(b.spec), "kubectl --kubeconfig /etc/kubernetes/admin.conf get namespace kube-system -o jsonpath='{.metadata.uid}'"},
	}
	for _, stage := range stages {
		state, err := LoadRunState(b.store.Dir)
		if err != nil {
			return result, err
		}
		if recorded := state.Phases[string(stage.phase)]; recorded != nil && recorded.State != PhasePendingState {
			if recorded.State != PhaseSucceededState {
				if phasesNeverRerun[stage.phase] || recorded.State != PhaseFailedState {
					return result, fmt.Errorf("%s: %s requires operator recheck; execution not repeated", RunReasonNeverRerun, stage.phase)
				}
			} else {
				if _, err := b.remote(ctx, identity.machineID, claim, run.Epoch, stage.phase, "", stage.post); err != nil {
					return result, err
				}
				if stage.phase == PhaseNodePrereq {
					b.attemptPreload(ctx, writer, run.Epoch, identity, claim, state)
				}
				continue
			}
		}
		budget := b.cfg.Spec.Timeouts.Readiness
		if stage.phase == PhaseNodePrereq || stage.phase == PhaseRuntime {
			budget = b.cfg.Spec.Timeouts.Download
		}
		err = b.store.RunPhaseWithTimeout(ctx, writer, run.Epoch, stage.phase, budget, nil, nil, func(stageCtx context.Context) error {
			out, err := b.remote(stageCtx, identity.machineID, claim, run.Epoch, stage.phase, stage.script, stage.post)
			if err != nil {
				return err
			}
			if stage.phase == PhaseInit {
				uid := strings.TrimSpace(out)
				return b.store.BindManagementCluster(writer, run.Epoch, uid)
			}
			return nil
		})
		if err != nil {
			return result, err
		}
		if stage.phase == PhaseNodePrereq {
			b.attemptPreload(ctx, writer, run.Epoch, identity, claim, state)
		}
	}
	state, err := LoadRunState(b.store.Dir)
	if err != nil {
		return result, err
	}
	// Credentials are a private local attachment, not a platform secret record.
	credentials, err := b.remote(ctx, identity.machineID, claim, run.Epoch, PhaseClusterVerified, "", provision.AdminKubeconfigScript)
	if err != nil {
		return result, err
	}
	result.KubeconfigFile = filepath.Join(b.store.Dir, "management.kubeconfig")
	if err := writeBootstrapAttachment(result.KubeconfigFile, []byte(credentials)); err != nil {
		return result, err
	}
	result.ManagementClusterUID, result.ClusterReady = state.ManagementClusterUID, true
	return result, nil
}

// remote holds a node-local lock for the entire stage, checks machine identity,
// pins a permanent run claim and monotonically advances the remote fence. Script
// stdout/stderr goes to root-only node logs, never into a receipt or normal output.
// bootstrapGuard is the node-side Day-0 fence shared by stage execution and
// preload streaming: a node-local lock, machine identity check, permanent owner
// claim pinning and a monotonic epoch. Both writers of /etc/hnb/bootstrap must
// go through this program so claim/epoch state stays coherent.
func bootstrapGuard(phase PhaseName, machineID, claim string, epoch int64) string {
	return fmt.Sprintf(`set -eu
# hnb-bootstrap-stage=%s
command -v flock >/dev/null
exec 9>/run/lock/hnb-management-bootstrap.lock
flock -n 9
test "$(cat /etc/machine-id)" = '%s'
test ! -L /etc/hnb
test ! -L /etc/hnb/bootstrap
if [ ! -e /etc/hnb/bootstrap ]; then
  for p in /etc/kubernetes/admin.conf /etc/kubernetes/kubelet.conf /var/lib/kubelet/config.yaml /etc/kubernetes/manifests /etc/rancher/rke2 /var/lib/rancher/rke2 /etc/rancher/k3s /var/lib/rancher/k3s /var/lib/hnb; do
    test ! -e "$p"
  done
  install -d -m 0700 /etc/hnb/bootstrap
  printf '%%s\n' '%s' > /etc/hnb/bootstrap/owner
  chmod 0600 /etc/hnb/bootstrap/owner
fi
test ! -L /etc/hnb/bootstrap/owner
test "$(cat /etc/hnb/bootstrap/owner)" = '%s'
old=0
if [ -e /etc/hnb/bootstrap/epoch ]; then old=$(cat /etc/hnb/bootstrap/epoch); fi
test "$old" -le %d
printf '%%s\n' '%d' > /etc/hnb/bootstrap/epoch
chmod 0600 /etc/hnb/bootstrap/epoch
`, phase, machineID, claim, claim, epoch, epoch)
}

func (b *ManagementBootstrap) remote(ctx context.Context, machineID, claim string, epoch int64, phase PhaseName, script, post string) (string, error) {
	guard := bootstrapGuard(phase, machineID, claim, epoch)
	if script != "" {
		if phase == PhaseInit {
			guard += "test ! -e /etc/kubernetes/admin.conf\ntest ! -e /etc/kubernetes/kubelet.conf\n"
		}
		guard += "umask 077\n(\n" + script + "\n) > /etc/hnb/bootstrap/" + string(phase) + ".log 2>&1\n"
	}
	out, err := b.runner.Run(ctx, b.cfg.Spec.Cluster.Nodes[0], guard+post+"\n")
	if err != nil {
		return "", fmt.Errorf("BOOTSTRAP_NODE_STAGE_FAILED: %s", phase)
	}
	return out, nil
}

// WithPreloadCache points the adapter at a private verified prepare cache so
// the prereq stage streams pinned images into the node's containerd before
// init. It is optional: without a cache the bootstrap pulls from the registry
// as before.
func (b *ManagementBootstrap) WithPreloadCache(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil || abs == string(filepath.Separator) {
		return fmt.Errorf("BOOTSTRAP_PRELOAD_INVALID: invalid cache directory")
	}
	fi, err := os.Lstat(abs)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("BOOTSTRAP_PRELOAD_INVALID: cache must be an existing private directory (0700)")
	}
	b.imageCache = abs
	return nil
}

// attemptPreload streams verified images once, best-effort: a failure is
// recorded as a retryable failed preparing phase and online pulls continue.
// It runs only after node-prereq (containerd present) and never re-executes a
// succeeded preparing phase on resume.
func (b *ManagementBootstrap) attemptPreload(ctx context.Context, writer string, epoch int64, identity bootstrapIdentity, claim string, state *InstallRun) {
	if b.imageCache == "" {
		return
	}
	if prep := state.Phases[string(PhasePreparing)]; prep != nil && prep.State != PhaseFailedState {
		return
	}
	streamer, ok := b.runner.(NodeStreamReader)
	if !ok {
		return
	}
	preloadCtx, cancel := context.WithTimeout(ctx, b.cfg.Spec.Timeouts.Download)
	defer cancel()
	_ = b.store.RunPhaseWithTimeout(preloadCtx, writer, epoch, PhasePreparing, b.cfg.Spec.Timeouts.Download, nil, nil, func(prepareCtx context.Context) error {
		preloader := &ImagePreloader{cacheDir: b.imageCache, runner: streamer}
		_, err := preloader.PreloadImages(prepareCtx, b.lock, preloadAuth{
			machineID: identity.machineID, claim: claim, epoch: epoch, node: b.cfg.Spec.Cluster.Nodes[0],
		})
		return err
	})
}

func writeBootstrapAttachment(path string, data []byte) error {
	if len(data) == 0 || len(data) > 1<<20 || !strings.Contains(string(data), "apiVersion:") || !strings.Contains(string(data), "clusters:") {
		return fmt.Errorf("BOOTSTRAP_CREDENTIAL_INVALID")
	}
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 {
			return fmt.Errorf("BOOTSTRAP_CREDENTIAL_UNSAFE")
		}
		old, err := os.ReadFile(path)
		if err != nil || string(old) != string(data) {
			return fmt.Errorf("BOOTSTRAP_CREDENTIAL_CHANGED")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("BOOTSTRAP_CREDENTIAL_UNAVAILABLE")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("BOOTSTRAP_CREDENTIAL_UNAVAILABLE")
	}
	_, writeErr := file.Write(data)
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("BOOTSTRAP_CREDENTIAL_WRITE_FAILED")
	}
	return nil
}

// ReleaseWriter releases only the current writer's fence; an old deferred release
// cannot clear a newer writer's lease.
func (s RunStore) ReleaseWriter(writer string, epoch int64) error {
	return s.withLock(func() error {
		run, err := s.read()
		if err != nil {
			return err
		}
		if err := s.assertWriter(run, writer, epoch); err != nil {
			return err
		}
		run.WriterID, run.LeaseExpires = "", s.now()
		return s.write(run)
	})
}

// Keep the remote fence formatting integral and independent of shell input.
var _ = strconv.IntSize
