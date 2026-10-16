package toolkit

import (
	"fmt"
	"time"
)

// Backup/restore planning is a mechanism: it records what a drill must prove and
// refuses unsafe restore targets. It performs no destructive action. A drill
// claim is never inferred from a plan — RTO/RPO stay unverified until a real
// run supplies evidence.

// RestoreKind distinguishes resource rollback from data recovery, because a
// schema migration cannot be reversed by rolling back resources (PINST-006).
type RestoreKind string

const (
	RestoreResourceRollback RestoreKind = "resource-rollback"
	RestoreBackupRestore    RestoreKind = "backup-restore"
	RestoreForwardMigrate   RestoreKind = "forward-migrate"
)

// RestorePlan is one approved recovery path. It binds a backup to the install
// identity that owns it, so a restore cannot target a foreign or shared base.
type RestorePlan struct {
	SchemaVersion string      `json:"schemaVersion"`
	InstallRunID  string      `json:"installRunId"`
	ConfigDigest  string      `json:"configDigest"`
	LockDigest    string      `json:"lockDigest"`
	Kind          RestoreKind `json:"kind"`
	// OwnedBaseUID is the management-base identity this install created. A
	// restore requires it to match the target, preventing cross-install or
	// foreign-cluster restores.
	OwnedBaseUID string `json:"ownedBaseUid"`
	BackupID     string `json:"backupId"`
	BackupDigest string `json:"backupDigest"`
	// RequiresBackupCheck marks paths that must verify backup existence and
	// compatibility before proceeding.
	RequiresBackupCheck bool   `json:"requiresBackupCheck"`
	TargetOwner         string `json:"targetOwner"`
	// ClaimsForwardMigrationReversal is rejected for resource rollbacks: a
	// resource rollback can never reverse a forward-only schema migration.
	ClaimsForwardMigrationReversal bool      `json:"claimsForwardMigrationReversal"`
	CreatedAt                      time.Time `json:"createdAt"`
}

// RestoreObservation is evidence supplied by a caller after (or during) a drill.
type RestoreObservation struct {
	StartedAt            time.Time      `json:"startedAt"`
	FinishedAt           time.Time      `json:"finishedAt"`
	Completed            bool           `json:"completed"`
	RTOAchieved          *time.Duration `json:"rtoAchieved,omitempty"`
	RPOAchieved          *time.Duration `json:"rpoAchieved,omitempty"`
	VerifiedBackupDigest string         `json:"verifiedBackupDigest,omitempty"`
}

// RestoreDrill records whether RTO/RPO were actually demonstrated.
type RestoreDrill struct {
	PlanID      string              `json:"planId"`
	ApprovedRTO time.Duration       `json:"approvedRto"`
	ApprovedRPO time.Duration       `json:"approvedRpo"`
	Observation *RestoreObservation `json:"observation,omitempty"`
	RTOVerified bool                `json:"rtoVerified"`
	RPOVerified bool                `json:"rpoVerified"`
}

// ValidateRestorePlan enforces the identity binding and the forward-migration
// boundary before any restore action is considered.
func ValidateRestorePlan(plan *RestorePlan, targetOwner, targetBaseUID, availableBackupDigest string) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	switch plan.Kind {
	case RestoreResourceRollback, RestoreBackupRestore, RestoreForwardMigrate:
	default:
		return fmt.Errorf("unsupported restore kind %q", plan.Kind)
	}
	if plan.InstallRunID == "" || plan.ConfigDigest == "" || plan.OwnedBaseUID == "" {
		return fmt.Errorf("restore plan must bind install identity and owned management base")
	}
	// The target must be the base this install owns — never a shared or foreign
	// cluster (PINST-006 / PINST-007).
	if targetOwner != plan.TargetOwner {
		return fmt.Errorf("restore target owner mismatch; refusing to act on a cluster this install does not own")
	}
	if targetBaseUID != plan.OwnedBaseUID {
		return fmt.Errorf("restore target management-base UID mismatch; refusing cross-install restore")
	}
	// Backup-restore requires a verified, matching backup.
	if plan.RequiresBackupCheck {
		if plan.Kind != RestoreBackupRestore {
			return fmt.Errorf("only backup-restore may require a backup check")
		}
		if plan.BackupDigest == "" || availableBackupDigest == "" {
			return fmt.Errorf("backup-restore requires an available backup")
		}
		if plan.BackupDigest != availableBackupDigest {
			return fmt.Errorf("available backup digest does not match the planned backup")
		}
	}
	// A resource rollback can never claim to reverse a forward data migration.
	if plan.Kind == RestoreResourceRollback && plan.ClaimsForwardMigrationReversal {
		return fmt.Errorf("resource rollback cannot reverse a data migration; use forward-migrate or backup-restore")
	}
	return nil
}

// EvaluateDrill records RTO/RPO evidence. It refuses to mark a drill verified
// without an actual completed observation inside the approved budget.
func EvaluateDrill(drill *RestoreDrill, obs *RestoreObservation, approvedRTO, approvedRPO time.Duration) error {
	if drill == nil {
		return fmt.Errorf("restore drill is required")
	}
	drill.ApprovedRTO, drill.ApprovedRPO = approvedRTO, approvedRPO
	if obs == nil {
		drill.RTOVerified, drill.RPOVerified = false, false
		return fmt.Errorf("no observation supplied; RTO/RPO remain unverified")
	}
	drill.Observation = obs
	if !obs.Completed || obs.FinishedAt.IsZero() || obs.StartedAt.IsZero() || obs.FinishedAt.Before(obs.StartedAt) {
		return fmt.Errorf("observation does not represent a completed recovery")
	}
	duration := obs.FinishedAt.Sub(obs.StartedAt)
	if obs.RTOAchieved != nil {
		duration = *obs.RTOAchieved
	}
	if approvedRTO > 0 && duration <= approvedRTO {
		drill.RTOVerified = true
	}
	if approvedRPO > 0 && obs.RPOAchieved != nil && *obs.RPOAchieved <= approvedRPO {
		drill.RPOVerified = true
	}
	if !drill.RTOVerified || !drill.RPOVerified {
		return fmt.Errorf("observed recovery is outside the approved RTO/RPO budget")
	}
	return nil
}

// ForwardMigrationRequired reports whether a resource rollback would cross a
// forward-only schema migration, so operators are told to recover data instead.
func ForwardMigrationRequired(fromLockDigest, toLockDigest string, hasForwardMigration bool) bool {
	return hasForwardMigration && fromLockDigest != "" && toLockDigest != "" && fromLockDigest != toLockDigest
}
