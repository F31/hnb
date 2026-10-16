package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

func writeLockFile(t *testing.T, name, releaseID, version string, images []toolkit.PinnedImage, migrations []toolkit.Migration) string {
	t.Helper()
	lock := toolkit.ReleaseLock{SchemaVersion: toolkit.LockSchemaVersion, ReleaseID: releaseID, ReleaseVersion: version,
		Images: images, Migrations: migrations,
		Compatibility: toolkit.LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}, Profiles: []string{"Minimal"}}}
	data, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pinnedImage(name, hex string) toolkit.PinnedImage {
	return toolkit.PinnedImage{Name: name, Ref: "registry.invalid/" + name + ":v1", Digest: "sha256:" + strings.Repeat(hex, 64)}
}

// 平台升级计划固定目标版本，并在 DB 兼容缺失时拒绝.
func TestPlatformUpgradePlanPinsTargetAndRequiresCompatibility(t *testing.T) {
	current := writeLockFile(t, "current.yaml", "r1", "1.0", []toolkit.PinnedImage{pinnedImage("apiserver", "a")}, nil)
	target := writeLockFile(t, "target.yaml", "r2", "1.1", []toolkit.PinnedImage{pinnedImage("apiserver", "b")}, nil)

	code, out, errOut := runCLI(t, "platform", "upgrade", "--current-lock", current, "--release-lock", target,
		"--api-reachable", "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("upgrade reported success: %s %s", out, errOut)
	}
	var plan toolkit.PlatformChangePlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("invalid plan JSON: %v (%s)", err, out)
	}
	if plan.Direction != "upgrade" || plan.ToReleaseID != "r2" || plan.ToVersion != "1.1" {
		t.Fatalf("target not pinned: %+v", plan)
	}
	if !strings.HasPrefix(plan.ToLockDigest, "sha256:") || !strings.HasPrefix(plan.FromLockDigest, "sha256:") {
		t.Fatalf("plan is not pinned by content digest: %+v", plan)
	}
	if plan.Authorized || !planContains(plan.Blocked, "DB_COMPATIBILITY_UNVERIFIED") {
		t.Fatalf("missing DB compatibility gate: %+v", plan)
	}
}

// API 不可用时拒绝通用目标执行，且不产生本地回退.
func TestPlatformUpgradeRefusedWhenAPIIsDown(t *testing.T) {
	current := writeLockFile(t, "current.yaml", "r1", "1.0", []toolkit.PinnedImage{pinnedImage("apiserver", "a")}, nil)
	target := writeLockFile(t, "target.yaml", "r2", "1.1", []toolkit.PinnedImage{pinnedImage("apiserver", "b")}, nil)
	code, out, errOut := runCLI(t, "platform", "upgrade", "--current-lock", current, "--release-lock", target)
	if code != ExitConnectionFailed {
		t.Fatalf("platform change with an unreachable API must fail with connection-failed, got %d (%s %s)", code, out, errOut)
	}
	combined := out + errOut
	for _, forbidden := range []string{"kubeadm", "ssh ", "bootstrap", "falling back"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("unreachable API produced local execution output %q: %s", forbidden, combined)
		}
	}
}

// 破坏性迁移差异强制已验证备份，缺失时拒绝.
func TestPlatformUpgradeRequiresVerifiedBackupForDestructiveMigration(t *testing.T) {
	current := writeLockFile(t, "current.yaml", "r1", "1.0", []toolkit.PinnedImage{pinnedImage("apiserver", "a")},
		[]toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.Repeat("1", 64)}})
	target := writeLockFile(t, "target.yaml", "r2", "1.1", []toolkit.PinnedImage{pinnedImage("apiserver", "a")},
		[]toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.Repeat("9", 64)}})

	// DB compatible but no backup.
	code, out, _ := runCLI(t, "platform", "upgrade", "--current-lock", current, "--release-lock", target,
		"--api-reachable", "--db-compatible", "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("destructive migration authorized without a backup: %s", out)
	}
	var plan toolkit.PlatformChangePlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresBackup || !planContains(plan.Blocked, "BACKUP_REQUIRED") {
		t.Fatalf("backup requirement not enforced: %+v", plan)
	}
	if plan.ClaimsDataRestored {
		t.Fatal("upgrade claimed data restoration")
	}
}

// 资源回滚不得宣称恢复数据.
func TestPlatformRollbackCannotClaimDataRestored(t *testing.T) {
	current := writeLockFile(t, "current.yaml", "r1", "1.1", []toolkit.PinnedImage{pinnedImage("apiserver", "a")},
		[]toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.Repeat("1", 64)}})
	target := writeLockFile(t, "target.yaml", "r2", "1.0", []toolkit.PinnedImage{pinnedImage("apiserver", "a")},
		[]toolkit.Migration{{Owner: "platform", Name: "001_init.sql", Checksum: strings.Repeat("1", 64)}})
	code, out, _ := runCLI(t, "platform", "rollback", "--current-lock", current, "--release-lock", target,
		"--api-reachable", "--db-compatible", "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("rollback reported success: %s", out)
	}
	var plan toolkit.PlatformChangePlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Direction != "resource-rollback" || plan.ClaimsDataRestored {
		t.Fatalf("rollback claimed data restoration: %+v", plan)
	}
}

func TestPlatformChangeRequiresBothLocks(t *testing.T) {
	code, _, errOut := runCLI(t, "platform", "upgrade", "--release-lock", "x.yaml")
	if code == ExitSuccess || !strings.Contains(errOut, "--current-lock") {
		t.Fatalf("upgrade without both locks accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "platform", "upgrade", "--current-lock", "missing.yaml", "--release-lock", "missing.yaml", "--api-reachable")
	if code == ExitSuccess {
		t.Fatalf("unreadable locks accepted: %s", errOut)
	}
}
