package toolkit

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// An unrecognized phase state must never be planned as "execute": the recorded
// precondition cannot be trusted, so the resume has to recheck or refuse.
func TestPlanResumeRefusesUnrecognizedPhaseState(t *testing.T) {
	run := &InstallRun{Phases: map[string]*RunPhase{
		string(PhaseInit):            {State: "half-applied"},
		string(PhaseClusterVerified): {State: ""},
		string(PhaseHandoff):         {State: PhaseSucceededState},
	}}
	actions := (&RunStore{}).PlanResume(run, []PhaseName{PhaseInit, PhaseClusterVerified, PhaseHandoff})
	byPhase := map[string]ResumeAction{}
	for _, action := range actions {
		byPhase[string(action.Phase)] = action
	}
	for _, name := range []PhaseName{PhaseInit, PhaseClusterVerified} {
		action := byPhase[string(name)]
		if action.Action != "recheck_or_refuse" {
			t.Fatalf("%s planned as %q, want recheck_or_refuse", name, action.Action)
		}
		if !strings.Contains(action.Reason, "unrecognized phase state") {
			t.Fatalf("%s reason does not explain the refusal: %q", name, action.Reason)
		}
	}
	if byPhase[string(PhaseHandoff)].Action != "skip_after_recheck" {
		t.Fatalf("succeeded phase changed behaviour: %q", byPhase[string(PhaseHandoff)].Action)
	}
}

// A contended lock must time out with a reason instead of hanging forever.
func TestRunLockWaitIsBounded(t *testing.T) {
	store := fixedStore(t)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	// Hold the lock from another descriptor for longer than the caller's budget.
	holder, err := os.OpenFile(store.Dir+"/run-state.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	done := make(chan error, 1)
	go func() { done <- store.withLock(func() error { return nil }) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("contended lock was acquired")
		}
	case <-time.After(runLockTimeout + 5*time.Second):
		t.Fatal("withLock blocked past its bounded wait")
	}
}

// Releasing the lock lets the next writer in, so the bound is not a deadlock.
func TestRunLockIsReleasedAfterUse(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.withLock(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	again := make(chan error, 1)
	go func() { again <- store.withLock(func() error { return nil }) }()
	select {
	case err := <-again:
		if err != nil {
			t.Fatalf("lock not reusable: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock stayed held after the section finished")
	}
}
