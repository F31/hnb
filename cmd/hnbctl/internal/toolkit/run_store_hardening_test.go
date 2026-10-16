package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedSucceededRun(t *testing.T, store RunStore) *InstallRun {
	t.Helper()
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	writer, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginPhase("writer-a", writer.Epoch, PhaseClusterVerified, always(true, "pre")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompletePhase("writer-a", writer.Epoch, PhaseClusterVerified, always(true, "post"), nil); err != nil {
		t.Fatal(err)
	}
	done, err := store.Finalize("writer-a", writer.Epoch, PhaseSucceededState, []PhaseName{PhaseClusterVerified})
	if err != nil {
		t.Fatal(err)
	}
	return done
}

// A lease must be exclusive of its expiry instant, not one tick after it.
func TestRunWriterLeaseExpiresAtBoundary(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	writer, err := store.AcquireWriter("writer-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expiry := writer.LeaseExpires
	run, err := store.read()
	if err != nil {
		t.Fatal(err)
	}

	// One nanosecond before expiry the writer is still valid.
	store.NowFunc = func() time.Time { return expiry.Add(-time.Nanosecond) }
	if err := store.assertWriter(run, "writer-a", writer.Epoch); err != nil {
		t.Fatalf("writer rejected just before expiry: %v", err)
	}

	// Exactly at expiry it is fenced.
	store.NowFunc = func() time.Time { return expiry }
	if err := store.assertWriter(run, "writer-a", writer.Epoch); !errors.Is(err, errRunReason(RunReasonFenced)) &&
		!strings.Contains(err.Error(), RunReasonFenced) {
		t.Fatalf("expiry boundary not fenced: %v", err)
	}
}

func errRunReason(reason string) error { return errors.New(reason) }

// A completed run must refuse every mutation, including taking a lease.
func TestRunCompletedRefusesLeaseAndFinalize(t *testing.T) {
	store := fixedStore(t)
	seedSucceededRun(t, store)

	if _, err := store.AcquireWriter("writer-b", time.Hour); err == nil ||
		!strings.Contains(err.Error(), RunReasonTerminalState) {
		t.Fatalf("lease taken on a completed run: %v", err)
	}
	if _, err := store.Finalize("writer-a", 1, PhaseSucceededState, []PhaseName{PhaseClusterVerified}); err == nil ||
		!strings.Contains(err.Error(), RunReasonTerminalState) {
		t.Fatalf("finalize accepted on a completed run: %v", err)
	}
	run, err := store.read()
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != PhaseSucceededState {
		t.Fatalf("completed run was modified: %s", run.Status)
	}
}

// Only exactly 0600/0700 is acceptable; a laxer mode is a finding, not a warning.
func TestRunStoreRequiresExactModes(t *testing.T) {
	store := fixedStore(t)
	if _, err := store.Create("run-1", fixtureConfigDigest, fixtureLockDigest, "existing", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.statePath(), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.statePath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.statePath(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDir(); err == nil || !strings.Contains(err.Error(), RunReasonStateUnsafe) {
		t.Fatalf("group-readable run directory accepted: %v", err)
	}
	if err := os.Chmod(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.statePath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.read(); err == nil || !strings.Contains(err.Error(), RunReasonStateUnsafe) {
		t.Fatalf("world-readable run state accepted: %v", err)
	}
}

// A pre-planted symlink must not become the lock that guards every write.
func TestRunStoreRejectsSymlinkedLock(t *testing.T) {
	store := fixedStore(t)
	if err := store.ensureDir(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(store.Dir, "run-state.lock")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := store.withLock(func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), RunReasonStateUnsafe) {
		t.Fatalf("symlinked lock accepted: %v", err)
	}
}
