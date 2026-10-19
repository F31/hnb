package observer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestObserverStateRoundTripAndPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if _, ok, err := LoadState(path); err != nil || ok {
		t.Fatalf("missing state = %v/%v", ok, err)
	}
	if err := SaveState(path, State{Generation: 2, NextSequence: 7}); err != nil {
		t.Fatal(err)
	}
	state, ok, err := LoadState(path)
	if err != nil || !ok || state.Generation != 2 || state.NextSequence != 7 {
		t.Fatalf("state = %+v/%v/%v", state, ok, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadState(path); err == nil {
		t.Fatal("world-readable state was accepted")
	}
}
