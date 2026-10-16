package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

const recoveryValidConfig = `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: management
spec:
  mode: bootstrap
  profile: Minimal
  releaseLock: ./release.lock.yaml
  cluster:
    strategy: provision
    distribution: kubeadm
    kubernetesVersion: 1.36.5
    apiEndpoint: 10.0.0.10:6443
    nodes:
      - name: cp-1
        roles: [control-plane]
        sshAddress: 10.0.0.10
        internalAddress: 10.0.0.10
        user: ubuntu
        sshKeyFile: ~/.ssh/hnb_install
    network:
      plugin: cilium
      version: 1.20.1
      podCIDR: 10.244.0.0/16
      serviceCIDR: 10.96.0.0/12
      routingMode: native
  platform:
    namespace: hnb-system
    publicBaseURL: https://hnb.example.internal
    tls:
      existingSecret: fixture-tls
    storage:
      className: fixture-storage
    infrastructure:
      postgresql:
        mode: bundled
      nats:
        mode: bundled
      planes:
        - name: platform
          database: hnb_platform
          role: hnb_platform
          postgresqlSecretRef: platform-postgresql
          natsSecretRef: platform-nats
        - name: market
          database: hnb_market
          role: hnb_market
          postgresqlSecretRef: market-postgresql
          natsSecretRef: market-nats
        - name: artifact
          database: hnb_artifact
          role: hnb_artifact
          postgresqlSecretRef: artifact-postgresql
          natsSecretRef: artifact-nats
  artifacts:
    mode: online
  timeouts:
    download: 15m
    readiness: 10m
`

func sha256Prefix(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func recoveryLockFixture(releaseID, version string) *toolkit.ReleaseLock {
	image := toolkit.PinnedImage{Name: "apiserver", Ref: "registry.invalid/apiserver:v1", Digest: "sha256:" + strings.Repeat("a", 64)}
	return &toolkit.ReleaseLock{SchemaVersion: toolkit.LockSchemaVersion, ReleaseID: releaseID, ReleaseVersion: version,
		Images:        []toolkit.PinnedImage{image},
		Compatibility: toolkit.LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}, Profiles: []string{"Minimal"}}}
}

// recoveryCLIFixture writes a matched config/lock/receipt/backup set into a
// private temp directory and returns the CLI arguments required to verify it.
func recoveryCLIFixture(t *testing.T) (configPath, lockPath, receiptPath, manifestPath, backupDir string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(dir, "hnb.yaml")
	if err := os.WriteFile(configPath, []byte(recoveryValidConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := recoveryLockFixture("recovery-source", "1.0")
	lockData, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	lockPath = filepath.Join(dir, "release.lock.yaml")
	if err := os.WriteFile(lockPath, lockData, 0o600); err != nil {
		t.Fatal(err)
	}
	// Mirror readConfig + VerifyRecoveryBackup digest computation exactly.
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := toolkit.DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.Normalize(filepath.Dir(absConfig), home); err != nil {
		t.Fatal(err)
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	configDigest, lockDigest := sha256Prefix(normalized), sha256Prefix(lockData)
	start := time.Now().UTC().Add(-2 * time.Hour)
	receipt := &toolkit.BootstrapReceipt{
		SchemaVersion: "bootstrap-receipt/v1", RunID: "run-recovery",
		ConfigDigest: configDigest, LockDigest: lockDigest,
		ManagementClusterUID: "base-owned-uid", StartedAt: start,
		Ownership: []string{"namespace/" + decoded.Spec.Platform.Namespace},
		Phases:    []toolkit.PhaseOutcome{{Name: toolkit.PhaseInit, Status: "succeeded", StartedAt: start, FinishedAt: start.Add(time.Minute)}},
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	receiptPath = filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(receiptPath, receiptData, 0o600); err != nil {
		t.Fatal(err)
	}
	backup := &toolkit.RecoveryBackup{
		SchemaVersion: toolkit.RecoveryBackupSchema, BackupID: "backup-recovery",
		RunID: receipt.RunID, ConfigDigest: receipt.ConfigDigest, LockDigest: receipt.LockDigest,
		ManagementClusterUID: receipt.ManagementClusterUID, Namespace: decoded.Spec.Platform.Namespace,
		SnapshotAt: start.Add(time.Hour),
	}
	backupDir = filepath.Join(dir, "backup")
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	add := func(kind, plane, database, role string) {
		name := kind + "-" + plane + ".backup"
		data := []byte("synthetic-private-payload token=must-not-print-" + name)
		if err := os.WriteFile(filepath.Join(backupDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		backup.Entries = append(backup.Entries, toolkit.RecoveryBackupEntry{Kind: kind, Plane: plane, Database: database, Role: role, Path: name, Digest: sha256Prefix(data), Size: int64(len(data))})
	}
	for _, p := range decoded.Spec.Platform.Infrastructure.Planes {
		add("postgresql", p.Name, p.Database, p.Role)
	}
	for _, kind := range []string{"jetstream", "identity", "kms"} {
		add(kind, "", "", "")
	}
	manifestData, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath = filepath.Join(dir, "backup-manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, lockPath, receiptPath, manifestPath, backupDir
}

func TestRecoveryVerifyCLIIdentifiesLocalBackupInputs(t *testing.T) {
	configPath, lockPath, receiptPath, manifestPath, backupDir := recoveryCLIFixture(t)
	for _, format := range []string{"json", "human"} {
		code, out, errOut := runCLI(t, "recovery", "verify",
			"-f", configPath, "--lock", lockPath,
			"--receipt", receiptPath, "--backup-manifest", manifestPath,
			"--backup-dir", backupDir, "--format", format)
		if code != ExitSuccess {
			t.Fatalf("recovery verify [%s] failed: %d (%s %s)", format, code, out, errOut)
		}
		if format == "human" {
			if !strings.Contains(out, "scope=local-backup-inputs") || !strings.Contains(out, "inputsVerified=true") || !strings.Contains(out, "restoreAuthorized=false") {
				t.Fatalf("human output missing verified scope/facts: %s", out)
			}
			if !strings.Contains(out, "execution blocked: TARGET_IDENTITY_NOT_OBSERVED") || !strings.Contains(out, "execution blocked: RESTORE_EXECUTOR_UNAVAILABLE") {
				t.Fatalf("human output missing blockers: %s", out)
			}
		} else {
			var report toolkit.RecoveryInputReport
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatalf("invalid report JSON: %v (%s)", err, out)
			}
			if !report.InputsVerified || len(report.Checked) != 6 || report.RestoreAuthorized || report.Applied || report.TargetIdentityObserved || report.RTOVerified || report.RPOVerified {
				t.Fatalf("local checks promoted to restore/target facts: %+v", report)
			}
			if report.RunID != "run-recovery" || report.BackupID != "backup-recovery" {
				t.Fatalf("report lost run/backup identity: %+v", report)
			}
		}
		if strings.Contains(out+errOut, "must-not-print") {
			t.Fatalf("backup payload leaked in [%s] output", format)
		}
	}
}

func TestRecoveryVerifyCLIRefusesForeignLock(t *testing.T) {
	configPath, _, receiptPath, manifestPath, backupDir := recoveryCLIFixture(t)
	foreign := filepath.Join(t.TempDir(), "foreign.lock.yaml")
	data, err := json.Marshal(recoveryLockFixture("foreign-source", "9.9"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "recovery", "verify",
		"-f", configPath, "--lock", foreign,
		"--receipt", receiptPath, "--backup-manifest", manifestPath,
		"--backup-dir", backupDir, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("foreign lock accepted: %s %s", out, errOut)
	}
	if !strings.Contains(out+errOut, "INVALID_CONFIG") {
		t.Fatalf("expected structured invalid-config failure: %s %s", out, errOut)
	}
}

func TestRecoveryCLIRejectsExecutionSubcommand(t *testing.T) {
	code, out, errOut := runCLI(t, "recovery", "restore",
		"-f", "x.yaml", "--receipt", "r", "--backup-manifest", "b", "--backup-dir", "d")
	if code != ExitBlocked {
		t.Fatalf("recovery execution must be blocked, got %d (%s %s)", code, out, errOut)
	}
	if !strings.Contains(out+errOut, "recovery execution is unavailable") {
		t.Fatalf("unexpected execution message: %s %s", out, errOut)
	}
}

func TestRecoveryVerifyCLIRequiresAllInputs(t *testing.T) {
	configPath, _, receiptPath, manifestPath, backupDir := recoveryCLIFixture(t)
	for _, args := range [][]string{
		{"recovery", "verify", "-f", configPath, "--receipt", receiptPath, "--backup-manifest", manifestPath},
		{"recovery", "verify", "-f", configPath, "--receipt", receiptPath, "--backup-dir", backupDir},
		{"recovery", "verify", "-f", configPath},
	} {
		code, _, _ := runCLI(t, args...)
		if code != ExitUsage {
			t.Fatalf("incomplete recovery verify must be usage failure, got %d for %v", code, args)
		}
	}
}
