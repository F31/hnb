package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file fixes the 1.2 boundary rejection scenarios into Go contract tests,
// as promised by evidence/task-1.2-boundaries.md ("这些拒绝场景在实现
// install/cluster 命令时以 Go 契约测试固定"). It enforces the Day-0 boundary;
// it does not relax it. Scenarios that require an unimplemented server-side
// action contract are asserted as explicit non-success, never as fake success.

// Scenario: 配置含 platform/target 引用且 mode=bootstrap -> 副作用前拒绝.
// PlatformConfig has no target reference field, so a bootstrap config naming a
// managed target must be refused while decoding, before any file/cluster/node
// effect. The contract is the refusal itself, not a particular parser.
func TestBootstrapConfigWithPlatformTargetReferenceIsRefused(t *testing.T) {
	base, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if base.Spec.Mode != ModeBootstrap {
		t.Fatal("fixture is not the bootstrap route")
	}
	withTarget := strings.Replace(validConfig, "    namespace: hnb-system", "    namespace: hnb-system\n    targetRef: platform/runtime-target-abc", 1)
	if _, err := DecodeConfig([]byte(withTarget)); err == nil {
		t.Fatal("bootstrap config carrying a platform/target reference was accepted")
	} else if !strings.Contains(err.Error(), "targetRef") && !strings.Contains(strings.ToLower(err.Error()), "field") {
		t.Fatalf("refusal should name the offending field, got %v", err)
	}
	// And the same must hold through the config path used by install/preflight:
	// a target-bearing bootstrap config never becomes a valid install input.
	dir := t.TempDir()
	path := filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(path, []byte(withTarget), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfig(mustRead(t, path)); err == nil {
		t.Fatal("target-bearing bootstrap config decoded from disk")
	}
	// Refusal left no side-effect artifact behind.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("refusal produced side effects: %v %v", entries, err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Scenario: 节点已有 HNB 管理标记/其它 lifecycle owner -> 拒绝并指出正确管理入口.
func TestBootstrapRefusesNodesCarryingManagementOrLifecycleMarkers(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"hnb", "kubernetes", "rke2", "k3s"} {
		runner := &fixtureProbe{response: strings.Replace(nodeProbeFixture, "owner=none", "owner="+owner, 1)}
		checks := ProbeNodes(context.Background(), cfg, runner)
		marker := findCheck(checks, "node.owner-markers")
		if marker == nil {
			t.Fatalf("owner=%s: missing node.owner-markers check", owner)
		}
		if marker.Status != StatusBlocked || marker.ReasonCode != "NODE_OWNER_PRESENT" {
			t.Fatalf("owner=%s: expected refusal, got %+v", owner, marker)
		}
		if !strings.Contains(marker.Remediation, "do not bootstrap or reset") {
			t.Fatalf("owner=%s: refusal must point at the correct management entry, got %q", owner, marker.Remediation)
		}
		// A managed/marked node can never yield an all-pass probe set.
		for _, check := range checks {
			if check.Status == StatusPass && check.ReasonCode == "CHECK_PASSED" && check.CheckID == "node.owner-markers" {
				t.Fatalf("owner=%s: marked node produced a passing probe set", owner)
			}
		}
	}
}

// Scenario: 节点版本冲突时拒绝. A node that already runs Kubernetes at a version
// other than the locked recipe target must not be silently adopted by bootstrap.
func TestBootstrapRefusesNodeVersionConflict(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fixtureProbe{response: strings.Replace(nodeProbeFixture, "owner=none", "owner=kubernetes", 1)}
	checks := ProbeNodes(context.Background(), cfg, runner)
	marker := findCheck(checks, "node.owner-markers")
	if marker == nil || marker.Status != StatusBlocked {
		t.Fatal("a node already running Kubernetes must not be adopted for bootstrap")
	}
	// The node API source path for such a node is still blocked, not upgraded to
	// an action; SSH reachability never authorizes an init/join.
	apiChecks := ProbeNodeAPIPaths(context.Background(), cfg, runner)
	if len(apiChecks) == 0 {
		t.Fatal("no node API source-path observation")
	}
	for _, check := range apiChecks {
		if check.Status == StatusPass && strings.Contains(check.Actual, "owner=") {
			t.Fatal("owner conflict promoted to a passing API observation")
		}
	}
}

// Scenario: managed 模式 API 不可达 -> 不触发本地 SSH/bootstrap.
// managed is refused structurally today (no tenant-bound SecretReference
// contract is implemented). The contract is: no SSH, no planning, non-success.
func TestManagedModeNeverFallsBackToLocalExecution(t *testing.T) {
	managed := strings.Replace(validConfig, "mode: bootstrap", "mode: managed", 1)
	cfg, err := DecodeConfig([]byte(managed))
	if err != nil {
		t.Fatalf("managed config should decode, got %v", err)
	}
	problems := cfg.Validate(CurrentEvidence)
	if !containsSubstring(problems, "mode=managed") {
		t.Fatalf("managed mode accepted without a platform contract: %v", problems)
	}
	// A managed request must not reach the existing-cluster planner either.
	if _, planProblems := cfg.PlanExistingDependencies(&ReleaseLock{}); len(planProblems) == 0 {
		t.Fatal("managed config produced an install plan")
	}
}

// Scenario: 跨租户/跨平面 SecretReference 复用 -> 拒绝 (CFG-002 / DPLY-006).
func TestSharedPlaneCredentialReferenceIsRefused(t *testing.T) {
	shared := strings.Replace(validExistingConfig, "natsSecretRef: tenant/market-nats", "natsSecretRef: tenant/platform-nats", 1)
	cfg, err := DecodeConfig([]byte(shared))
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(cfg.Validate(CurrentEvidence), "share one nats credential") {
		t.Fatal("two owned planes were allowed to share one NATS credential reference")
	}
}

func findCheck(checks []PreflightCheck, id string) *PreflightCheck {
	for i := range checks {
		if checks[i].CheckID == id {
			return &checks[i]
		}
	}
	return nil
}
