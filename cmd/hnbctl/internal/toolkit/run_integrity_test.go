package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunCreatePreservesCorruptState(t *testing.T) {
	store := fixedStore(t)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	bad := []byte("{interrupted-state")
	if err := os.WriteFile(store.statePath(), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("new-run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err == nil {
		t.Fatal("corrupt state was replaced by a new run")
	}
	data, err := os.ReadFile(store.statePath())
	if err != nil || string(data) != string(bad) {
		t.Fatalf("state changed: %v", err)
	}
}

func TestRunRejectsChangedRunIdentityAndOwnership(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("original", fixtureConfigDigest, fixtureLockDigest, "existing", []string{"namespace/a"}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		id, route string
		ownership []string
	}{
		{"other", "existing", []string{"namespace/a"}},
		{"original", "bootstrap", []string{"namespace/a"}},
		{"original", "existing", []string{"namespace/b"}},
	} {
		if _, err := store.Create(change.id, fixtureConfigDigest, fixtureLockDigest, change.route, change.ownership); err == nil {
			t.Fatal("changed installation identity accepted")
		}
	}
}

func TestRunReadRejectsUnsafeStateAndDoesNotCreateDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	if _, err := LoadRunState(dir); err == nil {
		t.Fatal("absent state accepted")
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("status read created a directory")
	}
	for _, mode := range []os.FileMode{0o644, 0o660} {
		store := fixedStore(t)
		if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(store.statePath(), mode); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRunState(store.Dir); err == nil {
			t.Fatal("unsafe state permissions accepted")
		}
	}
	store := fixedStore(t)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.statePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err == nil {
		t.Fatal("symlinked state accepted or replaced")
	}
	if _, err := os.Readlink(store.statePath()); err != nil {
		t.Fatal("symlink was overwritten")
	}
}

func TestRunConcurrentWritersHaveOneWinner(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	const writers = 24
	start := make(chan struct{})
	results := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			_, err := store.AcquireWriter(string(rune('a'+id)), time.Hour)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !strings.Contains(err.Error(), RunReasonWriterBusy) {
			t.Errorf("unexpected writer result: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("writers acquired concurrently: %d", winners)
	}
}

func TestRunLeaseExpiryAndTerminalStateRefuseMutation(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireWriter("writer", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now := store.now()
	store.NowFunc = func() time.Time { return now.Add(2 * time.Second) }
	if _, err := store.BeginPhase("writer", lease.Epoch, PhasePreparing, always(true, "")); err == nil {
		t.Fatal("expired writer mutated the run")
	}
	renewed, err := store.AcquireWriter("writer", time.Hour)
	if err != nil || renewed.Epoch <= lease.Epoch {
		t.Fatalf("expired token reused: %+v %v", renewed, err)
	}
	if _, err := store.BeginPhase("writer", renewed.Epoch, PhasePreparing, always(true, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase("writer", renewed.Epoch, PhasePreparing, always(true, ""), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finalize("writer", renewed.Epoch, PhaseSucceededState, []PhaseName{PhasePreparing}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase("writer", renewed.Epoch, PhaseHandoff, always(true, "")); err == nil {
		t.Fatal("completed run mutated")
	}
}

func TestInterruptedOneShotRequiresRecheckInsteadOfReplay(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "bootstrap", nil); err != nil {
		t.Fatal(err)
	}
	run, err := store.AcquireWriter("writer", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase("writer", run.Epoch, PhaseInit, always(true, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FailPhase("writer", run.Epoch, PhaseInit, "INTERRUPTED"); err != nil {
		t.Fatal(err)
	}
	state, err := store.read()
	if err != nil {
		t.Fatal(err)
	}
	if action := store.PlanResume(state, []PhaseName{PhaseInit})[0].Action; action != "recheck_or_refuse" {
		t.Fatalf("partially initialized cluster would replay: %s", action)
	}
	if _, err := store.BeginPhase("writer", run.Epoch, PhaseInit, always(true, "")); err == nil {
		t.Fatal("one-shot replay accepted")
	}
}

func TestPhaseBudgetCoversPreExecAndPostChecks(t *testing.T) {
	for _, stage := range []string{"pre", "exec", "post"} {
		t.Run(stage, func(t *testing.T) {
			store := fixedStore(t)
			if _, err := store.Create("run", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
				t.Fatal(err)
			}
			run, err := store.AcquireWriter("writer", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			late := func(ctx context.Context) (bool, string, error) {
				if _, ok := ctx.Deadline(); !ok {
					return false, "MISSING_DEADLINE", errors.New("check is unbounded")
				}
				<-ctx.Done()
				return true, "", nil // A late positive result must not count as success.
			}
			pre, post := always(true, ""), always(true, "")
			if stage == "pre" {
				pre = late
			}
			if stage == "post" {
				post = late
			}
			workRan := false
			err = store.RunPhaseWithTimeout(context.Background(), "writer", run.Epoch, PhasePreparing,
				20*time.Millisecond, pre, post, func(ctx context.Context) error {
					workRan = true
					if stage == "exec" {
						<-ctx.Done()
					}
					return nil
				})
			if err == nil || !strings.Contains(err.Error(), "RUN_PHASE_TIMEOUT") {
				t.Fatalf("late %s passed: %v", stage, err)
			}
			if stage == "pre" && workRan {
				t.Fatal("execution ran after a timed-out prerequisite")
			}
			state, err := store.read()
			if err != nil {
				t.Fatal(err)
			}
			if state.Phases[string(PhasePreparing)].State != PhaseFailedState {
				t.Fatal("timeout did not fail the phase")
			}
		})
	}
}

func TestLayerStatusDoesNotHideEarlierFailuresOrMissingChecks(t *testing.T) {
	for _, first := range []string{PhaseFailedState, PhaseBlockedState, PhasePendingState, PhaseRunningState, "missing", "unknown"} {
		run := &InstallRun{Phases: map[string]*RunPhase{
			string(PhasePlatformVerified): {Name: PhasePlatformVerified, State: PhaseSucceededState},
		}}
		if first != "missing" {
			run.Phases[string(PhaseDatabaseMigrate)] = &RunPhase{Name: PhaseDatabaseMigrate, State: first}
		}
		if outcome := LayerOutcomesFromRun(run)[LayerPlatform]; outcome.State == LayerPassed {
			t.Fatalf("later verification masked %s migration: %+v", first, outcome)
		}
	}
	run := &InstallRun{Phases: map[string]*RunPhase{
		string(PhasePlatformRelease): {Name: PhasePlatformRelease, State: PhaseSucceededState},
	}}
	if LayerOutcomesFromRun(run)[LayerPlatform].State == LayerPassed {
		t.Fatal("release install alone verified platform")
	}
}
