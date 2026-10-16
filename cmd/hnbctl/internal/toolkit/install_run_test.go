package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedStore(t *testing.T) RunStore {
	t.Helper()
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	return RunStore{Dir: filepath.Join(t.TempDir(), "run"), NowFunc: func() time.Time { return now }}
}

const (
	fixtureConfigDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	fixtureLockDigest   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func always(ok bool, reason string) Condition {
	return func(_ context.Context) (bool, string, error) { return ok, reason, nil }
}

// The run directory must be private and refuse a pre-existing unsafe directory.
func TestRunStoreRequiresPrivateDirectory(t *testing.T) {
	store := fixedStore(t)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(store.Dir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("run directory must be 0700: %v %v", fi, err)
	}
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	stateFi, err := os.Stat(store.statePath())
	if err != nil || stateFi.Mode().Perm() != 0o600 {
		t.Fatalf("run state must be 0600: %v %v", stateFi, err)
	}

	loose := RunStore{Dir: filepath.Join(t.TempDir(), "loose")}
	if err := os.MkdirAll(loose.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := loose.Create("run-2", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err == nil {
		t.Fatal("group/world-accessible run directory accepted")
	}

	linked := RunStore{Dir: filepath.Join(t.TempDir(), "linked")}
	if err := os.MkdirAll(linked.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(t.TempDir(), "real")
	if err := os.Symlink(real, linked.Dir+"-link"); err != nil {
		t.Fatal(err)
	}
	symlinked := RunStore{Dir: linked.Dir + "-link"}
	if _, err := symlinked.Create("run-3", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err == nil {
		t.Fatal("symlinked run directory accepted")
	}
}

// 改锁拒绝: a committed run refuses a different release lock or configuration.
func TestRunStoreRefusesChangedLockAndConfig(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	otherLock := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	if _, err := store.Create("run-1", fixtureConfigDigest, otherLock, "existing", nil); err == nil || !strings.Contains(err.Error(), RunReasonLockChanged) {
		t.Fatalf("changed lock accepted: %v", err)
	}
	otherConfig := "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	if _, err := store.Create("run-1", otherConfig, fixtureLockDigest, "existing", nil); err == nil || !strings.Contains(err.Error(), RunReasonConfigChange) {
		t.Fatalf("changed config accepted: %v", err)
	}
	// Identical immutable inputs resume the same run.
	run, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil)
	if err != nil || run.RunID != "run-1" {
		t.Fatalf("identical inputs did not resume: %+v %v", run, err)
	}
}

// 第二 writer 拒绝: a concurrent writer is refused while the lease is held.
func TestRunStoreRefusesSecondWriterAndFencesTakeover(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	first, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireWriter("writer-b", time.Hour); err == nil || !strings.Contains(err.Error(), RunReasonWriterBusy) {
		t.Fatalf("second concurrent writer accepted: %v", err)
	}
	// The same writer re-entering keeps its epoch.
	again, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil || again.Epoch != first.Epoch {
		t.Fatalf("same writer lost its epoch: %+v %v", again, err)
	}

	// After lease expiry a takeover advances the fencing epoch, and the old
	// writer's later writes are fenced out.
	store.NowFunc = func() time.Time { return time.Now().UTC().Add(2 * time.Hour) }
	second, err := store.AcquireWriter("writer-b", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if second.Epoch <= first.Epoch {
		t.Fatalf("fencing epoch did not advance: %d -> %d", first.Epoch, second.Epoch)
	}
	if _, err := store.BeginPhase("writer-a", first.Epoch, PhaseIdentity, always(true, "")); err == nil || !strings.Contains(err.Error(), RunReasonFenced) {
		t.Fatalf("fenced writer was allowed to mutate state: %v", err)
	}
	if _, err := store.BeginPhase("writer-b", second.Epoch, PhaseIdentity, always(true, "")); err != nil {
		t.Fatalf("current fencing writer refused: %v", err)
	}
}

// 断点恢复不重复 init: succeeded never-rerun phases are recheck_or_refuse, and
// BeginPhase refuses to re-enter them.
func TestResumeNeverRepeatsInitOrJoin(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "bootstrap", nil); err != nil {
		t.Fatal(err)
	}
	run, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	order := []PhaseName{PhaseValidating, PhasePreparing, PhaseInit, PhaseJoin, PhasePlatformRelease}
	for _, name := range order {
		if _, err := store.BeginPhase("writer-a", run.Epoch, name, always(true, "")); err != nil {
			t.Fatalf("begin %s: %v", name, err)
		}
		if _, err := store.CompletePhase("writer-a", run.Epoch, name, always(true, ""), []string{"evidence:" + string(name)}); err != nil {
			t.Fatalf("complete %s: %v", name, err)
		}
	}
	// Re-entering a completed one-shot phase is refused.
	if _, err := store.BeginPhase("writer-a", run.Epoch, PhaseInit, always(true, "")); err == nil || !strings.Contains(err.Error(), RunReasonNeverRerun) {
		t.Fatalf("completed init was re-entered: %v", err)
	}
	if _, err := store.BeginPhase("writer-a", run.Epoch, PhaseJoin, always(true, "")); err == nil || !strings.Contains(err.Error(), RunReasonNeverRerun) {
		t.Fatalf("completed join was re-entered: %v", err)
	}

	state, err := store.read()
	if err != nil {
		t.Fatal(err)
	}
	actions := store.PlanResume(state, order)
	byPhase := map[PhaseName]string{}
	for _, a := range actions {
		byPhase[a.Phase] = a.Action
	}
	if byPhase[PhaseInit] != "recheck_or_refuse" || byPhase[PhaseJoin] != "recheck_or_refuse" {
		t.Fatalf("resume would re-execute one-shot phases: %+v", actions)
	}
	for _, name := range []PhaseName{PhaseValidating, PhasePreparing, PhasePlatformRelease} {
		if byPhase[name] != "skip_after_recheck" {
			t.Fatalf("idempotent phase %s should be skipped after recheck, got %q", name, byPhase[name])
		}
	}
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.Action), "reset") {
			t.Fatalf("resume plan offers a reset action: %+v", a)
		}
	}
}

// A failed phase is re-executed on resume; a blocked precondition stays blocked.
func TestResumeReExecutesFailedAndRecordsFailedPostcondition(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	run, _ := store.AcquireWriter("writer-a", time.Hour)

	// Precondition failure blocks before any work.
	if _, err := store.BeginPhase("writer-a", run.Epoch, PhasePreflight, always(false, "API_UNREACHABLE")); err == nil {
		t.Fatal("failed precondition still began the phase")
	}
	state, _ := store.read()
	if state.Phases[string(PhasePreflight)].State != PhaseBlockedState {
		t.Fatalf("precondition failure must block, got %+v", state.Phases[string(PhasePreflight)])
	}

	// Postcondition failure does not record success.
	if _, err := store.BeginPhase("writer-a", run.Epoch, PhasePlatformRelease, always(true, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase("writer-a", run.Epoch, PhasePlatformRelease, always(false, "PLATFORM_NOT_READY"), nil); err == nil {
		t.Fatal("failed postcondition recorded success")
	}
	state, _ = store.read()
	release := state.Phases[string(PhasePlatformRelease)]
	if release.State != PhaseFailedState || release.ReasonCode != "PLATFORM_NOT_READY" {
		t.Fatalf("postcondition failure not recorded: %+v", release)
	}
	// Finalizing as succeeded is refused while a phase is unsucceeded.
	if _, err := store.Finalize("writer-a", run.Epoch, PhaseSucceededState, []PhaseName{PhasePlatformRelease, PhasePlatformVerified}); err == nil {
		t.Fatal("run finalized as succeeded with unsucceeded phases")
	}
	// Resume re-executes a failed phase.
	actions := store.PlanResume(state, []PhaseName{PhasePreflight, PhasePlatformRelease})
	for _, a := range actions {
		if a.Action != "execute" {
			t.Fatalf("failed/blocked phases must be re-executed: %+v", actions)
		}
	}
}

// Finalize succeeds only when every required phase is verified.
func TestFinalizeRequiresVerifiedPhases(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	run, _ := store.AcquireWriter("writer-a", time.Hour)
	for _, name := range []PhaseName{PhasePreparing, PhaseDatabaseMigrate, PhasePlatformRelease} {
		if _, err := store.BeginPhase("writer-a", run.Epoch, name, always(true, "")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompletePhase("writer-a", run.Epoch, name, always(true, ""), nil); err != nil {
			t.Fatal(err)
		}
	}
	finished, err := store.Finalize("writer-a", run.Epoch, PhaseSucceededState, []PhaseName{PhasePreparing, PhaseDatabaseMigrate, PhasePlatformRelease})
	if err != nil || finished.Status != PhaseSucceededState || finished.FinishedAt.IsZero() {
		t.Fatalf("verified run not finalized: %+v %v", finished, err)
	}
	// A run cannot be mutated by a stale writer after finalization.
	if _, err := store.BeginPhase("writer-a", run.Epoch, PhaseHandoff, always(true, "")); err == nil {
		t.Fatal("post-finalize phase should be refused")
	}
}
