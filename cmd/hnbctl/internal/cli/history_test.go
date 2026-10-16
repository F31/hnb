package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

const historyFixtureLock = "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal]\n"

// writeExistingFixture materializes a valid existing-cluster DeploymentConfig.
func writeExistingFixture(t *testing.T) (dir, configPath string) {
	t.Helper()
	dir = t.TempDir()
	configPath = filepath.Join(dir, "hnb.yaml")
	draft, err := templateConfig("existing", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	draft = strings.Replace(draft, "kubeconfigFile: \"\"", "kubeconfigFile: ./fixture-kubeconfig", 1)
	draft = strings.Replace(draft, "publicBaseURL: \"\"", "publicBaseURL: https://fixture.invalid", 1)
	draft = strings.Replace(draft, "existingSecret: \"\"", "existingSecret: fixture-tls", 1)
	draft = strings.Replace(draft, "className: \"\"", "className: fixture-storage", 1)
	for name, data := range map[string]string{
		"hnb.yaml": draft, "release.lock.yaml": historyFixtureLock, "fixture-kubeconfig": "fixture-only",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, configPath
}

// 安装即入历史：install 落盘运行记录，随后 history 可读且状态真实（规划被阻塞，不是成功）.
func TestInstallRecordsRunThenHistoryListsIt(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	t.Setenv("XDG_STATE_HOME", filepath.Dir(runsDir))

	code, _, errOut := runCLI(t, "install", "-f", configPath, "--runs-dir", runsDir)
	if code != ExitNotVerified {
		t.Fatalf("install must refuse authorization: %d %s", code, errOut)
	}

	code, out, errOut := runCLI(t, "history", "--runs-dir", runsDir, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("history failed: %d %s", code, errOut)
	}
	var doc struct {
		SchemaVersion string               `json:"schemaVersion"`
		Total         int                  `json:"total"`
		Runs          []toolkit.RunSummary `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != "install-history/v1" || doc.Total != 1 || len(doc.Runs) != 1 {
		t.Fatalf("unexpected history document: %+v", doc)
	}
	// Planning-only install must never be recorded as a completed installation.
	if doc.Runs[0].Status == toolkit.PhaseSucceededState {
		t.Fatalf("planning-only install recorded as succeeded: %+v", doc.Runs[0])
	}
	if doc.Runs[0].LockDigest == "" || doc.Runs[0].ConfigDigest == "" {
		t.Fatalf("history record lost the run identity: %+v", doc.Runs[0])
	}
	if !strings.Contains(doc.Runs[0].RoutingMode, "external-cluster") {
		t.Fatalf("history record lost the routing mode: %+v", doc.Runs[0])
	}
}

// 历史日志：logs 输出真实状态转换，且 JSON 可解析、序号连续.
func TestLogsReturnRecordedTransitions(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	if code, _, errOut := runCLI(t, "install", "-f", configPath, "--runs-dir", runsDir); code != ExitNotVerified {
		t.Fatalf("install must refuse authorization: %d %s", code, errOut)
	}

	code, out, errOut := runCLI(t, "logs", "--runs-dir", runsDir, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("logs failed: %d %s", code, errOut)
	}
	var doc struct {
		SchemaVersion string             `json:"schemaVersion"`
		Events        []toolkit.RunEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != "install-logs/v1" || len(doc.Events) < 3 {
		t.Fatalf("unexpected log document: %+v", doc)
	}
	var planningSeen bool
	for i, event := range doc.Events {
		if event.Sequence != int64(i+1) {
			t.Fatalf("log sequence gap at %d: %+v", i, doc.Events)
		}
		if event.Kind == "phase.changed" && event.Phase == toolkit.PhasePlanning && event.State == toolkit.PhaseSucceededState {
			planningSeen = true
		}
	}
	if !planningSeen {
		t.Fatalf("log does not record the planning stage: %+v", doc.Events)
	}
	// Human rendering must not leak credentials or node-init material.
	code, human, errOut := runCLI(t, "logs", "--runs-dir", runsDir)
	if code != ExitSuccess {
		t.Fatalf("human logs failed: %d %s", code, errOut)
	}
	for _, forbidden := range []string{"password=", "BEGIN EC PRIVATE KEY", "kubeadm"} {
		if strings.Contains(human, forbidden) {
			t.Fatalf("log leaked %q: %s", forbidden, human)
		}
	}
}

// 进度呈现：status --watch 输出阶段进度并以真实退出码结束.
func TestStatusWatchShowsProgress(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	if code, _, errOut := runCLI(t, "install", "-f", configPath, "--runs-dir", runsDir); code != ExitNotVerified {
		t.Fatalf("install must refuse authorization: %d %s", code, errOut)
	}
	_, out, errOut := runCLI(t, "history", "--runs-dir", runsDir, "--format", "json")
	if errOut != "" {
		t.Fatal(errOut)
	}
	var doc struct {
		Runs []toolkit.RunSummary `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}

	code, statusOut, _ := runCLI(t, "status", "--run-dir", doc.Runs[0].Directory, "--watch", "--interval", "100ms")
	if code == ExitSuccess {
		t.Fatalf("planning-only run must not verify: %d %s", code, statusOut)
	}
	if !strings.Contains(statusOut, "completed=") || !strings.Contains(statusOut, "planning") {
		t.Fatalf("status watch did not render progress: %s", statusOut)
	}
	// The watch must not print credential material.
	for _, forbidden := range []string{"password=", "BEGIN EC PRIVATE KEY"} {
		if strings.Contains(statusOut, forbidden) {
			t.Fatalf("status leaked %q: %s", forbidden, statusOut)
		}
	}
}

// 卸载绑定安装 run：不匹配的 run 拒绝，匹配的 run 产出删除自有资源的计划.
func TestUninstallBindsRecordedRun(t *testing.T) {
	_, configPath := writeExistingFixture(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	if code, _, errOut := runCLI(t, "install", "-f", configPath, "--runs-dir", runsDir); code != ExitNotVerified {
		t.Fatalf("install must refuse authorization: %d %s", code, errOut)
	}
	_, out, _ := runCLI(t, "history", "--runs-dir", runsDir, "--format", "json")
	var doc struct {
		Runs []toolkit.RunSummary `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}

	// Default uninstall now retains owned storage and its namespace.
	code, defaultOut, errOut := runCLI(t, "uninstall", "--runs-dir", runsDir, "-f", configPath, "--format", "json")
	if code != ExitNotVerified {
		t.Fatalf("retained-data plan must defer execution: %d %s", code, errOut)
	}
	var defaultPlan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(defaultOut), &defaultPlan); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(defaultPlan.Delete, "\n"), "persistentvolumeclaim") {
		t.Fatal("default uninstall deletes persistent data")
	}

	code, planOut, errOut := runCLI(t, "uninstall", "--run-dir", doc.Runs[0].Directory, "-f", configPath,
		"--acknowledge-storage-removal", "--format", "json")
	if code != ExitNotVerified {
		t.Fatalf("authorized removal plan must still defer execution: %d %s", code, errOut)
	}
	var plan toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(planOut), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || plan.RunID != doc.Runs[0].RunID {
		t.Fatalf("removal plan lost the recorded run: %+v", plan)
	}
	joined := strings.Join(plan.Delete, "\n")
	for _, forbidden := range []string{"external kubernetes", "external CNI", "external postgresql"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("external resource scheduled for deletion: %s", forbidden)
		}
	}
	if len(plan.Delete) == 0 {
		t.Fatalf("removal plan deletes nothing: %+v", plan)
	}

	// A config that is not the recorded one must be refused.
	other := strings.NewReplacer("fixture-tls", "other-tls").Replace(readFile(t, configPath))
	otherPath := filepath.Join(t.TempDir(), "other.yaml")
	if err := os.WriteFile(otherPath, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCLI(t, "uninstall", "--run-dir", doc.Runs[0].Directory, "-f", otherPath); code != ExitUsage {
		t.Fatalf("mismatched config must be refused: %d %s", code, errOut)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
