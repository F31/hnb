package iam

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestFileAgentSessionStoreAllowsGroupButRejectsWorld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	credential := AgentSessionCredential{Credential: "token", SessionID: uuid.NewString(), Generation: 1}
	store := FileAgentSessionStore{}
	if err := store.Save(path, credential); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Load(path); err != nil || !ok {
		t.Fatalf("group-readable session rejected: %v/%v", ok, err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(path); err == nil {
		t.Fatal("world-readable session accepted")
	}
}
