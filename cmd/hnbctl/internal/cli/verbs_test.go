package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 极简动词与完整命令行为一致：create config 生成草稿.
func TestVerbCreateConfigMatchesInit(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "hnb.yaml")
	var initOut, verbOut bytes.Buffer
	initCode := ExecuteWithInput(context.Background(), []string{"init", "--mode", "existing", "-f", configPath}, nil, &initOut, io.Discard)
	if initCode != ExitSuccess {
		t.Fatalf("init failed: %d %s", initCode, initOut.String())
	}
	verbDir := t.TempDir()
	verbPath := filepath.Join(verbDir, "verb.yaml")
	verbCode := ExecuteWithInput(context.Background(), []string{"create", "config", "--mode", "existing", "-f", verbPath}, nil, &verbOut, io.Discard)
	if verbCode != ExitSuccess {
		t.Fatalf("create config failed: %d %s", verbCode, verbOut.String())
	}
	initData, _ := os.ReadFile(configPath)
	verbData, _ := os.ReadFile(verbPath)
	if !strings.Contains(string(initData), "mode: existing") || !strings.Contains(string(verbData), "mode: existing") {
		t.Fatalf("draft mismatch: init=%q verb=%q", string(initData), string(verbData))
	}
}

// 极简动词缺参数时与原命令一致拒绝.
func TestVerbAddNodesRequiresTargetID(t *testing.T) {
	code, out, errOut := runCLI(t, "add", "nodes", "--platform-url", "https://api.invalid", "--token-file", writeTokenFile(t))
	if code == ExitSuccess || !strings.Contains(errOut, "--target-id") {
		t.Fatalf("add nodes without --target-id: %d %s %v", code, out, errOut)
	}
}

// delete installation 转发到 uninstall：无记录时非成功.
func TestVerbDeleteInstallationDelegatesToUninstall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runs")
	code, out, errOut := runCLI(t, "delete", "installation", "--installation", "run-absent", "--runs-dir", root)
	if code == ExitSuccess {
		t.Fatalf("delete installation without a record reported success: %s %s", out, errOut)
	}
	if !strings.Contains(errOut, "no matching install run") {
		t.Fatalf("expected a missing-record refusal, got %q", errOut)
	}
}

// upgrade 转发到 platform upgrade：必须给出两个锁.
func TestVerbUpgradeRequiresBothLocks(t *testing.T) {
	code, out, errOut := runCLI(t, "upgrade", "--current-lock", "a")
	if code == ExitSuccess || !strings.Contains(errOut, "--current-lock and --release-lock are required") {
		t.Fatalf("upgrade without both locks: %d %s %v", code, out, errOut)
	}
}

// artifact 转发到 bundle：缺 lock/cache/manifest 时拒绝.
func TestVerbArtifactRequiresLockCacheManifest(t *testing.T) {
	for _, args := range [][]string{
		{"artifact", "prepare", "--lock", "l.yaml", "--cache-dir", "cache"},
		{"artifact", "verify", "--lock", "l.yaml", "--cache-dir", "cache"},
	} {
		code, out, errOut := runCLI(t, args...)
		if code == ExitSuccess || !strings.Contains(errOut, "--manifest") {
			t.Fatalf("%v: expected --manifest refusal, got %d %s %v", args, code, out, errOut)
		}
	}
	code, _, errOut := runCLI(t, "artifact", "bogus")
	if code == ExitSuccess || !strings.Contains(errOut, "bundle subcommand") {
		t.Fatalf("artifact bogus not refused: %s", errOut)
	}
}

// 补全脚本包含命令名并使用固定输出，不注入任意 shell.
func TestVerbCompletionProducesSafeScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		code, out, errOut := runCLI(t, "completion", shell)
		if code != ExitSuccess {
			t.Fatalf("completion %s failed: %d %s", shell, code, errOut)
		}
		for _, want := range []string{"install", "uninstall", "cluster"} {
			if !strings.Contains(out, want) {
				t.Fatalf("completion %s missing %q", shell, want)
			}
		}
		for _, forbidden := range []string{"$(rm", "`", "; rm"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("completion %s contains injection %q", shell, forbidden)
			}
		}
	}
	if code, _, errOut := runCLI(t, "completion", "tcsh"); code == ExitSuccess || !strings.Contains(errOut, "bash|zsh|fish") {
		t.Fatalf("unknown shell not refused: %s", errOut)
	}
}

// registry 中存在同名命令（帮助/补全依赖唯一性）.
func TestCommandNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands {
		if seen[c.Name] {
			t.Fatalf("duplicate command name %q", c.Name)
		}
		seen[c.Name] = true
	}
}
