package toolkit

import (
	"strings"
	"testing"
)

func trainLock(extra string) string {
	return "schemaVersion: release-lock/v1\nreleaseId: fixture\nreleaseVersion: 0.1.0\n" +
		"images:\n  - name: portal\n    ref: registry.invalid/portal:v1\n    digest: sha256:" + strings.Repeat("a", 64) + "\n" +
		"compatibility:\n  os: [ubuntu-24.04]\n  arch: [amd64]\n  profiles: [Minimal, LiteHA, StandardHA, Enterprise]\n" + extra
}

func TestLockV1WithoutTrainFieldsRemainsReadable(t *testing.T) {
	lock, err := DecodeReleaseLock([]byte(trainLock("")))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.Validate(); len(problems) != 0 {
		t.Fatalf("v1 lock rejected: %v", problems)
	}
	if problems := lock.ValidateUpgrade(ProfileMinimal, ProfileLiteHA); len(problems) == 0 || !strings.Contains(problems[0], "UPGRADE_PATH_UNDECLARED") {
		t.Fatalf("undeclared upgrade must be refused: %v", problems)
	}
}

func TestLockV2TrainFieldsValidate(t *testing.T) {
	train := "profiles:\n  - name: Minimal\n    requiredCapabilities: [core, portal, market]\n    minKubernetesVersion: 1.30.0\n" +
		"  - name: LiteHA\n    requiredCapabilities: [core, portal, market, routing]\n" +
		"supportedUpgrades:\n  - {from: Minimal, to: LiteHA, backingChange: true}\n  - {from: LiteHA, to: StandardHA}\n" +
		"migrationChain:\n  - {owner: platform, name: 001-init, fromSchema: \"0\", toSchema: \"1\", checksum: " + strings.Repeat("b", 64) + "}\n" +
		"  - {owner: platform, name: 002-add, fromSchema: \"1\", toSchema: \"2\", checksum: " + strings.Repeat("c", 64) + "}\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.Validate(); len(problems) != 0 {
		t.Fatalf("valid train lock rejected: %v", problems)
	}
	if problems := lock.ValidateUpgrade(ProfileMinimal, ProfileLiteHA); len(problems) != 0 {
		t.Fatalf("declared contiguous upgrade rejected: %v", problems)
	}
}

func TestValidateUpgradeRejectsUndeclaredAndChainGap(t *testing.T) {
	train := "supportedUpgrades:\n  - {from: Minimal, to: LiteHA}\n" +
		"migrationChain:\n  - {owner: market, name: a, fromSchema: \"0\", toSchema: \"1\", checksum: " + strings.Repeat("d", 64) + "}\n" +
		"  - {owner: market, name: b, fromSchema: \"2\", toSchema: \"3\", checksum: " + strings.Repeat("e", 64) + "}\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.ValidateUpgrade(ProfileLiteHA, ProfileStandardHA); len(problems) == 0 || !strings.Contains(strings.Join(problems, ";"), "UPGRADE_PATH_UNDECLARED") {
		t.Fatalf("undeclared path not refused: %v", problems)
	}
	if problems := lock.ValidateUpgrade(ProfileMinimal, ProfileLiteHA); len(problems) == 0 || !strings.Contains(strings.Join(problems, ";"), "MIGRATION_CHAIN_GAP") {
		t.Fatalf("chain gap not detected: %v", problems)
	}
}

func TestLockV2RejectsBadMigrationChecksum(t *testing.T) {
	train := "supportedUpgrades:\n  - {from: Minimal, to: LiteHA}\n" +
		"migrationChain:\n  - {owner: platform, name: bad, fromSchema: \"0\", toSchema: \"1\", checksum: deadbeef}\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.Validate(); len(problems) == 0 {
		t.Fatal("bad migration checksum accepted")
	}
}

func TestCheckProfileContract(t *testing.T) {
	train := "profiles:\n  - name: Minimal\n    requiredCapabilities: [core, portal]\n    minKubernetesVersion: 1.30.0\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.CheckProfileContract(ProfileMinimal, "1.28.0"); len(problems) == 0 {
		t.Fatal("below-minimum kubernetes version accepted")
	}
	if problems := lock.CheckProfileContract(ProfileMinimal, "1.36.5+k3s1"); len(problems) != 0 {
		t.Fatalf("satisfied contract rejected: %v", problems)
	}
}

func TestCompareDottedVersions(t *testing.T) {
	if compareDottedVersions("1.36.5+k3s1", "1.30.0") <= 0 {
		t.Fatal("1.36 should exceed 1.30")
	}
	if compareDottedVersions("1.30", "1.30.0") != 0 {
		t.Fatal("1.30 should equal 1.30.0")
	}
}

func TestValidateUpgradeRequiresBackingChange(t *testing.T) {
	undeclared := "supportedUpgrades:\n  - {from: Minimal, to: LiteHA}\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(undeclared)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.ValidateUpgrade(ProfileMinimal, ProfileLiteHA); !strings.Contains(strings.Join(problems, ";"), "BACKING_CHANGE_NOT_SUPPORTED") {
		t.Fatalf("backing change without a declaration must be refused: %v", problems)
	}
	declared := "supportedUpgrades:\n  - {from: Minimal, to: LiteHA, backingChange: true}\n"
	lock, err = DecodeReleaseLock([]byte(trainLock(declared)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.ValidateUpgrade(ProfileMinimal, ProfileLiteHA); len(problems) != 0 {
		t.Fatalf("declared backing change refused: %v", problems)
	}
	// Same-kind HA transition needs no backingChange.
	sameKind := "supportedUpgrades:\n  - {from: LiteHA, to: StandardHA}\n"
	lock, err = DecodeReleaseLock([]byte(trainLock(sameKind)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.ValidateUpgrade(ProfileLiteHA, ProfileStandardHA); len(problems) != 0 {
		t.Fatalf("same-backing HA transition refused: %v", problems)
	}
}

func TestLockRejectsUnknownBacking(t *testing.T) {
	train := "profiles:\n  - name: LiteHA\n    backings: [in-cluster-ha, on-prem]\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.Validate(); !strings.Contains(strings.Join(problems, ";"), "unknown backing") {
		t.Fatalf("unknown backing accepted: %v", problems)
	}
}

func TestCheckProfileContractRejectsDisallowedBacking(t *testing.T) {
	train := "profiles:\n  - name: LiteHA\n    backings: [external]\n"
	lock, err := DecodeReleaseLock([]byte(trainLock(train)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.CheckProfileContract(ProfileLiteHA, ""); !strings.Contains(strings.Join(problems, ";"), "does not allow backing in-cluster-ha") {
		t.Fatalf("disallowed backing accepted: %v", problems)
	}
	allowed := "profiles:\n  - name: LiteHA\n    backings: [in-cluster-ha, external]\n"
	lock, err = DecodeReleaseLock([]byte(trainLock(allowed)))
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.CheckProfileContract(ProfileLiteHA, ""); len(problems) != 0 {
		t.Fatalf("allowed backing refused: %v", problems)
	}
}
