package toolkit

import (
	"strings"
	"testing"
	"time"
)

func backupPlanFixture() *RestorePlan {
	return &RestorePlan{SchemaVersion: "restore-plan/v1", InstallRunID: "run-1",
		ConfigDigest: "sha256:aaaa", LockDigest: "sha256:bbbb", Kind: RestoreBackupRestore,
		OwnedBaseUID: "uid-mine", BackupID: "backup-1", BackupDigest: "sha256:backup",
		RequiresBackupCheck: true, TargetOwner: "hnb-management-base",
		CreatedAt: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
}

// A restore may only act on the management base this install owns.
func TestRestoreRefusesForeignOrSharedBase(t *testing.T) {
	plan := backupPlanFixture()
	if err := ValidateRestorePlan(plan, "hnb-management-base", "uid-mine", "sha256:backup"); err != nil {
		t.Fatalf("owned base refused: %v", err)
	}
	// A different install's base must be refused.
	if err := ValidateRestorePlan(plan, "hnb-management-base", "uid-other", "sha256:backup"); err == nil {
		t.Fatal("restore accepted a foreign management-base UID")
	}
	// A shared/external cluster is not the owner.
	if err := ValidateRestorePlan(plan, "external-cluster", "uid-mine", "sha256:backup"); err == nil {
		t.Fatal("restore accepted a cluster this install does not own")
	}
	// An unbound plan cannot be used at all.
	unbound := backupPlanFixture()
	unbound.OwnedBaseUID = ""
	if err := ValidateRestorePlan(unbound, "hnb-management-base", "", "sha256:backup"); err == nil {
		t.Fatal("unbound restore plan accepted")
	}
}

// Backup-restore requires a matching, available backup.
func TestBackupRestoreRequiresMatchingBackup(t *testing.T) {
	plan := backupPlanFixture()
	if err := ValidateRestorePlan(plan, "hnb-management-base", "uid-mine", "sha256:different"); err == nil {
		t.Fatal("mismatched backup digest accepted")
	}
	if err := ValidateRestorePlan(plan, "hnb-management-base", "uid-mine", ""); err == nil {
		t.Fatal("missing backup accepted")
	}
	// Only backup-restore may require a backup check.
	bad := backupPlanFixture()
	bad.Kind = RestoreForwardMigrate
	if err := ValidateRestorePlan(bad, "hnb-management-base", "uid-mine", "sha256:backup"); err == nil {
		t.Fatal("forward-migrate accepted a backup check")
	}
}

// 资源回滚不得宣称已恢复前向数据迁移（PINST-006 场景）.
func TestResourceRollbackCannotClaimMigrationReversal(t *testing.T) {
	rollback := backupPlanFixture()
	rollback.Kind = RestoreResourceRollback
	rollback.RequiresBackupCheck = false
	if err := ValidateRestorePlan(rollback, "hnb-management-base", "uid-mine", ""); err != nil {
		t.Fatalf("plain resource rollback refused: %v", err)
	}
	rollback.ClaimsForwardMigrationReversal = true
	if err := ValidateRestorePlan(rollback, "hnb-management-base", "uid-mine", ""); err == nil {
		t.Fatal("resource rollback claimed to reverse a data migration")
	} else if !strings.Contains(err.Error(), "forward-migrate or backup-restore") {
		t.Fatalf("refusal must point at the correct path: %v", err)
	}
}

func TestForwardMigrationRequired(t *testing.T) {
	if !ForwardMigrationRequired("sha256:old", "sha256:new", true) {
		t.Fatal("forward migration across lock versions not detected")
	}
	if ForwardMigrationRequired("sha256:same", "sha256:same", true) {
		t.Fatal("same lock version reported as a forward migration")
	}
	if ForwardMigrationRequired("sha256:old", "sha256:new", false) {
		t.Fatal("no forward migration reported as present")
	}
}

// RTO/RPO 只有在真实完成的演练观测内才被标记 verified.
func TestDrillRequiresRealObservationInsideBudget(t *testing.T) {
	drill := &RestoreDrill{PlanID: "plan-1"}
	// No observation at all.
	if err := EvaluateDrill(drill, nil, 30*time.Minute, 15*time.Minute); err == nil {
		t.Fatal("drill verified without an observation")
	}
	if drill.RTOVerified || drill.RPOVerified {
		t.Fatal("missing observation marked RTO/RPO verified")
	}

	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	within := &RestoreObservation{StartedAt: start, FinishedAt: start.Add(20 * time.Minute), Completed: true,
		RPOAchieved: dur(5 * time.Minute)}
	if err := EvaluateDrill(drill, within, 30*time.Minute, 15*time.Minute); err != nil {
		t.Fatalf("within-budget drill refused: %v", err)
	}
	if !drill.RTOVerified || !drill.RPOVerified {
		t.Fatalf("within-budget drill not verified: %+v", drill)
	}

	// Over budget stays unverified.
	over := &RestoreObservation{StartedAt: start, FinishedAt: start.Add(2 * time.Hour), Completed: true,
		RPOAchieved: dur(90 * time.Minute)}
	drill2 := &RestoreDrill{PlanID: "plan-1"}
	if err := EvaluateDrill(drill2, over, 30*time.Minute, 15*time.Minute); err == nil {
		t.Fatal("over-budget drill verified")
	}
	if drill2.RTOVerified || drill2.RPOVerified {
		t.Fatalf("over-budget drill marked verified: %+v", drill2)
	}

	// An incomplete observation never verifies, however fast.
	incomplete := &RestoreObservation{StartedAt: start, FinishedAt: start.Add(time.Minute), Completed: false}
	drill3 := &RestoreDrill{PlanID: "plan-1"}
	if err := EvaluateDrill(drill3, incomplete, 30*time.Minute, 15*time.Minute); err == nil {
		t.Fatal("incomplete recovery verified")
	}
}

func TestUnsupportedRestoreKindRefused(t *testing.T) {
	plan := backupPlanFixture()
	plan.Kind = RestoreKind("magic-restore")
	if err := ValidateRestorePlan(plan, "hnb-management-base", "uid-mine", "sha256:backup"); err == nil {
		t.Fatal("unsupported restore kind accepted")
	}
}

func dur(d time.Duration) *time.Duration { return &d }
