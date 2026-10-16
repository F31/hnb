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

// writeRemovalFixture creates a run whose ConfigDigest matches the config file,
// so the run/config binding check passes.
func writeRemovalFixture(t *testing.T) (runDir, configPath string) {
	t.Helper()
	dir := t.TempDir()
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	for from, to := range map[string]string{
		"kubeconfigFile: \"\"": "kubeconfigFile: ./fixture-kubeconfig",
		"publicBaseURL: \"\"":  "publicBaseURL: https://fixture.invalid",
		"existingSecret: \"\"": "existingSecret: fixture-tls",
		"className: \"\"":      "className: fixture-storage",
	} {
		draft = strings.Replace(draft, from, to, 1)
	}
	configPath = filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(configPath, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, cfgErr := readConfig(configPath)
	if cfgErr != nil {
		t.Fatal(cfgErr)
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))

	store := toolkit.RunStore{Dir: filepath.Join(dir, "run")}
	if _, err := store.Create("run-removal", digest, "sha256:"+strings.Repeat("2", 64), "existing", []string{"namespace/hnb-system"}); err != nil {
		t.Fatal(err)
	}
	return store.Dir, configPath
}

// 卸载计划：保留外部 owner 与客户数据，删除仅限自有 release.
func planContains(list []string, want string) bool {
	for _, entry := range list {
		if strings.Contains(entry, want) {
			return true
		}
	}
	return false
}

func TestPlatformUninstallPlanPreservesExternalOwner(t *testing.T) {
	runDir, configPath := writeRemovalFixture(t)
	code, out, errOut := runCLI(t, "platform", "uninstall-hnb", "--run-dir", runDir, "-f", configPath,
		"--acknowledge-storage-removal", "--format", "json")
	// The plan is authorized but never executed locally.
	if code == ExitSuccess {
		t.Fatalf("removal reported as locally executed: %s %s", out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("invalid removal JSON: %v (%s)", err, out)
	}
	if !plan.Authorized || plan.Mode != toolkit.RemovalUninstallHNB {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	for _, forbidden := range []string{"external kubernetes", "external CNI", "business workloads", "secret/fixture-tls", "storageclass/fixture-storage"} {
		if planContains(plan.Delete, forbidden) {
			t.Fatalf("plan deletes preserved resource %q: %v", forbidden, plan.Delete)
		}
		if !planContains(plan.Retain, forbidden) {
			t.Fatalf("plan failed to retain %q: %v", forbidden, plan.Retain)
		}
	}
}

// 未确认存储删除时卸载被阻断.
func TestPlatformUninstallBlocksWithoutStorageAcknowledgement(t *testing.T) {
	runDir, configPath := writeRemovalFixture(t)
	code, out, errOut := runCLI(t, "platform", "uninstall-hnb", "--run-dir", runDir, "-f", configPath, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("uninstall authorized without acknowledgement: %s %s", out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !planContains(plan.Blocked, "PERSISTENT_STORAGE_REMOVAL_UNACKNOWLEDGED") {
		t.Fatalf("storage acknowledgement not enforced: %+v", plan)
	}
}

// 删除自建管理基座：未声明由本次安装创建时一律拒绝.
func TestPlatformDeleteBaseRefusesExternalCluster(t *testing.T) {
	runDir, configPath := writeRemovalFixture(t)
	code, out, errOut := runCLI(t, "platform", "delete-management-base", "--run-dir", runDir, "-f", configPath,
		"--acknowledge-storage-removal", "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("external cluster base deletion reported success: %s %s", out, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Authorized {
		t.Fatalf("delete-management-base authorized against an external cluster: %+v", plan)
	}
	if !planContains(plan.Blocked, "BASE_NOT_OWNED_BY_THIS_INSTALL") {
		t.Fatalf("ownership refusal missing: %v", plan.Blocked)
	}
}

// 解除纳管在本地不删除任何资源.
func TestPlatformUnmanageDeletesNothing(t *testing.T) {
	runDir, configPath := writeRemovalFixture(t)
	code, out, _ := runCLI(t, "platform", "unmanage", "--run-dir", runDir, "-f", configPath, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("unmanage reported as executed: %s", out)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Delete) != 0 {
		t.Fatalf("unmanage scheduled deletions: %v", plan.Delete)
	}
}

// 配置与 run 不匹配时拒绝出具计划.
func TestPlatformRemovalRefusesMismatchedRun(t *testing.T) {
	runDir, _ := writeRemovalFixture(t)
	other := filepath.Join(t.TempDir(), "other.yaml")
	// A structurally valid config that is simply not the recorded one.
	base, baseErr := templateConfig("existing", "Minimal")
	if baseErr != nil {
		t.Fatal(baseErr)
	}
	for from, to := range map[string]string{
		"kubeconfigFile: \"\"": "kubeconfigFile: ./fixture-kubeconfig",
		"publicBaseURL: \"\"":  "publicBaseURL: https://fixture.invalid",
		"existingSecret: \"\"": "existingSecret: fixture-tls",
		"className: \"\"":      "className: fixture-storage",
	} {
		base = strings.Replace(base, from, to, 1)
	}
	draft := strings.Replace(base, "namespace: hnb-system", "namespace: other-ns", 1)
	if err := os.WriteFile(other, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "platform", "uninstall-hnb", "--run-dir", runDir, "-f", other,
		"--acknowledge-storage-removal")
	if code == ExitSuccess {
		t.Fatal("mismatched configuration produced a removal plan")
	}
	if !strings.Contains(errOut, "does not match the recorded install run") {
		t.Fatalf("unexpected refusal: %s", errOut)
	}
}

func TestPlatformRequiresFlagsAndRejectsUnknownSubcommand(t *testing.T) {
	code, _, errOut := runCLI(t, "platform", "uninstall-hnb")
	if code == ExitSuccess || !strings.Contains(errOut, "--run-dir") {
		t.Fatalf("removal without --run-dir accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "platform", "destroy")
	if code == ExitSuccess || !strings.Contains(errOut, "unknown platform subcommand") {
		t.Fatalf("unknown platform subcommand accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "platform")
	if code == ExitSuccess || !strings.Contains(errOut, "platform subcommand required") {
		t.Fatalf("empty platform invocation accepted: %d %s", code, errOut)
	}
}
