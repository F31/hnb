package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// readArchive returns the decompressed text of every entry.
func readArchive(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	out := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[header.Name] = string(data)
	}
	return out
}

// 收集包必须输出身份与采集时间，且包含足够的定位证据。
func TestCollectCLIProducesIdentityAndEvidence(t *testing.T) {
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseFailedState, map[string]string{
		"platform-release":  "failed",
		"platform-verified": "failed",
	})
	output := filepath.Join(t.TempDir(), "diagnostics.tar.gz")
	code, out, errOut := runCLI(t, "collect", "--run-dir", runDir, "--output", output,
		"--reason-codes", "PLATFORM_LOGIN_FAILED", "--remediation-ref", "docs/runbooks/platform-login.md")
	if code != ExitSuccess {
		t.Fatalf("collect failed: %d %s %s", code, out, errOut)
	}
	for _, want := range []string{"bundle=", "contentDigest=sha256:", "collectedAt=", "redacted=true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("collect output missing %q: %s", want, out)
		}
	}
	entries := readArchive(t, output)
	for _, want := range []string{"bundle-identity.json", "install/run-summary.json", "diagnosis/reason-codes.json", "tool/identity.json"} {
		if _, ok := entries[want]; !ok {
			t.Fatalf("archive missing %q: %v", want, entries)
		}
	}
	// Failure localization evidence is retained.
	summary := entries["install/run-summary.json"]
	for _, want := range []string{"platform-release", "platform-verified", "failed"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("run summary missing %q: %s", want, summary)
		}
	}
	if !strings.Contains(entries["diagnosis/reason-codes.json"], "PLATFORM_LOGIN_FAILED") {
		t.Fatalf("reason codes not carried into the bundle: %s", entries["diagnosis/reason-codes.json"])
	}
	fi, err := os.Stat(output)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("archive must be private 0600: %v %v", fi, err)
	}
}

// 收集包不得覆盖已存在的文件。
func TestCollectRefusesToOverwriteArchive(t *testing.T) {
	output := filepath.Join(t.TempDir(), "diagnostics.tar.gz")
	if code, _, _ := runCLI(t, "collect", "--output", output); code != ExitSuccess {
		t.Fatalf("first collect failed: %d", code)
	}
	code, _, _ := runCLI(t, "collect", "--output", output)
	if code == ExitSuccess {
		t.Fatal("collect overwrote an existing archive")
	}
	// The refusal must come from exclusive creation, not from a partial write.
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("existing archive was removed: %v", err)
	}
}

func TestCollectRequiresOutput(t *testing.T) {
	code, _, errOut := runCLI(t, "collect")
	if code == ExitSuccess || !strings.Contains(errOut, "--output") {
		t.Fatalf("collect without --output accepted: %d %s", code, errOut)
	}
}

func TestCollectRejectsUnknownRunDir(t *testing.T) {
	output := filepath.Join(t.TempDir(), "d.tar.gz")
	code, _, _ := runCLI(t, "collect", "--run-dir", filepath.Join(t.TempDir(), "absent"), "--output", output)
	if code == ExitSuccess {
		t.Fatal("collect with an unknown run dir reported success")
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatal("collect wrote an archive despite an invalid run dir")
	}
}
