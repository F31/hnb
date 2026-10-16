package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeToPlansProfileTransition(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	code, out, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("upgrade plan failed: %d %s %s", code, out, errOut)
	}
	var plan struct {
		SchemaVersion string `json:"schemaVersion"`
		From          string `json:"from"`
		To            string `json:"to"`
		Authorized    bool   `json:"authorized"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.SchemaVersion != "profile-upgrade-plan/v1" || plan.From != "Minimal" || plan.To != "LiteHA" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	// LiteHA is not certified in the current evidence: the plan must be blocked.
	if plan.Authorized {
		t.Fatalf("uncertified target must not be authorized: %s", out)
	}
}

func TestUpgradeToUnknownProfileRefused(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	if code, _, _ := runCLI(t, "upgrade", "-f", configPath, "--to", "Nope"); code != ExitUsage {
		t.Fatalf("unknown target accepted: %d", code)
	}
}

func TestUpgradeExecuteRefusedWhenBlocked(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	code, out, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--execute")
	if code == ExitSuccess {
		t.Fatalf("blocked upgrade executed: %s %s", out, errOut)
	}
	if !strings.Contains(errOut, "upgrade execution refused") {
		t.Fatalf("missing refusal reason: %s", errOut)
	}
}

func TestScalePlanReplicas(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	code, out, errOut := runCLI(t, "scale", "-f", configPath, "--component", "apiserver", "--replicas", "3", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("scale plan failed: %d %s %s", code, out, errOut)
	}
	var plan struct {
		Authorized bool `json:"authorized"`
		Steps      []struct {
			Target string `json:"target"`
			From   string `json:"from"`
			To     string `json:"to"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].Target != "apiserver" || plan.Steps[0].To != "3" {
		t.Fatalf("unexpected scale plan: %+v", plan)
	}
}

func TestScaleRefusesStorageShrink(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	code, out, _ := runCLI(t, "scale", "-f", configPath, "--storage", "postgresql=1", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("scale plan should compute even when blocked: %d", code)
	}
	if !strings.Contains(out, "STORAGE_SHRINK_REFUSED") {
		t.Fatalf("storage shrink not refused: %s", out)
	}
}

// TestUpgradeExecuteRunsOnCertifiedProfile drives the gated upgrade executor
// through the CLI for a Minimal->Minimal transition (the only currently
// certified profile) with fixture Helm/kubectl, proving the run is created as
// an upgrade and finalised succeeded.
func TestUpgradeExecuteRunsOnCertifiedProfile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, bin, "helm", "#!/bin/sh\ncat <<'EOD'\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hnb-upgrade-fixture\ndata:\n  ok: \"true\"\nEOD\n")
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
	lockBody := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: 0.1.0\n" +
		"images:\n  - name: portal\n    ref: registry.invalid/portal:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\n    manifestDigest: sha256:" + strings.Repeat("c", 64) + "\n    platform: linux/amd64\n" +
		"charts:\n  - name: hnb\n    version: 0.1.0\n    digest: " + chartDigest + "\n" +
		"supportedUpgrades:\n  - {from: Minimal, to: Minimal}\n" +
		"compatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	if err := os.WriteFile(lock, []byte(lockBody), 0o600); err != nil {
		t.Fatal(err)
	}
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "releaseLock: ./release.lock.yaml", "releaseLock: "+lock, 1)
	draft = strings.Replace(draft, "kubeconfigFile: \"\"", "kubeconfigFile: "+kubeconfig, 1)
	draft = strings.Replace(draft, "publicBaseURL: \"\"", "publicBaseURL: https://fixture.invalid", 1)
	draft = strings.Replace(draft, "existingSecret: \"\"", "existingSecret: fixture-tls", 1)
	draft = strings.Replace(draft, "className: \"\"", "className: fixture-storage", 1)
	configPath := filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(configPath, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	bindings := filepath.Join(dir, "bindings.yaml")
	if err := os.WriteFile(bindings, []byte("portal:\n  db:\n    existingSecret: hnb-platform-pg\n    secretKey: dsn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(dir, "runs")
	code, out, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "Minimal", "--execute",
		"--chart-package", chart, "--bindings", bindings, "--workload", "Deployment/apiserver",
		"--runs-dir", runs, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("upgrade execute failed: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "status=succeeded") {
		t.Fatalf("upgrade did not report success: %s", out)
	}
}

// TestScaleExecuteAppliesWithFixtureCluster drives scale --execute through the
// CLI with a fixture kubectl, proving the run is created as a scale run and
// finalised succeeded.
func TestScaleExecuteAppliesWithFixtureCluster(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, bin, "kubectl", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	kubeconfig := filepath.Join(dir, "fixture-kubeconfig")
	if err := os.WriteFile(kubeconfig, []byte("fixture-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "kubeconfigFile: \"\"", "kubeconfigFile: "+kubeconfig, 1)
	draft = strings.Replace(draft, "publicBaseURL: \"\"", "publicBaseURL: https://fixture.invalid", 1)
	draft = strings.Replace(draft, "existingSecret: \"\"", "existingSecret: fixture-tls", 1)
	draft = strings.Replace(draft, "className: \"\"", "className: fixture-storage", 1)
	configPath := filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(configPath, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"
	if err := os.WriteFile(filepath.Join(dir, "release.lock.yaml"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(dir, "runs")
	code, out, errOut := runCLI(t, "scale", "-f", configPath, "--component", "apiserver", "--replicas", "3", "--execute", "--runs-dir", runs)
	if code != ExitSuccess {
		t.Fatalf("scale execute failed: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "status=succeeded") {
		t.Fatalf("scale did not report success: %s", out)
	}
}
