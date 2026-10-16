package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func recoveryFixture(t *testing.T) (*DeploymentConfig, []byte, *BootstrapReceipt, *RecoveryBackup, string) {
	t.Helper()
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	configData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	lock := lockFixture("recovery-source", "1.0")
	lockData, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-2 * time.Hour)
	receipt := &BootstrapReceipt{SchemaVersion: "bootstrap-receipt/v1", RunID: "run-recovery", ConfigDigest: digestOf(configData), LockDigest: digestOf(lockData), ManagementClusterUID: "base-owned-uid", StartedAt: start, Ownership: []string{"namespace/" + cfg.Spec.Platform.Namespace}, Phases: []PhaseOutcome{{Name: PhaseInit, Status: "succeeded", StartedAt: start, FinishedAt: start.Add(time.Minute)}}}
	backup := &RecoveryBackup{SchemaVersion: RecoveryBackupSchema, BackupID: "backup-recovery", RunID: receipt.RunID, ConfigDigest: receipt.ConfigDigest, LockDigest: receipt.LockDigest, ManagementClusterUID: receipt.ManagementClusterUID, Namespace: cfg.Spec.Platform.Namespace, SnapshotAt: start.Add(time.Hour)}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	add := func(kind, plane, database, role string) {
		name := kind + "-" + plane + ".backup"
		data := []byte("synthetic-private-payload token=must-not-print-" + name)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		backup.Entries = append(backup.Entries, RecoveryBackupEntry{Kind: kind, Plane: plane, Database: database, Role: role, Path: name, Digest: digestOf(data), Size: int64(len(data))})
	}
	for _, p := range cfg.Spec.Platform.Infrastructure.Planes {
		add("postgresql", p.Name, p.Database, p.Role)
	}
	for _, kind := range []string{"jetstream", "identity", "kms"} {
		add(kind, "", "", "")
	}
	return cfg, lockData, receipt, backup, dir
}

func TestRecoveryVerifiesActualBytesWithoutRestoring(t *testing.T) {
	cfg, lock, receipt, backup, dir := recoveryFixture(t)
	before, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	report, err := VerifyRecoveryBackup(context.Background(), cfg, lock, receipt, backup, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.InputsVerified || len(report.Checked) != 6 || report.RestoreAuthorized || report.Applied || report.TargetIdentityObserved || report.RTOVerified || report.RPOVerified {
		t.Fatalf("local checks promoted to restore/target/RTO facts: %+v", report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"must-not-print", "synthetic-private-payload", "hnb_platform", "hnb_market", "hnb_artifact"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("backup payload/database identity leaked")
		}
	}
	after, err := json.Marshal(receipt)
	if err != nil || string(before) != string(after) {
		t.Fatal("verification rewrote initialization history")
	}
}

func TestRecoveryRefusesEveryForeignBinding(t *testing.T) {
	for name, change := range map[string]func(*BootstrapReceipt, *RecoveryBackup){
		"run":            func(_ *BootstrapReceipt, b *RecoveryBackup) { b.RunID = "foreign-run" },
		"base":           func(_ *BootstrapReceipt, b *RecoveryBackup) { b.ManagementClusterUID = "foreign-base" },
		"namespace":      func(_ *BootstrapReceipt, b *RecoveryBackup) { b.Namespace = "customer-ns" },
		"config":         func(_ *BootstrapReceipt, b *RecoveryBackup) { b.ConfigDigest = "sha256:" + strings.Repeat("f", 64) },
		"lock":           func(_ *BootstrapReceipt, b *RecoveryBackup) { b.LockDigest = "sha256:" + strings.Repeat("f", 64) },
		"receipt":        func(r *BootstrapReceipt, _ *RecoveryBackup) { r.ConfigDigest = "sha256:" + strings.Repeat("f", 64) },
		"ownership":      func(r *BootstrapReceipt, _ *RecoveryBackup) { r.Ownership = []string{"namespace/customer-ns"} },
		"init-in-flight": func(r *BootstrapReceipt, _ *RecoveryBackup) { r.Phases[0].Status = "running" },
		"snapshot":       func(r *BootstrapReceipt, b *RecoveryBackup) { b.SnapshotAt = r.StartedAt.Add(-time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			cfg, lock, r, b, dir := recoveryFixture(t)
			change(r, b)
			if _, err := VerifyRecoveryBackup(context.Background(), cfg, lock, r, b, dir); err == nil {
				t.Fatal("foreign/unproven recovery binding accepted")
			}
		})
	}
}

func TestRecoveryRefusesMissingCorruptAndUnsafeBackup(t *testing.T) {
	for name, change := range map[string]func(*RecoveryBackup, string){
		"missing-plane":  func(b *RecoveryBackup, _ string) { b.Entries = b.Entries[1:] },
		"missing-kms":    func(b *RecoveryBackup, _ string) { b.Entries = b.Entries[:len(b.Entries)-1] },
		"wrong-database": func(b *RecoveryBackup, _ string) { b.Entries[0].Database = "customer_database" },
		"wrong-role":     func(b *RecoveryBackup, _ string) { b.Entries[0].Role = "customer_role" },
		"traversal":      func(b *RecoveryBackup, _ string) { b.Entries[0].Path = "../outside" },
		"unsupported":    func(b *RecoveryBackup, _ string) { b.Entries[0].Kind = "runtime-target" },
		"duplicate":      func(b *RecoveryBackup, _ string) { b.Entries = append(b.Entries, b.Entries[0]) },
		"corrupt": func(b *RecoveryBackup, dir string) {
			data := []byte(strings.Repeat("x", int(b.Entries[0].Size)))
			if err := os.WriteFile(filepath.Join(dir, b.Entries[0].Path), data, 0600); err != nil {
				t.Fatal(err)
			}
		},
		"loose-permissions": func(b *RecoveryBackup, dir string) {
			if err := os.Chmod(filepath.Join(dir, b.Entries[0].Path), 0644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, lock, r, b, dir := recoveryFixture(t)
			change(b, dir)
			if _, err := VerifyRecoveryBackup(context.Background(), cfg, lock, r, b, dir); err == nil {
				t.Fatal("incomplete/unsafe backup accepted")
			}
		})
	}
}

func TestRecoveryInputStrictDecodePermissionsAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"recovery-backup/v1","command":"restore everything"}`), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadRecoveryInput(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRecoveryBackup(data); err == nil {
		t.Fatal("arbitrary command field accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRecoveryInput(path); err == nil {
		t.Fatal("public recovery input accepted")
	}
	cfg, lock, r, b, dir := recoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyRecoveryBackup(ctx, cfg, lock, r, b, dir); err == nil {
		t.Fatal("canceled verification succeeded")
	}
	cfg.Spec.Mode = ModeExisting
	cfg.Spec.Cluster.Strategy = StrategyExisting
	if _, err := VerifyRecoveryBackup(context.Background(), cfg, lock, r, b, dir); err == nil {
		t.Fatal("field recovery accepted external cluster")
	}
}
