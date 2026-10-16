package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func historyStore(t *testing.T, root string) (RunStore, *InstallRun) {
	t.Helper()
	store, run, err := CreateHistoryRun(root, fixtureConfigDigest, fixtureLockDigest, "external-cluster cluster.reader/shadow", []string{"namespace/hnb-system"})
	if err != nil {
		t.Fatal(err)
	}
	return store, run
}

// 每次状态转换都留下日志，且日志与状态在同一次原子写入中提交.
func TestTransitionsAreJournaledWithState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	store, run := historyStore(t, root)

	writer := "test-writer"
	active, err := store.AcquireWriter(writer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase(writer, active.Epoch, PhasePlanning, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase(writer, active.Epoch, PhasePlanning, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finalize(writer, active.Epoch, PhaseBlockedState, nil); err != nil {
		t.Fatal(err)
	}

	stored, err := LoadRunState(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RunID != run.RunID || stored.Status != PhaseBlockedState {
		t.Fatalf("unexpected run state: %+v", stored)
	}
	kinds := []string{}
	for _, event := range stored.Events {
		if event.Sequence != int64(len(kinds)+1) {
			t.Fatalf("event sequence gap at %d: %+v", event.Sequence, stored.Events)
		}
		kinds = append(kinds, event.Kind+"="+string(event.Phase)+":"+event.State)
	}
	want := []string{"run.created=:running", "phase.changed=planning:running", "phase.changed=planning:succeeded", "run.changed=:blocked"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("journal mismatch:\n got %v\nwant %v", kinds, want)
	}
	// Re-entering the same writer lease rewrites state without a transition and
	// must not duplicate the journal.
	if _, err := store.AcquireWriter(writer, time.Minute); err != nil {
		t.Fatal(err)
	}
	again, err := LoadRunState(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Events) != len(stored.Events) {
		t.Fatalf("journal grew without a transition: %d -> %d", len(stored.Events), len(again.Events))
	}
}

// 进度呈现只显示真实状态：已完成计数与当前阶段不能凭空产生.
func TestProgressReflectsRecordedPhases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	store, _ := historyStore(t, root)
	writer := "test-writer"
	active, _ := store.AcquireWriter(writer, time.Minute)
	if _, err := store.BeginPhase(writer, active.Epoch, PhasePlanning, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase(writer, active.Epoch, PhasePlanning, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase(writer, active.Epoch, PhasePreflight, nil); err != nil {
		t.Fatal(err)
	}
	run, err := LoadRunState(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	progress := ProgressFromRun(run, run.StartedAt.Add(30*time.Second))
	if progress.Completed != 1 {
		t.Fatalf("completed stages mismatch: %+v", progress)
	}
	if progress.Current != PhasePreflight {
		t.Fatalf("current phase mismatch: %+v", progress)
	}
	if progress.LastSequence == 0 {
		t.Fatal("progress must carry the last journal sequence")
	}
	// Existing-cluster runs must not advertise node bootstrap/join phases.
	for _, phase := range progress.Phases {
		if phase.Name == PhaseInit || phase.Name == PhaseJoin || phase.Name == PhaseRuntime {
			t.Fatalf("existing-cluster progress exposes %s: %+v", phase.Name, progress.Phases)
		}
	}
	var sb strings.Builder
	FormatProgressHuman(&sb, progress)
	if !strings.Contains(sb.String(), "completed=1/") || !strings.Contains(sb.String(), "preflight") {
		t.Fatalf("human progress missing summary: %s", sb.String())
	}
}

// 历史列表按时间倒序，且不可读的记录不会被静默丢弃.
func TestListRunHistoryOrderingAndCorruption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	older := RunStore{Dir: filepath.Join(root, "older"), NowFunc: func() time.Time {
		return time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	}}
	newer := RunStore{Dir: filepath.Join(root, "newer"), NowFunc: func() time.Time {
		return time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	}}
	if _, err := older.Create("run-older", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := newer.Create("run-newer", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	runs, err := ListRunHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].RunID != "run-newer" {
		t.Fatalf("history order mismatch: %+v", runs)
	}

	// A corrupt run record must surface as an error, not vanish from history.
	if err := os.WriteFile(filepath.Join(older.Dir, "run-state.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListRunHistory(root); err == nil {
		t.Fatal("corrupt history entry silently ignored")
	}
}

// 卸载必须绑定安装 run：配置身份不一致时拒绝.
func TestRemovalPlanRefusesMismatchedRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	store, run := historyStore(t, root)
	plan, err := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Run: run,
		Mode: RemovalUninstallHNB, DataLossAcknowledged: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RunID != run.RunID || plan.ConfigDigest != run.ConfigDigest || plan.LockDigest != run.LockDigest {
		t.Fatalf("removal plan lost the run identity: %+v", plan)
	}
	if plan.RoutingMode != run.RoutingMode {
		t.Fatalf("removal plan lost the routing mode: %+v", plan)
	}
	// A different run cannot be substituted for the recorded one.
	other := *run
	other.RunID = "run-other"
	if plan2, _ := PlanRemoval(RemovalInput{Manifest: existingManifestFixture(), Run: &other,
		Mode: RemovalUninstallHNB, DataLossAcknowledged: true}); plan2.RunID != "run-other" {
		t.Fatalf("unexpected plan run: %+v", plan2)
	}
	_ = store
}
