package iam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewEnrollmentSecretIsHighEntropyAndHashed(t *testing.T) {
	first, firstHash, err := NewEnrollmentSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := NewEnrollmentSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 43 || len(firstHash) != 64 || len(secondHash) != 64 {
		t.Fatalf("unexpected secret/hash lengths: %d/%d/%d", len(first), len(firstHash), len(secondHash))
	}
	if first == second || firstHash == secondHash {
		t.Fatal("enrollment secrets are not unique")
	}
	if HashEnrollmentSecret(first) != firstHash {
		t.Fatal("enrollment hash is not stable")
	}
}

func TestReadEnrollmentSecretFileRequiresPrivateFileAndValidShape(t *testing.T) {
	dir := t.TempDir()
	secret, _, err := NewEnrollmentSecret()
	if err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadEnrollmentSecretFile(good); err != nil || got != secret {
		t.Fatalf("read = %q err=%v", got, err)
	}
	badMode := filepath.Join(dir, "bad-mode")
	if err := os.WriteFile(badMode, []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollmentSecretFile(badMode); err == nil {
		t.Fatal("group/other-readable secret file was accepted")
	}
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEnrollmentSecretFile(short); err == nil {
		t.Fatal("short secret was accepted")
	}
}
