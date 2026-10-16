package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

const RecoveryBackupSchema = "recovery-backup/v1"

// RecoveryBackup is a local correlation/identity manifest, not platform truth.
// A verified manifest proves file integrity, not live ownership or restore safety.
type RecoveryBackup struct {
	SchemaVersion        string                `yaml:"schemaVersion" json:"schemaVersion"`
	BackupID             string                `yaml:"backupId" json:"backupId"`
	RunID                string                `yaml:"runId" json:"runId"`
	ConfigDigest         string                `yaml:"configDigest" json:"configDigest"`
	LockDigest           string                `yaml:"lockDigest" json:"lockDigest"`
	ManagementClusterUID string                `yaml:"managementClusterUID" json:"managementClusterUID"`
	Namespace            string                `yaml:"namespace" json:"namespace"`
	SnapshotAt           time.Time             `yaml:"snapshotAt" json:"snapshotAt"`
	Entries              []RecoveryBackupEntry `yaml:"entries" json:"entries"`
}

type RecoveryBackupEntry struct {
	Kind     string `yaml:"kind" json:"kind"` // postgresql | jetstream | identity | kms
	Plane    string `yaml:"plane,omitempty" json:"plane,omitempty"`
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	Role     string `yaml:"role,omitempty" json:"role,omitempty"`
	Path     string `yaml:"path" json:"path"` // relative, regular file; never exported
	Digest   string `yaml:"digest" json:"digest"`
	Size     int64  `yaml:"size" json:"size"`
}

type RecoveryFileCheck struct {
	Kind   string `yaml:"kind" json:"kind"`
	Plane  string `yaml:"plane,omitempty" json:"plane,omitempty"`
	Digest string `yaml:"digest" json:"digest"`
	Size   int64  `yaml:"size" json:"size"`
}

type RecoveryInputReport struct {
	SchemaVersion          string              `yaml:"schemaVersion" json:"schemaVersion"`
	Scope                  string              `yaml:"scope" json:"scope"`
	RunID                  string              `yaml:"runId" json:"runId"`
	BackupID               string              `yaml:"backupId" json:"backupId"`
	ConfigDigest           string              `yaml:"configDigest" json:"configDigest"`
	LockDigest             string              `yaml:"lockDigest" json:"lockDigest"`
	SnapshotAt             time.Time           `yaml:"snapshotAt" json:"snapshotAt"`
	VerifiedAt             time.Time           `yaml:"verifiedAt" json:"verifiedAt"`
	Checked                []RecoveryFileCheck `yaml:"checked" json:"checked"`
	InputsVerified         bool                `yaml:"inputsVerified" json:"inputsVerified"`
	TargetIdentityObserved bool                `yaml:"targetIdentityObserved" json:"targetIdentityObserved"`
	RestoreAuthorized      bool                `yaml:"restoreAuthorized" json:"restoreAuthorized"`
	Applied                bool                `yaml:"applied" json:"applied"`
	RTOVerified            bool                `yaml:"rtoVerified" json:"rtoVerified"`
	RPOVerified            bool                `yaml:"rpoVerified" json:"rpoVerified"`
	Blockers               []string            `yaml:"blockers" json:"blockers"`
}

// ReadRecoveryInput refuses broad permissions, symlinks, special files and
// unbounded input. Errors never echo a sensitive file's path or contents.
func ReadRecoveryInput(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 || fi.Size() > 1<<20 {
		return nil, fmt.Errorf("recovery input must be a private regular file no larger than 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("recovery input unreadable")
	}
	return data, nil
}

func DecodeRecoveryBackup(data []byte) (*RecoveryBackup, error) {
	var backup RecoveryBackup
	if err := decodeStrict(data, &backup); err != nil {
		return nil, fmt.Errorf("invalid recovery backup manifest")
	}
	if backup.SchemaVersion != RecoveryBackupSchema {
		return nil, fmt.Errorf("unsupported recovery backup schema")
	}
	return &backup, nil
}

// VerifyRecoveryBackup is intentionally read-only. It never acquires a writer
// lease, edits receipt/Operation history, contacts a target, or runs SQL/shell.
func VerifyRecoveryBackup(ctx context.Context, cfg *DeploymentConfig, lockData []byte, receipt *BootstrapReceipt, backup *RecoveryBackup, dir string) (RecoveryInputReport, error) {
	report := RecoveryInputReport{SchemaVersion: "recovery-input-report/v1", Scope: "local-backup-inputs", VerifiedAt: time.Now().UTC(), Blockers: []string{"TARGET_IDENTITY_NOT_OBSERVED", "RESTORE_EXECUTOR_UNAVAILABLE"}}
	if cfg == nil || receipt == nil || backup == nil {
		return report, fmt.Errorf("configuration, initialization receipt and backup manifest are required")
	}
	if len(cfg.Validate(CurrentEvidence)) > 0 {
		return report, fmt.Errorf("invalid deployment configuration")
	}
	if cfg.Spec.Mode != ModeBootstrap || cfg.Spec.Cluster.Strategy != StrategyProvision {
		return report, fmt.Errorf("field management-base recovery must not target an external or managed cluster")
	}
	if cfg.Spec.Platform.Infrastructure.PostgreSQL.Mode != "bundled" || cfg.Spec.Platform.Infrastructure.NATS.Mode != "bundled" {
		return report, fmt.Errorf("shared/external infrastructure requires its owner's recovery contract")
	}
	lock, err := DecodeReleaseLock(lockData)
	if err != nil || len(lock.Validate()) > 0 {
		return report, fmt.Errorf("invalid recorded release lock")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return report, fmt.Errorf("cannot compute configuration identity")
	}
	configDigest, lockDigest := digestOf(encoded), digestOf(lockData)
	if receipt.SchemaVersion != "bootstrap-receipt/v1" || !validName(receipt.RunID) || !validName(receipt.ManagementClusterUID) || receipt.StartedAt.IsZero() {
		return report, fmt.Errorf("initialization receipt identity is incomplete")
	}
	if receipt.ConfigDigest != configDigest || receipt.LockDigest != lockDigest {
		return report, fmt.Errorf("receipt does not match original configuration/release lock")
	}
	initialized := false
	for _, p := range receipt.Phases {
		if p.Name == PhaseInit && p.Status == "succeeded" && !p.FinishedAt.IsZero() && !p.FinishedAt.Before(receipt.StartedAt) {
			initialized = true
		}
	}
	ownedNamespace := false
	for _, resource := range receipt.Ownership {
		if resource == "namespace/"+cfg.Spec.Platform.Namespace {
			ownedNamespace = true
		}
	}
	if !initialized || !ownedNamespace {
		return report, fmt.Errorf("receipt lacks successful initialization and the exact owned platform namespace")
	}
	if backup.SchemaVersion != RecoveryBackupSchema || !validName(backup.BackupID) || backup.RunID != receipt.RunID || backup.ManagementClusterUID != receipt.ManagementClusterUID || backup.Namespace != cfg.Spec.Platform.Namespace || backup.ConfigDigest != configDigest || backup.LockDigest != lockDigest {
		return report, fmt.Errorf("backup belongs to a different install, base, namespace or lock")
	}
	if backup.SnapshotAt.IsZero() || backup.SnapshotAt.Before(receipt.StartedAt) || backup.SnapshotAt.After(report.VerifiedAt.Add(5*time.Minute)) {
		return report, fmt.Errorf("backup snapshot time is invalid")
	}
	if len(backup.Entries) == 0 || len(backup.Entries) > 32 {
		return report, fmt.Errorf("backup entry budget invalid")
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm()&0077 != 0 {
		return report, fmt.Errorf("backup directory must be private and not a symlink")
	}
	planes := map[string]PlaneConfig{}
	for _, p := range cfg.Spec.Platform.Infrastructure.Planes {
		planes[p.Name] = p
	}
	seen, paths := map[string]bool{}, map[string]bool{}
	for _, entry := range backup.Entries {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("recovery verification canceled")
		}
		key := entry.Kind + "/" + entry.Plane
		if seen[key] || paths[entry.Path] || !validBundleDigest(entry.Digest) || entry.Size <= 0 || entry.Size > 16<<30 {
			return report, fmt.Errorf("duplicate or invalid backup identity")
		}
		seen[key], paths[entry.Path] = true, true
		switch entry.Kind {
		case "postgresql":
			plane, ok := planes[entry.Plane]
			if !ok || entry.Database != plane.Database || entry.Role != plane.Role {
				return report, fmt.Errorf("PG backup does not match an owned plane/database/role")
			}
		case "jetstream", "identity", "kms":
			if entry.Plane != "" || entry.Database != "" || entry.Role != "" {
				return report, fmt.Errorf("invalid non-PG backup scope")
			}
		default:
			return report, fmt.Errorf("unsupported backup material kind")
		}
		path, err := bundleSafePath(dir, entry.Path)
		if err != nil {
			return report, fmt.Errorf("backup material missing or outside its private directory")
		}
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode().Perm()&0077 != 0 || fi.Size() != entry.Size {
			return report, fmt.Errorf("backup material is not private or has incorrect size")
		}
		if err := verifyRecoveryFile(ctx, path, entry); err != nil {
			return report, err
		}
		report.Checked = append(report.Checked, RecoveryFileCheck{Kind: entry.Kind, Plane: entry.Plane, Digest: entry.Digest, Size: entry.Size})
	}
	for plane := range planes {
		if !seen["postgresql/"+plane] {
			return report, fmt.Errorf("missing owned-plane PostgreSQL backup")
		}
	}
	for _, kind := range []string{"jetstream", "identity", "kms"} {
		if !seen[kind+"/"] {
			return report, fmt.Errorf("missing message/identity/KMS backup material")
		}
	}
	sort.Slice(report.Checked, func(i, j int) bool {
		return report.Checked[i].Kind+report.Checked[i].Plane < report.Checked[j].Kind+report.Checked[j].Plane
	})
	report.RunID, report.BackupID, report.ConfigDigest, report.LockDigest, report.SnapshotAt = receipt.RunID, backup.BackupID, configDigest, lockDigest, backup.SnapshotAt.UTC()
	report.InputsVerified = true
	return report, nil
}

func verifyRecoveryFile(ctx context.Context, path string, entry RecoveryBackupEntry) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("backup file unreadable")
	}
	defer f.Close()
	h := sha256.New()
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if ctx.Err() != nil {
			return fmt.Errorf("recovery verification canceled")
		}
		n, err := f.Read(buffer)
		if n > 0 {
			size += int64(n)
			if size > entry.Size {
				return fmt.Errorf("backup changed while being verified")
			}
			h.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("backup transfer interrupted")
		}
	}
	if size != entry.Size || fmt.Sprintf("sha256:%x", h.Sum(nil)) != entry.Digest {
		return fmt.Errorf("backup checksum differs from recorded snapshot")
	}
	return nil
}
