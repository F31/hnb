package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

type shortcutPlan struct {
	ID                  string `json:"installationId"`
	Dir                 string `json:"runDir"`
	ExecutionAuthorized bool   `json:"executionAuthorized"`
}

func quickExistingArgs(t *testing.T) ([]string, string) {
	t.Helper()
	dir, _ := writeExistingFixture(t)
	root := filepath.Join(t.TempDir(), "runs")
	return []string{"install", "--kubeconfig", filepath.Join(dir, "fixture-kubeconfig"),
		"--domain", "fixture.invalid", "--tls-secret", "fixture-tls", "--storage-class", "fixture-storage",
		"--lock", filepath.Join(dir, "release.lock.yaml"), "--runs-dir", root, "--format", "json"}, root
}

func planShortcut(t *testing.T, args []string) shortcutPlan {
	t.Helper()
	code, out, stderr := runCLI(t, append(args, "--plan")...)
	if code != ExitSuccess {
		t.Fatalf("shortcut plan: %d %s %s", code, out, stderr)
	}
	var plan shortcutPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.ID == "" || plan.ExecutionAuthorized {
		t.Fatalf("plan fabricated an installation: %+v", plan)
	}
	return plan
}

func TestShortcutExistingInstallSavesIdentityAndRestoresUninstallWithoutConfig(t *testing.T) {
	args, root := quickExistingArgs(t)
	plan := planShortcut(t, args)
	cfg, err := toolkit.LoadInstallSnapshot(plan.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Spec.Cluster.KubeconfigFile == "" || cfg.Spec.Platform.Namespace != "hnb-system" || cfg.Metadata.Name != "hnb" {
		t.Fatalf("incorrect shortcut configuration: %+v", cfg)
	}
	for _, name := range []string{"deployment.yaml", "release.lock.yaml"} {
		fi, err := os.Stat(filepath.Join(plan.Dir, name))
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatalf("snapshot not private: %s %v", name, err)
		}
	}
	code, out, stderr := runCLI(t, "uninstall", "--installation", plan.ID, "--runs-dir", root, "--plan", "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("uninstall snapshot restore: %d %s", code, stderr)
	}
	var removal toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &removal); err != nil {
		t.Fatal(err)
	}
	if removal.RunID != plan.ID || removal.ExecutionAuthorized || !removal.Authorized {
		t.Fatalf("invalid removal: %+v", removal)
	}
	deleted := strings.Join(removal.Delete, "\n")
	if strings.Contains(deleted, "persistentvolumeclaim") || strings.Contains(deleted, "namespace/") {
		t.Fatalf("default uninstall deletes data/namespace: %s", deleted)
	}
	if !strings.Contains(strings.Join(removal.RetainedData, "\n"), "persistentvolumeclaim") {
		t.Fatal("retained owned data absent from plan")
	}
	// Source config/lock files are not needed to restore the bound snapshots.
	if err := os.Remove(cfg.Spec.ReleaseLock); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "uninstall", "--installation", plan.ID, "--runs-dir", root, "--plan"); code != ExitSuccess {
		t.Fatalf("uninstall depended on deleted source files: %d %s", code, stderr)
	}
}

func TestShortcutHostPlanNeverReadsPasswordOrClaimsNodeOwnership(t *testing.T) {
	args, root := quickExistingArgs(t)
	for i, arg := range args {
		if arg == "--kubeconfig" {
			args[i], args[i+1] = "--host", "192.0.2.10"
		}
	}
	for i, arg := range args {
		if arg == "--lock" {
			data, err := os.ReadFile(args[i+1])
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.Replace(string(data), "  profiles:", "  kubernetesVersions: [1.36.5]\n  profiles:", 1))
			if err := os.WriteFile(args[i+1], data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	args = append(args, "--internal-address", "10.0.0.10", "--user", "ubuntu", "--ask-pass")
	plan := planShortcut(t, args) // No stdin: planning must not ask for a password.
	cfg, err := toolkit.LoadInstallSnapshot(plan.Dir)
	if err != nil {
		t.Fatal(err)
	}
	node := cfg.Spec.Cluster.Nodes[0]
	if node.SSHAuth != "password" || node.InternalAddress != "10.0.0.10" || cfg.Spec.Cluster.APIEndpoint != "10.0.0.10:6443" {
		t.Fatalf("incorrect host plan: %+v", node)
	}
	run, err := toolkit.LoadRunState(plan.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != toolkit.PhaseBlockedState || len(run.Ownership) != 0 {
		t.Fatalf("planning claimed an installed node: %+v", run)
	}
	if code, _, stderr := runCLI(t, "uninstall", "--installation", plan.ID, "--runs-dir", root); code == ExitSuccess || !strings.Contains(stderr, "not ownership evidence") {
		t.Fatalf("host plan authorized removal: %d %s", code, stderr)
	}
}

func TestShortcutPreflightPersistsFailureAndYesDoesNotBypassGates(t *testing.T) {
	args, root := quickExistingArgs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // No external requests in this fixture.
	var out, stderr bytes.Buffer
	code := ExecuteWithInput(ctx, append(args, "--yes"), nil, &out, &stderr)
	if code == ExitSuccess {
		t.Fatal("--yes bypassed a failed preflight/execution gate")
	}
	runs, err := toolkit.ListRunHistory(root)
	if err != nil || len(runs) != 1 {
		t.Fatalf("missing run: %+v %v", runs, err)
	}
	run, err := toolkit.LoadRunState(runs[0].Directory)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != toolkit.PhaseBlockedState || run.Phases[string(toolkit.PhasePreflight)] == nil {
		t.Fatalf("preflight outcome not persisted: %+v", run)
	}
	if _, err := os.Stat(filepath.Join(runs[0].Directory, "preflight.json")); err != nil {
		t.Fatal("preflight report not saved")
	}
}

func TestShortcutUninstallDataSelectionAndConfirmation(t *testing.T) {
	args, root := quickExistingArgs(t)
	plan := planShortcut(t, args)
	base := []string{"uninstall", "--installation", plan.ID, "--runs-dir", root, "--delete-data"}
	if code, _, stderr := runCLI(t, base...); code != ExitBlocked || !strings.Contains(stderr, "not confirmed") {
		t.Fatalf("destructive plan accepted without confirmation: %d %s", code, stderr)
	}
	code, out, stderr := runCLI(t, append(base, "--yes", "--format", "json")...)
	if code != ExitNotVerified {
		t.Fatalf("confirmed plan must still defer execution: %d %s", code, stderr)
	}
	var removal toolkit.RemovalPlan
	if err := json.Unmarshal([]byte(out), &removal); err != nil {
		t.Fatal(err)
	}
	if !removal.DataLossAcknowledged || !strings.Contains(strings.Join(removal.Delete, "\n"), "persistentvolumeclaim") || removal.ExecutionAuthorized {
		t.Fatalf("incorrect destructive plan: %+v", removal)
	}
	if !strings.Contains(strings.Join(removal.Delete, "\n"), "namespace/") {
		t.Fatal("data deletion did not schedule namespace removal")
	}
	if strings.Contains(strings.Join(removal.Retain, "\n"), "namespace/") {
		t.Fatal("data deletion still retained the namespace")
	}
	if strings.Contains(strings.Join(removal.Retain, "\n"), "secret/") {
		t.Fatal("data deletion still retained operator-owned secrets")
	}
}

func TestShortcutRejectsAmbiguousOrIncompleteOptionsBeforeCreatingHistory(t *testing.T) {
	for _, flags := range [][]string{
		{"--host", "192.0.2.10", "--kubeconfig", "cluster"},
		{"-f", "config", "--host", "192.0.2.10"},
		{"--host", "192.0.2.10", "--ssh-key", "key", "--ask-pass"},
		{"--kubeconfig", "cluster", "--ask-pass"},
		{"--host", "192.0.2.10", "--ask-pass"},
		{"--host", "192.0.2.10", "--domain", "https://bad.invalid", "--tls-secret", "tls", "--storage-class", "storage", "--ask-pass"},
	} {
		root := filepath.Join(t.TempDir(), "not-created")
		argv := append([]string{"install", "--runs-dir", root}, flags...)
		if code, _, _ := runCLI(t, argv...); code != ExitUsage {
			t.Fatalf("bad flags accepted: %v", flags)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid shortcut created history")
		}
	}
}

func TestShortcutUninstallRefusesMissingOrChangedSnapshots(t *testing.T) {
	for _, kind := range []string{"config-change", "lock-change", "public-file", "symlink", "missing"} {
		t.Run(kind, func(t *testing.T) {
			args, root := quickExistingArgs(t)
			plan := planShortcut(t, args)
			path := filepath.Join(plan.Dir, "deployment.yaml")
			switch kind {
			case "config-change":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.Replace(string(data), "hnb-system", "foreign-system", 1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "lock-change":
				path = filepath.Join(plan.Dir, "release.lock.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.Replace(string(data), "releaseId: fixture", "releaseId: other", 1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "public-file":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				other := filepath.Join(t.TempDir(), "foreign.yaml")
				if err := os.WriteFile(other, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			code, out, stderr := runCLI(t, "uninstall", "--installation", plan.ID, "--runs-dir", root, "--plan", "--yes", "--format", "json")
			if code != ExitUsage || out != "" || !strings.Contains(stderr, "snapshot") {
				t.Fatalf("unbound snapshot accepted: %d %s %s", code, out, stderr)
			}
		})
	}
}

// --execute 走显式执行入口；existing route 证据已认证，但缺少 applier 输入时仍拒绝且不记录成功.
func TestShortcutExecuteRefusesWithStableReasons(t *testing.T) {
	args, root := quickExistingArgs(t)
	code, out, errOut := runCLI(t, append(args, "--execute")...)
	if code != ExitUsage {
		t.Fatalf("execute without applier inputs must be refused: %d %s", code, errOut)
	}
	if !strings.Contains(errOut, "chart-package") {
		t.Fatalf("execute refusal must name the missing applier inputs: %s", errOut)
	}
	if strings.Contains(out+errOut, "executionAuthorized\": true") || strings.Contains(out+errOut, "executionAuthorized=true") {
		t.Fatalf("execute claimed authorization: %s %s", out, errOut)
	}
	runs, err := toolkit.ListRunHistory(root)
	if err != nil || len(runs) != 1 || runs[0].Status == toolkit.PhaseSucceededState {
		t.Fatalf("execute recorded a success: %+v %v", runs, err)
	}
	// --plan 与 --execute 互斥.
	if c, _, _ := runCLI(t, append(args, "--plan", "--execute")...); c != ExitUsage {
		t.Fatalf("--plan with --execute accepted: %d", c)
	}
}

// bootstrap 路线的 --execute 在门禁未认证时拒绝，且不创建任何 run 或接触节点.
func TestBootstrapExecuteRefusesWithoutEvidence(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "release.lock.yaml")
	lockBody := "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: test\nimages:\n  - name: fixture\n    ref: registry.invalid/fixture:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\ncompatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  kubernetesVersions: [1.36.5]\n  profiles: [Minimal]\n"
	if err := os.WriteFile(lock, []byte(lockBody), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "runs")
	args := []string{"install", "--host", "192.0.2.10", "--internal-address", "10.0.0.10",
		"--user", "ubuntu", "--ask-pass", "--domain", "fixture.invalid", "--tls-secret", "fixture-tls",
		"--storage-class", "fixture-storage", "--lock", lock, "--runs-dir", root, "--format", "json", "--execute"}
	code, out, errOut := runCLI(t, args...)
	if code != ExitNotVerified {
		t.Fatalf("bootstrap execute must stay refused: %d %s", code, errOut)
	}
	if !strings.Contains(errOut, "bootstrap execution refused") || !strings.Contains(errOut, "no full platform delivery evidence") {
		t.Fatalf("bootstrap refusal missing evidence reason: %s", errOut)
	}
	if strings.Contains(out+errOut, "kubeadm") || strings.Contains(out+errOut, "ssh ") {
		t.Fatalf("bootstrap refusal leaked node actions: %s %s", out, errOut)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("bootstrap execute created a run while gated")
	}
}
