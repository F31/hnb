package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestHelpListsCommands(t *testing.T) {
	code, out, _ := runCLI(t, "help")
	if code != ExitSuccess {
		t.Fatalf("expected success, got %d", code)
	}
	for _, name := range []string{"init", "preflight", "install", "cluster", "collect"} {
		if !strings.Contains(out, name) {
			t.Fatalf("help missing %q", name)
		}
	}
}

func TestUnknownCommandReturnsUsage(t *testing.T) {
	code, _, err := runCLI(t, "frobnicate")
	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d", code)
	}
	if !strings.Contains(err, "unknown command") {
		t.Fatalf("unexpected error output: %s", err)
	}
}

func TestVersionExitsZero(t *testing.T) {
	code, out, _ := runCLI(t, "version")
	if code != ExitSuccess {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(out, "hnbctl") {
		t.Fatalf("version output missing: %s", out)
	}
}

func TestMatrixShowsCertificationBoundary(t *testing.T) {
	code, out, _ := runCLI(t, "matrix")
	if code != ExitSuccess {
		t.Fatalf("expected success, got %d", code)
	}
	if !strings.Contains(out, "profile Minimal: certified") {
		t.Fatalf("matrix should show Minimal certified: %s", out)
	}
	if !strings.Contains(out, "profile StandardHA: blocked") {
		t.Fatalf("matrix should show StandardHA blocked: %s", out)
	}
	if !strings.Contains(out, "existing is certified for Minimal full platform delivery") {
		t.Fatalf("matrix should name the certified existing route: %s", out)
	}
	if !strings.Contains(out, "bootstrap and HA/offline remain blocked") {
		t.Fatalf("matrix must not promote bootstrap/HA: %s", out)
	}
}

func TestInitDrafts(t *testing.T) {
	for _, recipe := range []struct{ mode, os, user string }{
		{"existing", "", ""}, {"bootstrap", "ubuntu-24.04", "ubuntu"}, {"bootstrap", "ubuntu-26.04", "ubuntu"}, {"bootstrap", "rocky-10.1", "root"},
	} {
		t.Run(recipe.mode+recipe.os, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hnb.yaml")
			args := []string{"init", "--mode", recipe.mode, "-f", path}
			if recipe.os != "" {
				args = append(args, "--os", recipe.os)
			}
			code, _, errOut := runCLI(t, args...)
			if code != ExitSuccess {
				t.Fatalf("init: %s", errOut)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := toolkit.DecodeConfig(data)
			if err != nil {
				t.Fatal(err)
			}
			if recipe.user != "" && cfg.Spec.Cluster.Nodes[0].User != recipe.user {
				t.Fatal("wrong OS user")
			}
			if len(cfg.Validate(toolkit.CurrentEvidence)) == 0 {
				t.Fatal("unfilled draft must not validate")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("permissions: %v", info.Mode())
			}
		})
	}
}

func TestInitInteractiveMatchesNoninteractive(t *testing.T) {
	dir := t.TempDir()
	interactivePath := filepath.Join(dir, "interactive.yaml")
	directPath := filepath.Join(dir, "direct.yaml")
	var out, errOut bytes.Buffer
	input := strings.NewReader("bootstrap\n\nrocky-10.1\n" + interactivePath + "\n")
	code := ExecuteWithInput(context.Background(), []string{"init", "--interactive"}, input, &out, &errOut)
	if code != ExitSuccess {
		t.Fatalf("interactive init: %s", errOut.String())
	}
	code, _, msg := runCLI(t, "init", "--mode=bootstrap", "--os=rocky-10.1", "-f", directPath)
	if code != ExitSuccess {
		t.Fatal(msg)
	}
	a, _ := os.ReadFile(interactivePath)
	b, _ := os.ReadFile(directPath)
	if !bytes.Equal(a, b) {
		t.Fatal("interactive and noninteractive drafts differ")
	}
}

func TestInitRefusesOverwriteAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "customer.yaml")
	original := []byte("customer configuration")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, link} {
		code, _, _ := runCLI(t, "init", "--mode", "existing", "-f", path)
		if code != ExitUsage {
			t.Fatalf("overwrite accepted: %s", path)
		}
	}
	data, _ := os.ReadFile(target)
	if !bytes.Equal(data, original) {
		t.Fatal("customer configuration overwritten")
	}
}

func TestInitInvalidInputDoesNotCreateFile(t *testing.T) {
	for _, args := range [][]string{
		{"--mode", "existing", "--profile"},
		{"--mode", "existing", "--os", "rocky-10.1"},
		{"--mode", "bootstrap", "--os", "unknown"},
		{"--mode", "existing", "extra"},
		{"--interactive"},
	} {
		path := filepath.Join(t.TempDir(), "hnb.yaml")
		code, _, _ := runCLI(t, append([]string{"init", "-f", path}, args...)...)
		if code != ExitUsage {
			t.Fatalf("invalid input accepted: %v", args)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid input created a file")
		}
	}
}

func TestInitExistingWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hnb.yaml")
	code, out, _ := runCLI(t, "init", "--mode", "existing", "--profile", "Minimal", "-f", path)
	if code != ExitSuccess {
		t.Fatalf("init failed with %d: %s", code, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "mode: existing") || !strings.Contains(text, "publicBaseURL: \"\"") {
		t.Fatalf("template missing required fields: %s", text)
	}
	if strings.Contains(text, "latest") {
		t.Fatalf("template must not contain fake placeholders")
	}
}

func TestInitRejectsUncertifiedProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hnb.yaml")
	code, _, errOut := runCLI(t, "init", "--mode", "existing", "--profile", "StandardHA", "-f", path)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for unsupported profile, got %d", code)
	}
	if !strings.Contains(errOut, "not certified") {
		t.Fatalf("expected clear unsupported reason: %s", errOut)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("apiVersion: deployment.hnb.io/v1alpha1\nkind: DeploymentConfig\nspec:\n  mode: managed\n"), 0o600)
	code, _, errOut := runCLI(t, "validate", "-f", path)
	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d", code)
	}
	if !strings.Contains(errOut, "invalid") {
		t.Fatalf("expected invalid message: %s", errOut)
	}
}

func TestNodeAddRequiresTargetAndNodesFile(t *testing.T) {
	code, _, errOut := runCLI(t, "node", "add")
	if code != ExitUsage {
		t.Fatalf("expected usage exit for missing node add flags, got %d", code)
	}
	if !strings.Contains(errOut, "--target-id") {
		t.Fatalf("expected --target-id hint: %s", errOut)
	}
	code, _, errOut = runCLI(t, "node", "add", "--target-id", "target-abc")
	if code != ExitUsage {
		t.Fatalf("expected usage exit for missing nodes file, got %d", code)
	}
	if !strings.Contains(errOut, "-f nodes.yaml") {
		t.Fatalf("expected nodes file hint: %s", errOut)
	}
}

func TestInstallNeverReportsSuccessBeforeCertification(t *testing.T) {
	code, _, errOut := runCLI(t, "install", "-f", "x.yaml")
	if code == ExitSuccess {
		t.Fatal("install must not report success before its machinery ships")
	}
	if errOut == "" {
		t.Fatal("install must return an explicit failure reason")
	}
	// A missing config is refused before any planning or node action.
	if !strings.Contains(errOut, "DeploymentConfig") {
		t.Fatalf("expected config read failure: %s", errOut)
	}
	// Draft bootstrap config has empty required fields; validation refuses before
	// any node or cluster action, and never reports success.
	dir := t.TempDir()
	boot, _ := templateConfig("bootstrap", "Minimal")
	bootPath := filepath.Join(dir, "boot.yaml")
	os.WriteFile(bootPath, []byte(boot), 0o600)
	code, _, errOut = runCLI(t, "install", "-f", bootPath)
	if code == ExitSuccess {
		t.Fatal("install must never report success for an unconfigured bootstrap")
	}
}

func TestValidateFixtureConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.yaml")
	cfg, _ := templateConfig("bootstrap", "Minimal")
	os.WriteFile(path, []byte(cfg), 0o600)
	code, out, _ := runCLI(t, "validate", "-f", path)
	if code != ExitUsage {
		t.Fatalf("expected usage exit because publicBaseURL is empty, got %d (%s)", code, out)
	}
}

func TestValidateLoadsDefaultLockAndChecksDeclaredProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hnb.yaml")
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "kubeconfigFile: \"\"", "kubeconfigFile: ./fixture-kubeconfig", 1)
	draft = strings.Replace(draft, "publicBaseURL: \"\"", "publicBaseURL: https://fixture.invalid", 1)
	draft = strings.Replace(draft, "existingSecret: \"\"", "existingSecret: fixture-tls", 1)
	draft = strings.Replace(draft, "className: \"\"", "className: fixture-storage", 1)
	if err := os.WriteFile(filepath.Join(dir, "fixture-kubeconfig"), []byte("fixture-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, msg := runCLI(t, "validate", "-f", path)
	if code != ExitUsage || !strings.Contains(msg, "read lock") {
		t.Fatalf("missing lock accepted: %d %s", code, msg)
	}
	// Synthetic identities are decoding fixtures only, never release evidence.
	lock := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	if err := os.WriteFile(filepath.Join(dir, "release.lock.yaml"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, msg := runCLI(t, "validate", "-f", path)
	if code != ExitSuccess || !strings.Contains(out, "is structurally valid") {
		t.Fatalf("certified delivery rejected: %d %s %s", code, out, msg)
	}
	lock = strings.Replace(lock, "profiles: [Minimal]", "profiles: [Enterprise]", 1)
	if err := os.WriteFile(filepath.Join(dir, "release.lock.yaml"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, msg = runCLI(t, "validate", "-f", path)
	if code != ExitUsage || !strings.Contains(msg, "requested profile") {
		t.Fatalf("incompatible lock accepted: %d %s", code, msg)
	}
}

func TestPreflightPartialReportCannotAuthorizeInstallation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "hnb.yaml")
	outputPath := filepath.Join(dir, "report.json")
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "mode: online", "mode: offline", 1)
	draft = strings.Replace(draft, "bundlePath: \"\"", "bundlePath: ./fixture-bundle", 1)
	for from, to := range map[string]string{
		"kubeconfigFile: \"\"": "kubeconfigFile: ./fixture-kubeconfig",
		"publicBaseURL: \"\"":  "publicBaseURL: https://fixture.invalid",
		"existingSecret: \"\"": "existingSecret: fixture-tls",
		"className: \"\"":      "className: fixture-storage",
	} {
		draft = strings.Replace(draft, from, to, 1)
	}
	// Fixtures only: no registry, node or cluster is contacted by this existing-route test.
	lock := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	inputs := map[string]string{"hnb.yaml": draft, "release.lock.yaml": lock, "fixture-kubeconfig": "customer-secret-fixture"}
	for name, data := range inputs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := runCLI(t, "preflight", "-f", configPath, "--output", outputPath)
	if code != ExitBlocked || !strings.Contains(out, "PREFLIGHT_CHECK_SET_INCOMPLETE") {
		t.Fatalf("partial preflight passed: %d %s %s", code, out, errOut)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var report toolkit.PreflightReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "stdout-report.json")
	jsonCode, jsonOut, jsonErr := runCLI(t, "preflight", "-f", configPath, "--format", "json", "--output", jsonPath)
	if jsonCode != ExitBlocked || !strings.Contains(jsonErr, "PREFLIGHT_BLOCKED") {
		t.Fatalf("JSON outcome: %d %s", jsonCode, jsonErr)
	}
	var jsonReport toolkit.PreflightReport
	if err := json.Unmarshal([]byte(jsonOut), &jsonReport); err != nil {
		t.Fatal("stdout is not a single JSON report:", err)
	}
	jsonFile, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonFile) != jsonOut {
		t.Fatal("file and stdout JSON projections differ")
	}
	if err := jsonReport.Validate(); err != nil {
		t.Fatal(err)
	}
	if jsonReport.CheckSetComplete {
		t.Fatal("partial check set declared complete")
	}
	var failedOutputErr bytes.Buffer
	failedCode := Execute(context.Background(), []string{"preflight", "-f", configPath, "--format", "json"}, closedCLIWriter{}, &failedOutputErr)
	if failedCode != ExitError || !strings.Contains(failedOutputErr.String(), "OUTPUT_FAILED") {
		t.Fatalf("output error was concealed: %d %s", failedCode, failedOutputErr.String())
	}
	if report.RunID == "" || report.GeneratedAt.IsZero() || !strings.HasPrefix(report.ConfigDigest, "sha256:") || !strings.HasPrefix(report.LockDigest, "sha256:") || report.BlockingCount() == 0 {
		t.Fatalf("missing report identities: %+v", report)
	}
	if strings.Contains(string(data)+out+errOut, "customer-secret-fixture") {
		t.Fatal("credential leaked into report")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("report permissions are not private")
	}
	code, _, _ = runCLI(t, "preflight", "-f", configPath, "--output", outputPath)
	if code != ExitUsage {
		t.Fatal("existing report overwritten")
	}
	again, _ := os.ReadFile(outputPath)
	if !bytes.Equal(data, again) {
		t.Fatal("report changed after rejected overwrite")
	}
	for name, before := range inputs {
		after, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(after) != before {
			t.Fatal("preflight changed an input file")
		}
	}
}

type closedCLIWriter struct{}

func (closedCLIWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestPreflightRejectsBadFormatBeforeReadingOrWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	code, out, errOut := runCLI(t, "preflight", "-f", "nonexistent.yaml", "--format", "yaml", "--output", path)
	if code != ExitUsage || out != "" || !strings.Contains(errOut, "--format must be") {
		t.Fatalf("bad format was not rejected first: %d %s", code, errOut)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("bad format wrote a report")
	}
}

func TestLockGenerateUsageAndSideEffectFreeFailures(t *testing.T) {
	code, _, errOut := runCLI(t, "lock")
	if code != ExitUsage || !strings.Contains(errOut, "generate") {
		t.Fatalf("missing lock subcommand: %d %s", code, errOut)
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "release.lock.yaml")
	code, _, errOut = runCLI(t, "lock", "generate", "-f", "missing.yaml", "--output", output)
	if code != ExitUsage || !strings.Contains(errOut, "unavailable") {
		t.Fatalf("missing input not rejected: %d %s", code, errOut)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed generation wrote an output")
	}
	code, _, errOut = runCLI(t, "lock", "generate", "-f", filepath.Join(dir, "missing.yaml"), "--output", output, "extra")
	if code != ExitUsage || !strings.Contains(errOut, "positional") {
		t.Fatalf("positional argument accepted: %d %s", code, errOut)
	}
}

func TestLockGenerateRefusesExistingOutputBeforeResolution(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "release-input.yaml")
	if err := os.WriteFile(input, []byte("schemaVersion: release-input/v1\nreleaseId: candidate\nreleaseVersion: 0.2.0\nimages:\n- name: fixture\n  ref: registry.invalid/fixture:v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "release.lock.yaml")
	if err := os.WriteFile(output, []byte("customer data"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "lock", "generate", "-f", input, "--output", output)
	if code != ExitUsage || !strings.Contains(errOut, "already exists") {
		t.Fatalf("existing output not refused: %d %s", code, errOut)
	}
	data, _ := os.ReadFile(output)
	if string(data) != "customer data" {
		t.Fatal("existing customer file was overwritten")
	}
}
