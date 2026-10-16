package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file fixes the remaining 1.2 rejection scenarios at the CLI surface:
// managed/unsupported actions must return explicit non-success and must never
// emit a fabricated Operation, an install instruction, or a success message.

func writeBootstrapDraft(t *testing.T, dir string) string {
	t.Helper()
	draft, err := templateConfig("bootstrap", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(path, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Scenario: managed 模式 API 503/超时 -> 保留已知引用, 不触发本地 SSH/bootstrap.
// Today managed is refused during validation. The CLI contract is non-success
// with the managed-specific reason, no fabricated operationId, no imperative
// local-execution instruction, and no run artifacts on disk.
func TestManagedRequestReturnsConnectionLevelRefusalNotLocalFallback(t *testing.T) {
	dir := t.TempDir()
	bootstrap, err := templateConfig("bootstrap", "Minimal")
	if err != nil {
		t.Fatal(err)
	}
	managed := strings.Replace(bootstrap, "mode: bootstrap", "mode: managed", 1)
	path := filepath.Join(dir, "managed.yaml")
	if err := os.WriteFile(path, []byte(managed), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(dir)
	code, out, errOut := runCLI(t, "install", "-f", path)
	if code == ExitSuccess {
		t.Fatalf("managed install must not succeed: %s %s", out, errOut)
	}
	if !strings.Contains(errOut, "mode=managed") {
		t.Fatalf("expected the managed-contract refusal, got %s", errOut)
	}
	combined := out + errOut
	// Imperative local-execution instructions would mean a real fallback path.
	for _, forbidden := range []string{"operationId", "ssh -", "hnbctl install", "kubectl apply", "Run the following", "kubeadm init"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("managed refusal leaked a local execution fallback %q: %s", forbidden, combined)
		}
	}
	after, _ := os.ReadDir(dir)
	if len(after) != len(before) {
		t.Fatalf("managed refusal wrote artifacts: %v -> %v", before, after)
	}
}

// Scenario: 同义 CreateKubernetesTarget 请求 -> 幂等; 未实现时必须非成功且
// 不得伪造 operationId (idempotent replay belongs to task 4.7).
func TestClusterCreateDoesNotFabricateOperation(t *testing.T) {
	for _, sub := range []string{"create", "import"} {
		code, out, errOut := runCLI(t, "cluster", sub)
		if code == ExitSuccess {
			t.Fatalf("cluster %s must not report success before the server contract ships", sub)
		}
		combined := out + errOut
		for _, forbidden := range []string{"operationId\":", "\"status\": \"succeeded", "kubeadm", "ssh "} {
			if strings.Contains(combined, forbidden) {
				t.Fatalf("cluster %s fabricated execution output %q: %s", sub, forbidden, combined)
			}
		}
	}
}

// Scenario: node add 是服务端权威动作 —— 平台不可达时不本地回退，也不伪造
// Operation；声明文件缺 SSH 凭据时本地拒绝。
func TestNodeAddNeverFabricatesWithoutPlatform(t *testing.T) {
	dir := t.TempDir()
	nodesPath := filepath.Join(dir, "nodes.yaml")
	if err := os.WriteFile(nodesPath, []byte("sshSecretRef:\n  provider: vault\n  scope: tenant-a\n  name: cluster-ssh\nnodes:\n  - name: worker-3\n    role: worker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token := writeTokenFile(t)
	code, out, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", nodesPath,
		"--platform-url", "https://127.0.0.1:1", "--token-file", token, "--idempotency-key", "node-add-42",
		"--format", "json", "--timeout", "1s", "--expected-version", "3")
	if code != ExitConnectionFailed {
		t.Fatalf("unreachable platform must map to connection-failed exit, got %d (%s %s)", code, out, errOut)
	}
	combined := out + errOut
	for _, forbidden := range []string{"operationId\":", "\"status\": \"succeeded", "kubeadm", "ssh "} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("node add fabricated execution output %q: %s", forbidden, combined)
		}
	}
	if !strings.Contains(errOut, "PLATFORM_UNAVAILABLE") {
		t.Fatalf("expected PLATFORM_UNAVAILABLE reason: %s", errOut)
	}
}

func TestNodeAddRefusesDeclarationWithoutSSHSecret(t *testing.T) {
	dir := t.TempDir()
	nodesPath := filepath.Join(dir, "nodes.yaml")
	if err := os.WriteFile(nodesPath, []byte("nodes:\n  - name: worker-3\n    role: worker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", nodesPath)
	if code != ExitUsage {
		t.Fatalf("missing sshSecretRef must be refused locally, got %d (%s)", code, errOut)
	}
	if !strings.Contains(errOut, "sshSecretRef") {
		t.Fatalf("expected sshSecretRef hint: %s", errOut)
	}
}

// A valid but uncertified bootstrap config still refuses install and leaves the
// working directory untouched: no receipt, no run state, no plan file.
func TestBootstrapInstallRefusalLeavesNoRunArtifacts(t *testing.T) {
	dir := t.TempDir()
	configPath := writeBootstrapDraft(t, dir)
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "install", "-f", configPath, "--output", filepath.Join(dir, "plan.json"))
	if code == ExitSuccess {
		t.Fatalf("bootstrap install must not succeed before 4.4/4.5: %s %s", out, errOut)
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("refused bootstrap wrote artifacts: %v -> %v", before, after)
	}
}
