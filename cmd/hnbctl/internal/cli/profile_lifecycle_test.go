package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
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

func TestUpgradeControlDomainPrecondition(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	if code, _, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--backing", "in-cluster-ha", "--control-node-count", "1"); code != ExitUsage || !strings.Contains(errOut, "HA_CONTROL_NODES_INSUFFICIENT") {
		t.Fatalf("insufficient control domains not refused: %d %s", code, errOut)
	}
	code, out, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--control-node-count", "2", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("plan should compute: %d %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "HA_CONTROL_NODES_INSUFFICIENT") {
		t.Fatalf("control-domain precondition missing from plan: %s", out)
	}
}

func TestUpgradeBackingOverrideAndUnknown(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	if code, _, _ := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--backing", "on-prem"); code != ExitUsage {
		t.Fatalf("unknown backing accepted: %d", code)
	}
	code, out, errOut := runCLI(t, "upgrade", "-f", configPath, "--to", "LiteHA", "--backing", "external", "--control-node-count", "3", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("external backing plan failed: %d %s %s", code, out, errOut)
	}
	if strings.Contains(out, "HA_CONTROL_NODES_INSUFFICIENT") || strings.Contains(out, "OPERATOR_NOT_INSTALLED") {
		t.Fatalf("external backing must not require in-cluster preconditions: %s", out)
	}
}

func TestScaleDataPlaneComponentMapping(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	code, out, _ := runCLI(t, "scale", "-f", configPath, "--component", "nats", "--replicas", "3", "--format", "json")
	if code != ExitSuccess || !strings.Contains(out, "SCALE_UNKNOWN_COMPONENT") {
		t.Fatalf("bundled data-plane scale not refused: %d %s", code, out)
	}
	code, out, _ = runCLI(t, "scale", "--profile", "LiteHA", "--component", "nats", "--replicas", "5", "--control-node-count", "2", "--format", "json")
	if code != ExitSuccess || !strings.Contains(out, "HA_CONTROL_NODES_INSUFFICIENT") {
		t.Fatalf("control-domain precondition missing from scale: %d %s", code, out)
	}
	code, out, _ = runCLI(t, "scale", "--profile", "LiteHA", "--component", "postgresql", "--replicas", "5", "--control-node-count", "3", "--format", "json")
	if code != ExitSuccess || !strings.Contains(out, "Cluster.spec.instances") {
		t.Fatalf("postgresql data-plane mapping missing: %d %s", code, out)
	}
}

func TestScaleStepArgsMapDataPlane(t *testing.T) {
	args, err := scaleStepArgs(toolkit.UpgradeStep{Kind: "components", Target: "postgresql", To: "5"})
	if err != nil || strings.Join(args, " ") != `patch cluster.postgresql.cnpg.io/postgresql --type merge -p {"spec":{"instances":5}}` {
		t.Fatalf("postgresql scale args wrong: %v %v", args, err)
	}
	args, _ = scaleStepArgs(toolkit.UpgradeStep{Kind: "components", Target: "nats", To: "5"})
	if strings.Join(args, " ") != "scale statefulset/nats --replicas 5" {
		t.Fatalf("nats scale args wrong: %v", args)
	}
	args, _ = scaleStepArgs(toolkit.UpgradeStep{Kind: "components", Target: "apiserver", To: "3"})
	if strings.Join(args, " ") != "scale deployment/apiserver --replicas 3" {
		t.Fatalf("deployment scale args wrong: %v", args)
	}
	args, _ = scaleStepArgs(toolkit.UpgradeStep{Kind: "storage", Target: "postgresql", To: "10Gi"})
	if !strings.Contains(strings.Join(args, " "), `"storage":{"size":"10Gi"}`) {
		t.Fatalf("postgresql storage args wrong: %v", args)
	}
	args, _ = scaleStepArgs(toolkit.UpgradeStep{Kind: "storage", Target: "nats", To: "10Gi"})
	if !strings.Contains(strings.Join(args, " "), "pvc nats-data") {
		t.Fatalf("nats storage args wrong: %v", args)
	}
	if _, err := scaleStepArgs(toolkit.UpgradeStep{Kind: "resources", Target: "portal", To: "bad"}); err == nil {
		t.Fatal("malformed resources accepted")
	}
	if got := scaleVerifyArgs("nats", "5m"); strings.Join(got, " ") != "rollout status statefulset/nats --timeout 5m" {
		t.Fatalf("nats verify args wrong: %v", got)
	}
}
