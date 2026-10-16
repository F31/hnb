package toolkit

import (
	"strings"
	"testing"
	"time"
)

func changeNow() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }

func lockFixture(releaseID, version string, migrations ...Migration) *ReleaseLock {
	return &ReleaseLock{SchemaVersion: LockSchemaVersion, ReleaseID: releaseID, ReleaseVersion: version,
		Images: []PinnedImage{
			{Name: "apiserver", Ref: "registry.invalid/apiserver:v1", Digest: "sha256:" + strings.Repeat("a", 64)},
			{Name: "portal", Ref: "registry.invalid/portal:v1", Digest: "sha256:" + strings.Repeat("b", 64)},
		},
		Migrations:    migrations,
		Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
}

const sumOne = "1111111111111111111111111111111111111111111111111111111111111111"

// 版本 diff 正确识别新增/变更/移除的镜像与迁移.
func TestDiffLocksDetectsArtifactAndMigrationChanges(t *testing.T) {
	from := lockFixture("r1", "1.0", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	to := lockFixture("r2", "1.1", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne},
		Migration{Owner: "platform", Name: "002_more.sql", Checksum: "2222222222222222222222222222222222222222222222222222222222222222"})
	to.Images = append(to.Images, PinnedImage{Name: "extension-controller", Ref: "registry.invalid/ext:v1", Digest: "sha256:" + strings.Repeat("c", 64)})
	// portal is dropped from the release (index 1 replaced by a differently named image).
	to.Images[1] = PinnedImage{Name: "portal-legacy", Ref: "registry.invalid/portal:v1", Digest: "sha256:" + strings.Repeat("b", 64)}

	diff, err := DiffLocks(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if diff.FromReleaseID != "r1" || diff.ToVersion != "1.1" {
		t.Fatalf("wrong diff identity: %+v", diff)
	}
	added, removed := 0, 0
	for _, change := range diff.Changed {
		if change.To != "" && change.From == "" {
			added++
		}
		if change.From != "" && change.To == "" {
			removed++
		}
	}
	if added != 2 || removed != 1 { // extension-controller added, portal removed
		t.Fatalf("artifact delta wrong: %+v", diff.Changed)
	}
	if len(diff.Migrations) != 1 || diff.Migrations[0].Change != "added" {
		t.Fatalf("migration delta wrong: %+v", diff.Migrations)
	}
	if diff.DestructiveMigration {
		t.Fatal("adding a migration is not destructive")
	}
}

// 计划固定版本：目标必须是内容摘要，不能是可变引用.
func TestPlanPinsTargetByContentIdentity(t *testing.T) {
	from := lockFixture("r1", "1.0")
	to := lockFixture("r2", "1.1")
	fromDigest := lockContentDigest(from)
	plan, err := BuildPlatformChangePlan("upgrade", from, to, fromDigest, true, "", "", changeNow())
	if err != nil {
		t.Fatal(err)
	}
	if plan.FromLockDigest != fromDigest || plan.ToLockDigest != lockContentDigest(to) {
		t.Fatalf("plan is not pinned by digest: %+v", plan)
	}
	if plan.ToReleaseID != "r2" || plan.ToVersion != "1.1" {
		t.Fatalf("target identity wrong: %+v", plan)
	}
	// An unpinned "from" is refused.
	if _, err := BuildPlatformChangePlan("upgrade", from, to, "", true, "", "", changeNow()); err == nil {
		t.Fatal("plan accepted without the current lock identity")
	}
	if _, err := BuildPlatformChangePlan("upgrade", from, to, "not-a-digest", true, "", "", changeNow()); err == nil {
		t.Fatal("plan accepted a non-digest current identity")
	}
}

// 强制 DB 兼容检查.
func TestPlanForcesDatabaseCompatibility(t *testing.T) {
	from, to := lockFixture("r1", "1.0"), lockFixture("r2", "1.1")
	plan, err := BuildPlatformChangePlan("upgrade", from, to, lockContentDigest(from), false, "", "", changeNow())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized {
		t.Fatal("plan authorized without database compatibility")
	}
	if !containsSubstring(plan.Blocked, "DB_COMPATIBILITY_UNVERIFIED") {
		t.Fatalf("missing DB compatibility block: %v", plan.Blocked)
	}
}

// 破坏性迁移差异强制备份检查（PINST-006）.
func TestDestructiveMigrationForcesVerifiedBackup(t *testing.T) {
	from := lockFixture("r1", "1.0", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	// The applied migration's bytes changed: destructive.
	to := lockFixture("r2", "1.1", Migration{Owner: "platform", Name: "001_init.sql", Checksum: "3333333333333333333333333333333333333333333333333333333333333333"})
	fromDigest := lockContentDigest(from)

	// No backup recorded -> blocked.
	plan, err := BuildPlatformChangePlan("upgrade", from, to, fromDigest, true, "", "", changeNow())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !containsSubstring(plan.Blocked, "BACKUP_REQUIRED") {
		t.Fatalf("destructive migration did not force a backup: %+v", plan)
	}
	if !plan.RequiresBackup {
		t.Fatal("plan did not mark a backup as required")
	}
	// Recorded backup not available -> blocked.
	plan, _ = BuildPlatformChangePlan("upgrade", from, to, fromDigest, true, "sha256:"+strings.Repeat("d", 64), "sha256:"+strings.Repeat("e", 64), changeNow())
	if plan.Authorized || !containsSubstring(plan.Blocked, "BACKUP_UNVERIFIED") {
		t.Fatalf("unavailable backup did not block: %+v", plan)
	}
	// Verified backup -> authorized.
	backup := "sha256:" + strings.Repeat("d", 64)
	plan, _ = BuildPlatformChangePlan("upgrade", from, to, fromDigest, true, backup, backup, changeNow())
	if !plan.Authorized || !plan.BackupVerified {
		t.Fatalf("verified backup did not authorize the plan: %+v", plan)
	}
}

// 移除迁移同样属于破坏性差异.
func TestRemovedMigrationIsDestructive(t *testing.T) {
	from := lockFixture("r1", "1.0", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne},
		Migration{Owner: "platform", Name: "002_old.sql", Checksum: sumOne})
	to := lockFixture("r2", "1.1", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	diff, err := DiffLocks(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.DestructiveMigration {
		t.Fatalf("removing a migration not treated as destructive: %+v", diff.Migrations)
	}
}

// 资源回滚不得宣称已恢复数据.
func TestResourceRollbackCannotClaimDataRestored(t *testing.T) {
	from := lockFixture("r1", "1.1", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	to := lockFixture("r2", "1.0", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	plan, err := BuildPlatformChangePlan("resource-rollback", from, to, lockContentDigest(from), true, "", "", changeNow())
	if err != nil {
		t.Fatal(err)
	}
	if plan.ClaimsDataRestored {
		t.Fatalf("resource rollback claimed data restoration: %+v", plan)
	}
	// With a destructive delta it is refused outright.
	from2 := lockFixture("r1", "1.1", Migration{Owner: "platform", Name: "001_init.sql", Checksum: sumOne})
	to2 := lockFixture("r2", "1.0")
	plan, _ = BuildPlatformChangePlan("resource-rollback", from2, to2, lockContentDigest(from2), true, "", "", changeNow())
	if plan.Authorized || !containsSubstring(plan.Blocked, "RESOURCE_ROLLBACK_CANNOT_RESTORE_DATA") {
		t.Fatalf("resource rollback across a destructive delta not refused: %+v", plan)
	}
}

// 相同锁不产生变更计划.
func TestIdenticalLocksAreNoChange(t *testing.T) {
	lock := lockFixture("r1", "1.0")
	plan, err := BuildPlatformChangePlan("upgrade", lock, lock, lockContentDigest(lock), true, "", "", changeNow())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !containsSubstring(plan.Blocked, "NO_CHANGE") {
		t.Fatalf("identical locks produced an authorized change: %+v", plan)
	}
}

// API 不可用时拒绝通用目标执行，无本地回退.
func TestGenericTargetExecutionRefusedWhenAPIDown(t *testing.T) {
	if err := AuthorizeGenericTargetChange(true); err != nil {
		t.Fatalf("reachable platform refused: %v", err)
	}
	err := AuthorizeGenericTargetChange(false)
	if err == nil {
		t.Fatal("generic target execution authorized while the platform API is down")
	}
	if !strings.Contains(err.Error(), "PLATFORM_UNAVAILABLE") {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

func TestUnsupportedDirectionRefused(t *testing.T) {
	lock := lockFixture("r1", "1.0")
	if _, err := BuildPlatformChangePlan("sideways", lock, lock, lockContentDigest(lock), true, "", "", changeNow()); err == nil {
		t.Fatal("unsupported change direction accepted")
	}
	if _, err := DiffLocks(nil, lock); err == nil {
		t.Fatal("diff accepted a missing lock")
	}
}
