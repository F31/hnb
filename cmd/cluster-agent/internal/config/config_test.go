package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadKubeTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("  service-account-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := readKubeTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if token != "service-account-token" {
		t.Fatalf("unexpected token %q", token)
	}
}

func TestReadKubeTokenFileRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readKubeTokenFile(path); err == nil {
		t.Fatal("expected empty token file to fail")
	}
}
