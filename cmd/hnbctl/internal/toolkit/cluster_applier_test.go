package toolkit

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClusterRunner struct {
	mu         sync.Mutex
	calls      [][]string
	applyErr   error
	rolloutErr error
}

func (r *fakeClusterRunner) Run(_ context.Context, kubeconfig string, stdin io.Reader, args ...string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string{kubeconfig}, args...))
	r.mu.Unlock()
	if len(args) > 0 && args[0] == "apply" {
		if stdin != nil {
			_, _ = io.ReadAll(stdin)
		}
		if r.applyErr != nil {
			return "", r.applyErr
		}
		return "applied", nil
	}
	for _, a := range args {
		if a == "rollout" {
			if r.rolloutErr != nil {
				return "", r.rolloutErr
			}
			return "ready", nil
		}
	}
	return "", nil
}

func (r *fakeClusterRunner) callArgs() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func certifiedMatrix() *EvidenceMatrix {
	return &EvidenceMatrix{
		Version:       "test",
		CertifiedOS:   []string{"ubuntu-24.04"},
		CertifiedArch: []string{"amd64"},
		Profiles:      []EvidenceProfile{{Name: string(ProfileMinimal), Status: "certified"}},
		Routes:        []EvidenceProfile{{Name: string(ModeExisting), Status: "certified"}},
	}
}

// blockedMatrix keeps an explicit "gated" scenario independent of the current
// global evidence, whose existing-route Minimal state may legitimately change.
func blockedMatrix() *EvidenceMatrix {
	return &EvidenceMatrix{
		Version:  "test",
		Profiles: []EvidenceProfile{{Name: string(ProfileMinimal), Status: "blocked", Reasons: []string{"test"}}},
		Routes:   []EvidenceProfile{{Name: string(ModeExisting), Status: "blocked", Reasons: []string{"test"}}},
	}
}

func existingApplierConfig() *DeploymentConfig {
	return &DeploymentConfig{Spec: ConfigSpec{
		Mode:      ModeExisting,
		Profile:   ProfileMinimal,
		Cluster:   ClusterConfig{Strategy: StrategyExisting, KubeconfigFile: "/tmp/kc"},
		Platform:  PlatformConfig{Namespace: "hnb-system"},
		Artifacts: ArtifactsConfig{Mode: ArtifactsOnline},
		Timeouts:  TimeoutConfig{Readiness: time.Minute},
	}}
}

func newApplierRun(t *testing.T) (RunStore, string, int64) {
	t.Helper()
	store := RunStore{Dir: t.TempDir() + "/run"}
	if _, err := store.Create("run-apply-1", "sha256:cfg", "sha256:lock", "existing", nil); err != nil {
		t.Fatal(err)
	}
	writer := "install-writer"
	run, err := store.AcquireWriter(writer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return store, writer, run.Epoch
}

// 门禁未认证时必须拒绝，且不调用任何集群命令.
func TestRunPlatformInstallRefusesWithoutEvidence(t *testing.T) {
	store, writer, epoch := newApplierRun(t)
	runner := &fakeClusterRunner{}
	_, err := RunPlatformInstall(context.Background(), store, writer, epoch,
		existingApplierConfig(), "/tmp/kc", []byte("kind: Deployment"), nil, runner, blockedMatrix())
	if err == nil || !strings.Contains(err.Error(), "PLATFORM_NOT_CERTIFIED") {
		t.Fatalf("uncertified pipeline not refused: %v", err)
	}
	if len(runner.callArgs()) != 0 {
		t.Fatalf("cluster contacted while gated: %v", runner.callArgs())
	}
}

// 已认证证据下应用清单并等待 rollout，阶段被记录.
func TestRunPlatformInstallAppliesAndVerifies(t *testing.T) {
	store, writer, epoch := newApplierRun(t)
	runner := &fakeClusterRunner{}
	result, err := RunPlatformInstall(context.Background(), store, writer, epoch,
		existingApplierConfig(), "/tmp/kc", []byte("kind: Deployment\n"), []WorkloadRef{{Kind: "deployment", Name: "apiserver"}}, runner, certifiedMatrix())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.Verified || result.Namespace != "hnb-system" {
		t.Fatalf("pipeline result incomplete: %+v", result)
	}
	calls := runner.callArgs()
	if len(calls) != 2 {
		t.Fatalf("expected apply + rollout, got %v", calls)
	}
	if !containsStr(calls[0], "apply") || !containsStr(calls[0], "hnb-system") {
		t.Fatalf("apply call wrong: %v", calls[0])
	}
	if !containsStr(calls[1], "rollout") || !containsStr(calls[1], "deployment/apiserver") {
		t.Fatalf("rollout call wrong: %v", calls[1])
	}
	run, err := LoadRunState(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []PhaseName{PhasePlatformRelease, PhasePlatformVerified} {
		if state := run.Phases[string(phase)]; state == nil || state.State != PhaseSucceededState {
			t.Fatalf("phase %s not recorded as succeeded: %+v", phase, state)
		}
	}
	// 分层结果仍是 partial：迁移未执行，平台层不得显示为已完整交付.
	report := NewDeliveryReport(run.RunID, LayerOutcomesFromRun(run))
	if report.CompleteDelivery || report.Overall == DeliverySuccess {
		t.Fatalf("apply alone must not be complete delivery: %+v", report)
	}
}

// rollout 失败必须作为阶段失败，且不能记录平台层通过.
func TestRunPlatformInstallRolloutFailureFailsClosed(t *testing.T) {
	store, writer, epoch := newApplierRun(t)
	runner := &fakeClusterRunner{rolloutErr: io.ErrUnexpectedEOF}
	result, err := RunPlatformInstall(context.Background(), store, writer, epoch,
		existingApplierConfig(), "/tmp/kc", []byte("kind: Deployment\n"), []WorkloadRef{{Kind: "deployment", Name: "apiserver"}}, runner, certifiedMatrix())
	if err == nil {
		t.Fatal("rollout failure reported success")
	}
	if !result.Applied || result.Verified {
		t.Fatalf("unexpected result on rollout failure: %+v", result)
	}
	run, loadErr := LoadRunState(store.Dir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if state := run.Phases[string(PhasePlatformVerified)]; state == nil || state.State != PhaseFailedState {
		t.Fatalf("rollout failure not recorded: %+v", state)
	}
	if state := run.Phases[string(PhasePlatformRelease)]; state == nil || state.State != PhaseSucceededState {
		t.Fatalf("release phase should have succeeded before rollout: %+v", state)
	}
}

func TestPlatformApplierRejectsInvalidInput(t *testing.T) {
	runner := &fakeClusterRunner{}
	applier := PlatformApplier{Runner: runner}
	if err := applier.Apply(context.Background()); err == nil {
		t.Fatal("empty applier input accepted")
	}
	applier = PlatformApplier{Runner: runner, Kubeconfig: "/kc", Namespace: "ns", Manifests: []byte("x")}
	if err := applier.Verify(context.Background()); err != nil {
		t.Fatalf("verify with no workloads should be a no-op: %v", err)
	}
	applier.Workloads = []WorkloadRef{{Kind: "deployment"}}
	if err := applier.Verify(context.Background()); err == nil {
		t.Fatal("workload without name accepted")
	}
}
