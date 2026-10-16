package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// fakeBootstrapNode simulates the small, stateful surface of a dedicated blank
// management node: machine-id, kubeadm version, a permanent owner claim under
// /etc/hnb/bootstrap, foreign lifecycle markers, admin.conf presence and the
// cluster UID visible through it. The remote guard's claim/epoch writes are
// replayed so resumption observes the same permanent identity.
type fakeBootstrapNode struct {
	mu            sync.Mutex
	machineID     string
	wantedVersion string
	version       string
	claim         string
	epoch         int64
	adminConf     bool
	clusterUID    string
	foreign       bool
	fail          map[string]bool
	scripts       []string
	streamScripts []string
	streamErr     bool
	streamData    [][]byte
	events        []string
}

func newFakeBootstrapNode() *fakeBootstrapNode {
	return &fakeBootstrapNode{
		machineID:     "0123456789abcdef0123456789abcdef",
		wantedVersion: "1.36.5",
		version:       "none",
		clusterUID:    "kube-system-uid-1",
		fail:          map[string]bool{},
	}
}

var ownerClaimRe = regexp.MustCompile(`printf '%s\\n' '([0-9a-f]{64})' > /etc/hnb/bootstrap/owner`)
var epochRe = regexp.MustCompile(`printf '%s\\n' '([0-9]+)' > /etc/hnb/bootstrap/epoch`)
var stageRe = regexp.MustCompile(`# hnb-bootstrap-stage=([a-z-]+)`)

const managementKubeconfigFixture = "apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://10.0.0.10:6443\n  name: management\n"

func bootstrapStage(script string) string {
	if match := stageRe.FindStringSubmatch(script); len(match) == 2 {
		return match[1]
	}
	return ""
}

func (n *fakeBootstrapNode) Run(_ context.Context, _ NodeConfig, script string) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.scripts = append(n.scripts, script)
	if strings.Contains(script, "machineId=") {
		n.events = append(n.events, "run:identity")
		uid := "none"
		if n.adminConf {
			uid = n.clusterUID
		}
		claim := n.claim
		if claim == "" {
			claim = "none"
		}
		foreign := "no"
		if n.foreign {
			foreign = "yes"
		}
		return fmt.Sprintf("machineId=%s\nversion=%s\nclaim=%s\nforeign=%s\nclusterUID=%s\n", n.machineID, n.version, claim, foreign, uid), nil
	}
	if script == NodeProbeScript {
		n.events = append(n.events, "run:probe")
		fixture := nodeProbeFixture
		if n.claim != "" {
			fixture = strings.Replace(fixture, "owner=none", "owner=hnb", 1)
		}
		return fixture, nil
	}
	if strings.Contains(script, "cat /etc/kubernetes/admin.conf") {
		n.events = append(n.events, "run:credentials")
		return managementKubeconfigFixture, nil
	}
	stage := bootstrapStage(script)
	if stage == "" {
		return "", fmt.Errorf("unexpected bootstrap program")
	}
	n.events = append(n.events, "run:"+stage)
	if match := ownerClaimRe.FindStringSubmatch(script); len(match) == 2 {
		n.claim = match[1]
	}
	if match := epochRe.FindStringSubmatch(script); len(match) == 2 {
		fmt.Sscanf(match[1], "%d", &n.epoch)
	}
	if n.fail[stage] {
		return "", fmt.Errorf("simulated %s failure", stage)
	}
	switch stage {
	case "init":
		if !n.adminConf {
			n.adminConf = true
		}
		return n.clusterUID, nil
	case "node-prereq":
		n.version = "v" + n.wantedVersion
	}
	return "", nil
}

// Stream receives one assembled image archive per pinned image, under the same
// Day-0 guard program the stage scripts use.
func (n *fakeBootstrapNode) Stream(_ context.Context, _ NodeConfig, script string, data io.Reader) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.scripts = append(n.scripts, script)
	n.streamScripts = append(n.streamScripts, script)
	n.events = append(n.events, "stream")
	buf, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}
	n.streamData = append(n.streamData, buf)
	if n.streamErr {
		return "", fmt.Errorf("simulated stream failure")
	}
	return "imported\n", nil
}

func (n *fakeBootstrapNode) claimIsOwned() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.claim != ""
}

// bootstrapLockFixture builds a release lock whose images cover the full cilium
// node recipe so the dedicated adapter's closure gate passes.
func bootstrapLockFixture(t *testing.T) *ReleaseLock {
	t.Helper()
	images := []string{
		"registry.invalid/hnb/cilium:v1.20.1",
		"registry.invalid/hnb/cilium-operator:v1.20.1",
		"registry.invalid/hnb/cilium-envoy:v1.37.5-1786810558-766ccfb37260a43e9d228837aa84ce3faf9f64e7",
		"registry.invalid/hnb/kube-apiserver:v1.36.5",
		"registry.invalid/hnb/kube-controller-manager:v1.36.5",
		"registry.invalid/hnb/kube-scheduler:v1.36.5",
		"registry.invalid/hnb/coredns:v1.14.2",
		"registry.invalid/hnb/pause:3.10.2",
	}
	pinned := make([]PinnedImage, 0, len(images))
	for i, ref := range images {
		pinned = append(pinned, PinnedImage{
			Name:   fmt.Sprintf("node-%d", i),
			Ref:    ref,
			Digest: fmt.Sprintf("sha256:%064x", i+1),
		})
	}
	lock := &ReleaseLock{
		SchemaVersion:  LockSchemaVersion,
		ReleaseID:      "management-fixture",
		ReleaseVersion: "1.0",
		Images:         pinned,
		Compatibility: LockCompatibility{
			OS:                 []string{"ubuntu-24.04"},
			Arch:               []string{"amd64"},
			KubernetesVersions: []string{"1.36.5"},
			Profiles:           []string{"Minimal"},
		},
	}
	if problems := lock.Validate(); len(problems) > 0 {
		t.Fatalf("lock fixture invalid: %v", problems)
	}
	return lock
}

func bootstrapFixture(t *testing.T) (*DeploymentConfig, *ReleaseLock, BootstrapSources) {
	t.Helper()
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Normalize(t.TempDir(), os.Getenv("HOME")); err != nil {
		t.Fatal(err)
	}
	lock := bootstrapLockFixture(t)
	if problems := lock.ValidateCompatibility(cfg, CurrentEvidence); len(problems) > 0 {
		t.Fatalf("lock/cfg incompatible: %v", problems)
	}
	sources := BootstrapSources{
		ImageRepository:     "registry.invalid/hnb",
		SandboxImage:        "registry.invalid/hnb/pause:3.10.2",
		CNIImageRepository:  "registry.invalid/hnb",
		HelmChartRepository: "https://registry.invalid/hnb/charts",
	}
	return cfg, lock, sources
}

func privateRunDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestManagementBootstrapFullRunAndResumeRechecks(t *testing.T) {
	cfg, lock, sources := bootstrapFixture(t)
	runID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	runDir := privateRunDir(t)
	node := newFakeBootstrapNode()
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ClusterReady || result.PlatformVerified || result.ManagementClusterUID != "kube-system-uid-1" || result.RunID != runID {
		t.Fatalf("unexpected result: %+v", result)
	}
	run, err := LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if run.RunID != runID || run.RoutingMode != "bootstrap" || run.ManagementClusterUID != "kube-system-uid-1" {
		t.Fatalf("run identity not bound: %+v", run)
	}
	for _, phase := range []PhaseName{PhaseNodePrereq, PhaseRuntime, PhaseInit, PhaseNetwork, PhaseClusterVerified} {
		if p := run.Phases[string(phase)]; p == nil || p.State != PhaseSucceededState {
			t.Fatalf("phase %s not succeeded: %+v", phase, run.Phases)
		}
	}
	if run.Status != PhaseRunningState {
		t.Fatal("bootstrap without platform verification must not be finalized succeeded")
	}
	kubeconfig, err := os.ReadFile(filepath.Join(runDir, "management.kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kubeconfig), "clusters:") {
		t.Fatal("kubeconfig attachment missing")
	}
	probeChecks := ProbeLockedNodes(context.Background(), cfg, lock, node)
	var ownerCheck *PreflightCheck
	for i := range probeChecks {
		if probeChecks[i].CheckID == "node.owner-markers" {
			ownerCheck = &probeChecks[i]
			break
		}
	}
	if ownerCheck == nil || ownerCheck.Actual != "hnb" {
		t.Fatalf("owner claim not observed on node: %+v", probeChecks)
	}
	scripts := append([]string(nil), node.scripts...)

	// Resumption must not re-execute init and must recheck succeeded phases.
	resumed, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	result, err = resumed.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ClusterReady || result.ManagementClusterUID != "kube-system-uid-1" {
		t.Fatalf("resume changed result: %+v", result)
	}
	resumedScripts := node.scripts[len(scripts):]
	var initRuns int
	for _, script := range resumedScripts {
		if bootstrapStage(script) == "init" && strings.Contains(script, "kubeadm init") {
			initRuns++
		}
	}
	if initRuns != 0 {
		t.Fatal("resume re-executed init")
	}
}

func TestManagementBootstrapRetriesOnlyFailedPhase(t *testing.T) {
	cfg, lock, sources := bootstrapFixture(t)
	runID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	runDir := privateRunDir(t)
	node := newFakeBootstrapNode()
	node.fail["runtime"] = true
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Execute(context.Background()); err == nil || !strings.Contains(err.Error(), "BOOTSTRAP_NODE_STAGE_FAILED") {
		t.Fatalf("runtime failure not surfaced: %v", err)
	}
	run, err := LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if p := run.Phases[string(PhaseNodePrereq)]; p == nil || p.State != PhaseSucceededState {
		t.Fatalf("prereq not succeeded before the failed stage: %+v", run.Phases)
	}
	if p := run.Phases[string(PhaseRuntime)]; p == nil || p.State != PhaseFailedState {
		t.Fatalf("runtime not recorded failed: %+v", run.Phases)
	}
	if !node.claimIsOwned() {
		t.Fatal("claim was not pinned before the failed stage")
	}
	scripts := append([]string(nil), node.scripts...)
	node.fail["runtime"] = false
	retried, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	result, err := retried.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ClusterReady {
		t.Fatalf("retry did not reach cluster ready: %+v", result)
	}
	run, err = LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if p := run.Phases[string(PhaseRuntime)]; p == nil || p.State != PhaseSucceededState {
		t.Fatal("retry did not complete the failed phase")
	}
	retryScripts := node.scripts[len(scripts):]
	var prereqReexec, runtimeRerun int
	for _, script := range retryScripts {
		switch bootstrapStage(script) {
		case "node-prereq":
			if strings.Contains(script, "apt-get") || strings.Contains(script, "__PREREQ") {
				prereqReexec++
			}
		case "runtime":
			if strings.Contains(script, "hnb-bootstrap.toml") {
				runtimeRerun++
			}
		}
	}
	if prereqReexec != 0 {
		t.Fatal("retry re-executed the succeeded prereq phase")
	}
	if runtimeRerun != 1 {
		t.Fatalf("retry did not re-execute only the failed runtime phase: %d", runtimeRerun)
	}
}

func TestManagementBootstrapRefusesIdentityAndBoundaryViolations(t *testing.T) {
	cfg, lock, sources := bootstrapFixture(t)
	runID := "cccccccccccccccccccccccccccccccc"
	base := func(runDir string, mutate func(*fakeBootstrapNode)) (*ManagementBootstrap, *fakeBootstrapNode, error) {
		node := newFakeBootstrapNode()
		mutate(node)
		adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
		if err != nil {
			return nil, nil, err
		}
		_, err = adapter.Execute(context.Background())
		return adapter, node, err
	}
	for name, mutate := range map[string]func(*fakeBootstrapNode){
		"foreign lifecycle marker": func(n *fakeBootstrapNode) { n.foreign = true },
		"kubeadm version conflict": func(n *fakeBootstrapNode) { n.version = "v1.37.0" },
		"existing foreign claim": func(n *fakeBootstrapNode) {
			n.claim = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		},
	} {
		_, _, err := base(privateRunDir(t), mutate)
		if err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// A replaced cluster behind the same run must be refused on resumption.
	node := newFakeBootstrapNode()
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, privateRunDir(t), node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	node.mu.Lock()
	node.clusterUID = "kube-system-uid-replaced"
	node.mu.Unlock()
	adapter, err = NewManagementBootstrap(cfg, lock, sources, runID, adapter.store.Dir, node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Execute(context.Background()); err == nil || !strings.Contains(err.Error(), "BOOTSTRAP_CLUSTER_CHANGED") {
		t.Fatalf("replaced cluster adopted: %v", err)
	}
	// The dedicated adapter refuses routes it is not certified for. Node edits
	// clone the element first: the fixture shares one backing array.
	for name, adjust := range map[string]func(*DeploymentConfig){
		"managed":          func(c *DeploymentConfig) { c.Spec.Mode = ModeManaged },
		"existing":         func(c *DeploymentConfig) { c.Spec.Mode = ModeExisting; c.Spec.Cluster.Strategy = StrategyExisting },
		"mirror artifacts": func(c *DeploymentConfig) { c.Spec.Artifacts.Mode = ArtifactsMirror },
		"multi-node": func(c *DeploymentConfig) {
			first := c.Spec.Cluster.Nodes[0]
			c.Spec.Cluster.Nodes = []NodeConfig{first, first}
		},
		"worker role": func(c *DeploymentConfig) {
			worker := c.Spec.Cluster.Nodes[0]
			worker.Roles = []NodeRole{RoleWorker}
			c.Spec.Cluster.Nodes = []NodeConfig{worker}
		},
	} {
		candidate := *cfg
		adjust(&candidate)
		if _, err := NewManagementBootstrap(&candidate, lock, sources, runID, t.TempDir(), newFakeBootstrapNode()); err == nil {
			t.Fatalf("%s accepted by the dedicated bootstrap adapter", name)
		}
	}
	// The provision recipe is not certifiable when a node image is absent from the lock.
	lockMissing := *lock
	lockMissing.Images = append([]PinnedImage(nil), lock.Images[:len(lock.Images)-1]...)
	if _, err := NewManagementBootstrap(cfg, &lockMissing, sources, runID, t.TempDir(), newFakeBootstrapNode()); err == nil || !strings.Contains(err.Error(), "BOOTSTRAP_LOCK_INVALID") {
		t.Fatalf("missing node image not caught: %v", err)
	}
}

// preloadCacheFixture builds a verified prepare cache covering the full node
// recipe with real content digests, plus a matching release lock. The cache is
// an existing private directory, as required by WithPreloadCache.
func preloadCacheFixture(t *testing.T) (string, *ReleaseLock) {
	t.Helper()
	recipe, err := ResolveNodeRecipe("ubuntu-24.04", "amd64", "cilium", "1.20.1", "1.36.5",
		"registry.invalid/hnb/pause:3.10.2", "registry.invalid/hnb", "registry.invalid/hnb")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "hnb-preload-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), 0o700); err != nil {
		t.Fatal(err)
	}
	images := make([]PinnedImage, 0, len(recipe.Images))
	for i, rimg := range recipe.Images {
		layer := bytes.Repeat([]byte(rimg.Ref), 48)
		config := []byte(fmt.Sprintf(`{"architecture":"amd64","os":"linux","variant":"fixture-%d"}`, i))
		manifest, err := json.Marshal(ociManifest{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.manifest.v1+json",
			Config:        ociDescriptor{Digest: digestOf(config), Size: int64(len(config))},
			Layers:        []ociDescriptor{{Digest: digestOf(layer), Size: int64(len(layer))}},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, blob := range [][]byte{config, layer, manifest} {
			sum := strings.TrimPrefix(digestOf(blob), "sha256:")
			if err := os.WriteFile(filepath.Join(dir, "blobs", "sha256", sum), blob, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		images = append(images, PinnedImage{
			Name: rimg.Purpose, Ref: rimg.Ref, Digest: digestOf(manifest), Platform: "linux/amd64",
		})
	}
	lock := &ReleaseLock{
		SchemaVersion:  LockSchemaVersion,
		ReleaseID:      "preload-fixture",
		ReleaseVersion: "1.0",
		Images:         images,
		Compatibility: LockCompatibility{
			OS:                 []string{"ubuntu-24.04"},
			Arch:               []string{"amd64"},
			KubernetesVersions: []string{"1.36.5"},
			Profiles:           []string{"Minimal"},
		},
	}
	if problems := lock.Validate(); len(problems) > 0 {
		t.Fatalf("preload lock fixture invalid: %v", problems)
	}
	return dir, lock
}

func TestManagementBootstrapPreloadStreamsLockedImagesBeforeInit(t *testing.T) {
	cfg, _, sources := bootstrapFixture(t)
	cacheDir, lock := preloadCacheFixture(t)
	runID := "dddddddddddddddddddddddddddddddd"
	runDir := privateRunDir(t)
	node := newFakeBootstrapNode()
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.WithPreloadCache(cacheDir); err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ClusterReady {
		t.Fatalf("preload broke bootstrap: %+v", result)
	}
	run, err := LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if p := run.Phases[string(PhasePreparing)]; p == nil || p.State != PhaseSucceededState {
		t.Fatalf("preparing phase not succeeded: %+v", run.Phases)
	}
	if len(node.streamData) != len(lock.Images) || len(node.streams()) != len(lock.Images) {
		t.Fatalf("expected one stream per pinned image: data=%d scripts=%d", len(node.streamData), len(node.streams()))
	}
	for _, script := range node.streams() {
		if bootstrapStage(script) != "preparing" || !strings.Contains(script, "ctr -n k8s.io images import") {
			t.Fatalf("stream program lacks preparing guard or ctr import: %s", script)
		}
		if match := ownerClaimRe.FindStringSubmatch(script); len(match) != 2 {
			t.Fatalf("stream program does not pin the owner claim: %s", script)
		}
	}
	first := firstStreamEventIndex(node.events)
	initIndex := indexOf(node.events, "run:init")
	if first < 0 || initIndex < 0 || first > initIndex {
		t.Fatalf("preload did not run before init: events=%v", node.events)
	}
	// The streamed archive must be a complete OCI layout naming the pinned ref.
	var buf bytes.Buffer
	buf.Write(node.streamData[0])
	tr := tar.NewReader(&buf)
	refs := map[string]string{}
	sizes := map[string]int64{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		sizes[hdr.Name] = int64(len(data))
		if hdr.Name == "index.json" {
			var index struct {
				Manifests []struct {
					Digest      string            `json:"digest"`
					Annotations map[string]string `json:"annotations"`
				} `json:"manifests"`
			}
			if json.Unmarshal(data, &index) != nil || len(index.Manifests) != 1 {
				t.Fatalf("streamed layout index invalid")
			}
			refs["ref"] = index.Manifests[0].Annotations["org.opencontainers.image.ref.name"]
			refs["digest"] = index.Manifests[0].Digest
		}
	}
	want := lock.Images[0]
	if refs["ref"] != want.Ref || refs["digest"] != want.Digest {
		t.Fatalf("layout names wrong image: %+v want %s/%s", refs, want.Ref, want.Digest)
	}
	if sizes["oci-layout"] == 0 || len(sizes) != 2+3 { // oci-layout, index.json, manifest, config, layer
		t.Fatalf("layout entries incomplete: %v", sizes)
	}
}

func TestManagementBootstrapPreloadFailureIsBestEffortAndRetryable(t *testing.T) {
	cfg, _, sources := bootstrapFixture(t)
	cacheDir, lock := preloadCacheFixture(t)
	runID := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	runDir := privateRunDir(t)
	node := newFakeBootstrapNode()
	node.streamErr = true
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.WithPreloadCache(cacheDir); err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background())
	if err != nil {
		t.Fatalf("preload failure must not block the online bootstrap: %v", err)
	}
	if !result.ClusterReady {
		t.Fatalf("bootstrap did not continue past preload failure: %+v", result)
	}
	run, err := LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if p := run.Phases[string(PhasePreparing)]; p == nil || p.State != PhaseFailedState {
		t.Fatalf("preload failure not recorded: %+v", run.Phases)
	}
	streamsAfterFailure := len(node.streamData)
	node.streamErr = false
	resumed, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, node)
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.WithPreloadCache(cacheDir); err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	run, err = LoadRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	p := run.Phases[string(PhasePreparing)]
	if p == nil || p.State != PhaseSucceededState {
		t.Fatalf("resume did not retry the failed preload: state=%+v attempts=%d streamsBefore=%d streamsAfter=%d",
			p, p.Attempts, streamsAfterFailure, len(node.streamData))
	}
	// A succeeded preload is never re-executed.
	if _, err := resumed.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(node.streamData) != streamsAfterFailure+len(lock.Images) {
		t.Fatalf("succeeded preload re-executed: %d streams", len(node.streamData))
	}
}

func (n *fakeBootstrapNode) streams() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.streamScripts...)
}

func firstStreamEventIndex(events []string) int {
	return indexOf(events, "stream")
}

func indexOf(events []string, want string) int {
	for i, e := range events {
		if e == want {
			return i
		}
	}
	return -1
}
