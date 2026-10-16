package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type offlineForbiddenTransport struct{ calls int }

func (t *offlineForbiddenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, fmt.Errorf("network forbidden during bundle lifecycle")
}

func TestBundleLifecycleNeverUsesHTTP(t *testing.T) {
	lock, source, index, sandbox := writeBundleFixture(t)
	manifest := filepath.Join(t.TempDir(), "bundle.json")
	target := filepath.Join(t.TempDir(), "imported")
	transport := &offlineForbiddenTransport{}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previous }()
	for _, args := range [][]string{
		{"bundle", "prepare", "--lock", lock, "--cache-dir", source, "--manifest", manifest, "--os-package-index", index, "--sandbox-image", sandbox},
		{"bundle", "verify", "--lock", lock, "--cache-dir", source, "--manifest", manifest},
		{"bundle", "import", "--lock", lock, "--cache-dir", source, "--manifest", manifest, "--target-dir", target},
		{"bundle", "verify", "--lock", filepath.Join(target, "release.lock.json"), "--cache-dir", target, "--manifest", filepath.Join(target, "bundle.json")},
	} {
		if code, out, stderr := runCLI(t, args...); code != ExitSuccess {
			t.Fatalf("offline step failed: %v %d %s %s", args, code, out, stderr)
		}
	}
	if transport.calls != 0 {
		t.Fatal("bundle lifecycle attempted HTTP fallback")
	}
}

func TestBundleImportRequiresActualDestination(t *testing.T) {
	lock, source, index, sandbox := writeBundleFixture(t)
	manifest := filepath.Join(t.TempDir(), "bundle.json")
	if code, _, stderr := runCLI(t, "bundle", "prepare", "--lock", lock, "--cache-dir", source, "--manifest", manifest, "--os-package-index", index, "--sandbox-image", sandbox); code != ExitSuccess {
		t.Fatal(stderr)
	}
	if code, _, _ := runCLI(t, "bundle", "import", "--lock", lock, "--cache-dir", source, "--manifest", manifest); code == ExitSuccess {
		t.Fatal("import without destination reported success")
	}
	// Damaged source must fail before any destination side effect.
	if err := os.WriteFile(index, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "must-not-exist")
	if code, _, _ := runCLI(t, "bundle", "import", "--lock", lock, "--cache-dir", source, "--manifest", manifest, "--target-dir", target); code == ExitSuccess {
		t.Fatal("corrupt source imported")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed validation created destination")
	}
}
