package cli

import (
	"context"
	"encoding/json"

	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// writeRun builds a durable install run with the given phase states.
func writeRun(t *testing.T, dir, status string, phases map[string]string) string {
	t.Helper()
	store := toolkit.RunStore{Dir: filepath.Join(dir, "run")}
	if _, err := store.Create("run-abc", "sha256:"+strings.Repeat("1", 64), "sha256:"+strings.Repeat("2", 64), "existing", []string{"namespace/hnb-system"}); err != nil {
		t.Fatal(err)
	}
	run, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for name, state := range phases {
		phase := toolkit.PhaseName(name)
		if state == toolkit.PhaseSucceededState {
			if _, err := store.BeginPhase("writer-a", run.Epoch, phase, func(context.Context) (bool, string, error) { return true, "", nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CompletePhase("writer-a", run.Epoch, phase, func(context.Context) (bool, string, error) { return true, "", nil }, nil); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := store.FailPhase("writer-a", run.Epoch, phase, "TEST_"+strings.ToUpper(state)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Finalize("writer-a", run.Epoch, status, nil); err != nil {
		t.Fatal(err)
	}
	return store.Dir
}

// 集群 Ready 但平台失败 -> status 必须非成功，且不得报告完整交付。
func TestStatusNeverReportsSuccessWhenPlatformLayerFails(t *testing.T) {
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseFailedState, map[string]string{
		string(toolkit.PhaseNodePrereq):       toolkit.PhaseSucceededState,
		string(toolkit.PhaseInit):             toolkit.PhaseSucceededState,
		string(toolkit.PhaseJoin):             toolkit.PhaseSucceededState,
		string(toolkit.PhaseNetwork):          toolkit.PhaseSucceededState,
		string(toolkit.PhaseClusterVerified):  toolkit.PhaseSucceededState,
		string(toolkit.PhasePlatformRelease):  toolkit.PhaseSucceededState,
		string(toolkit.PhasePlatformVerified): toolkit.PhaseFailedState,
	})
	code, out, errOut := runCLI(t, "status", "--run-dir", runDir, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("platform failure reported success: %s %s", out, errOut)
	}
	var report toolkit.DeliveryReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report %q: %v", out, err)
	}
	if report.CompleteDelivery {
		t.Fatal("cluster Ready + platform failure reported completeDelivery")
	}
	if report.Overall == toolkit.DeliverySuccess {
		t.Fatal("overall success with a failed platform layer")
	}
	// The cluster pass and platform failure are both visible.
	if len(report.Layers) != len(toolkit.DeliveryLayers) {
		t.Fatalf("expected all layers: %+v", report.Layers)
	}
	cluster, platform := report.Layers[1], report.Layers[2]
	if cluster.Layer != toolkit.LayerCluster || cluster.State != toolkit.LayerPassed {
		t.Fatalf("cluster layer mislabeled: %+v", cluster)
	}
	if platform.Layer != toolkit.LayerPlatform || platform.State != toolkit.LayerFailed {
		t.Fatalf("platform layer mislabeled: %+v", platform)
	}
}

// A fully verified run is the only case that reports complete delivery.
func TestStatusReportsSuccessOnlyWhenEveryLayerVerified(t *testing.T) {
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseSucceededState, map[string]string{
		string(toolkit.PhaseNodePrereq):       toolkit.PhaseSucceededState,
		string(toolkit.PhaseRuntime):          toolkit.PhaseSucceededState,
		string(toolkit.PhaseInit):             toolkit.PhaseSucceededState,
		string(toolkit.PhaseJoin):             toolkit.PhaseSucceededState,
		string(toolkit.PhaseNetwork):          toolkit.PhaseSucceededState,
		string(toolkit.PhaseClusterVerified):  toolkit.PhaseSucceededState,
		string(toolkit.PhaseIdentity):         toolkit.PhaseSucceededState,
		string(toolkit.PhaseInfrastructure):   toolkit.PhaseSucceededState,
		string(toolkit.PhaseDatabaseMigrate):  toolkit.PhaseSucceededState,
		string(toolkit.PhasePlatformRelease):  toolkit.PhaseSucceededState,
		string(toolkit.PhasePlatformVerified): toolkit.PhaseSucceededState,
		string(toolkit.PhaseHandoff):          toolkit.PhaseSucceededState,
	})
	code, out, errOut := runCLI(t, "status", "--run-dir", runDir)
	if code != ExitSuccess {
		t.Fatalf("fully verified run must report success: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "completeDelivery=true") {
		t.Fatalf("human output must state complete delivery: %s", out)
	}
}

// status refuses a run id that does not match and never leaks secrets.
func TestStatusRejectsMismatchedRunIDAndHidesSecrets(t *testing.T) {
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseFailedState, map[string]string{string(toolkit.PhaseInit): toolkit.PhaseSucceededState})
	code, out, errOut := runCLI(t, "status", "--run-dir", runDir, "--run-id", "other-run")
	if code == ExitSuccess || !strings.Contains(errOut, "does not match") {
		t.Fatalf("mismatched run id accepted: %d %s", code, errOut)
	}
	// Missing run dir -> invalid config, not success.
	code, _, errOut = runCLI(t, "status", "--run-dir", filepath.Join(dir, "absent"))
	if code == ExitSuccess {
		t.Fatalf("absent run dir reported success: %s", errOut)
	}
	// Human output carries no credential-shaped content.
	_, out, _ = runCLI(t, "status", "--run-dir", runDir)
	for _, forbidden := range []string{"password", "BEGIN EC PRIVATE KEY", "dsn=", "kubeconfig"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("status output leaked %q: %s", forbidden, out)
		}
	}
}

// resume emits a recovery plan and never claims success; it also refuses an
// already-succeeded run.
func TestResumeEmitsPlanAndRefusesSuccess(t *testing.T) {
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseFailedState, map[string]string{
		string(toolkit.PhaseNodePrereq): toolkit.PhaseSucceededState,
		string(toolkit.PhaseInit):       toolkit.PhaseSucceededState,
	})
	code, out, errOut := runCLI(t, "resume", "--run-dir", runDir, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("resume reported success: %s", out)
	}
	if !strings.Contains(errOut, "certified install route") {
		t.Fatalf("resume must gate on the certified route: %s", errOut)
	}
	var doc struct {
		RunID  string                 `json:"runId"`
		Status string                 `json:"status"`
		Resume []toolkit.ResumeAction `json:"resume"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid resume JSON %q: %v", out, err)
	}
	byPhase := map[toolkit.PhaseName]string{}
	for _, a := range doc.Resume {
		byPhase[a.Phase] = a.Action
	}
	if byPhase[toolkit.PhaseInit] != "recheck_or_refuse" {
		t.Fatalf("completed init must not be re-executed on resume: %+v", byPhase)
	}
	// No reset action anywhere.
	for _, a := range doc.Resume {
		if strings.Contains(strings.ToLower(a.Action), "reset") {
			t.Fatalf("resume plan offers a reset action: %+v", a)
		}
	}
	// An already-succeeded run is not resumable.
	okDir := writeRun(t, filepath.Join(dir, "ok"), toolkit.PhaseSucceededState, map[string]string{string(toolkit.PhaseInit): toolkit.PhaseSucceededState})
	if code, _, _ := runCLI(t, "resume", "--run-dir", okDir); code == ExitSuccess {
		t.Fatal("resume of a succeeded run must not report success")
	}
}

// resume always requires an explicit run directory. status resolves the recorded
// run from the install history, so it only fails when no run has been recorded.
func TestResumeRequiresRunDirAndStatusResolvesHistory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	code, _, errOut := runCLI(t, "resume")
	if code == ExitSuccess || !strings.Contains(errOut, "--run-dir") {
		t.Fatalf("resume without --run-dir: %d %s", code, errOut)
	}

	code, _, errOut = runCLI(t, "status")
	if code == ExitSuccess || !strings.Contains(errOut, "no matching install run") {
		t.Fatalf("status without any recorded run: %d %s", code, errOut)
	}
}

// status resolves the latest recorded run, so an operator can watch progress
// without remembering the run directory.
func TestStatusResolvesLatestRecordedRun(t *testing.T) {
	runsRoot := filepath.Join(t.TempDir(), "runs")
	t.Setenv("XDG_STATE_HOME", filepath.Dir(runsRoot))
	runDir := writeRun(t, runsRoot, toolkit.PhaseBlockedState, map[string]string{
		string(toolkit.PhasePlanning): toolkit.PhaseSucceededState,
	})
	code, out, errOut := runCLI(t, "status", "--runs-dir", runsRoot)
	if code == ExitSuccess {
		t.Fatalf("a blocked run must not verify: %s", out)
	}
	if !strings.Contains(errOut, "blocked") {
		t.Fatalf("status must report the recorded state: %s", errOut)
	}
	if _, err := toolkit.LoadRunState(runDir); err != nil {
		t.Fatalf("fixture run must stay readable: %v", err)
	}
}
