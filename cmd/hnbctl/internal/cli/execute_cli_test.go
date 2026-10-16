package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// writeChartPackage writes a minimal packaged Chart whose root metadata and
// selection overlay satisfy the locked renderer without contacting a registry.
func writeChartPackage(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("hnb/Chart.yaml", "apiVersion: v2\nname: hnb\nversion: 0.1.0\n")
	write("hnb/overlays/minimal.yaml", "global:\n  imageRegistry: \"\"\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestShortcutExecuteAppliesCertifiedExistingRoute drives the certified
// existing-cluster route through the real CLI with a fixture cluster runner and
// a fixture Helm, proving the applier pipeline records release+verified phases
// and finalises the run as succeeded only after the gate passes.
func TestShortcutExecuteAppliesCertifiedExistingRoute(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, bin, "helm", "#!/bin/sh\ncat <<'EOF'\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hnb-fixture\ndata:\n  ok: \"true\"\nEOF\n")
	writeExecutable(t, bin, "kubectl", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	chart := filepath.Join(dir, "hnb.tgz")
	writeChartPackage(t, chart)
	chartBytes, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	chartDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(chartBytes))

	kubeconfig := filepath.Join(dir, "fixture-kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte("fixture-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, "release.lock.yaml")
	lockBody := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\n" +
		"images:\n  - name: portal\n    ref: registry.invalid/portal:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\n    manifestDigest: sha256:" + strings.Repeat("c", 64) + "\n    platform: linux/amd64\n" +
		"charts:\n  - name: hnb\n    version: 0.1.0\n    digest: " + chartDigest + "\n" +
		"compatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	if err := os.WriteFile(lock, []byte(lockBody), 0o600); err != nil {
		t.Fatal(err)
	}
	bindings := filepath.Join(dir, "bindings.yaml")
	if err := os.WriteFile(bindings, []byte("portal:\n  db:\n    existingSecret: hnb-platform-pg\n    secretKey: dsn\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(dir, "runs")
	args := []string{"install", "--kubeconfig", kubeconfig, "--domain", "fixture.invalid",
		"--tls-secret", "fixture-tls", "--storage-class", "fixture-storage",
		"--lock", lock, "--runs-dir", root, "--format", "json",
		"--execute", "--chart-package", chart, "--bindings", bindings,
		"--workload", "Deployment/portal"}
	code, out, errOut := runCLI(t, args...)
	if code != ExitSuccess {
		t.Fatalf("certified execute failed: %d %s %s", code, out, errOut)
	}
	var plan shortcutPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.ExecutionAuthorized {
		t.Fatalf("execute did not authorize: %s", out)
	}
	run, err := toolkit.LoadRunState(plan.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != toolkit.PhaseSucceededState {
		t.Fatalf("run not finalized succeeded: %s", run.Status)
	}
	for _, name := range []toolkit.PhaseName{toolkit.PhasePlatformRelease, toolkit.PhasePlatformVerified} {
		if p := run.Phases[string(name)]; p == nil || p.State != toolkit.PhaseSucceededState {
			t.Fatalf("phase %s not succeeded: %+v", name, p)
		}
	}
}
