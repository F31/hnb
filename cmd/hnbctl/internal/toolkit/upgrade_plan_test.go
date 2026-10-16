package toolkit

import (
	"strings"
	"testing"
)

func certifiedProfiles(profiles ...Profile) *EvidenceMatrix {
	m := &EvidenceMatrix{}
	for _, p := range profiles {
		m.Profiles = append(m.Profiles, EvidenceProfile{Name: string(p), Status: "certified"})
	}
	return m
}

func trainLockForUpgrade() *ReleaseLock {
	return &ReleaseLock{
		SchemaVersion: LockSchemaVersion, ReleaseID: "fixture", ReleaseVersion: "0.1.0",
		SupportedUpgrades: []LockUpgradePath{{From: "Minimal", To: "LiteHA"}, {From: "LiteHA", To: "StandardHA"}},
		MigrationChain: []MigrationStep{
			{Owner: "platform", Name: "001", FromSchema: "0", ToSchema: "1", Checksum: strings.Repeat("a", 64), Reversible: true},
			{Owner: "platform", Name: "002", FromSchema: "1", ToSchema: "2", Checksum: strings.Repeat("b", 64), Reversible: false},
		},
	}
}

func TestPlanUpgradeMinimalToLiteAuthorized(t *testing.T) {
	plan, err := PlanUpgrade(UpgradeInput{
		From: ProfileMinimal, To: ProfileLiteHA,
		Lock: trainLockForUpgrade(), Evidence: certifiedProfiles(ProfileMinimal, ProfileLiteHA),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized {
		t.Fatalf("authorized upgrade blocked: %v", plan.Blocked)
	}
	kinds := map[string]bool{}
	for _, s := range plan.Steps {
		kinds[s.Kind] = true
	}
	for _, want := range []string{"components", "backings", "migration", "release", "verify"} {
		if !kinds[want] {
			t.Fatalf("plan missing %s step: %+v", want, plan.Steps)
		}
	}
	if kinds["storage"] {
		t.Fatalf("externalising storage must not emit a storage step: %+v", plan.Steps)
	}
	// Irreversible migration must be marked as such.
	for _, s := range plan.Steps {
		if s.Kind == "migration" && s.Target == "platform/002" && s.Reversible {
			t.Fatal("irreversible migration marked reversible")
		}
	}
}

func TestPlanUpgradeBlockedWhenTargetUncertified(t *testing.T) {
	plan, err := PlanUpgrade(UpgradeInput{
		From: ProfileMinimal, To: ProfileLiteHA,
		Lock: trainLockForUpgrade(), Evidence: certifiedProfiles(ProfileMinimal),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "TARGET_PROFILE_NOT_CERTIFIED") {
		t.Fatalf("uncertified target not blocked: %v", plan.Blocked)
	}
}

func TestPlanUpgradeBlockedWhenPathUndeclared(t *testing.T) {
	lock := trainLockForUpgrade()
	lock.SupportedUpgrades = nil
	plan, err := PlanUpgrade(UpgradeInput{
		From: ProfileMinimal, To: ProfileLiteHA,
		Lock: lock, Evidence: certifiedProfiles(ProfileMinimal, ProfileLiteHA),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "UPGRADE_PATH_UNDECLARED") {
		t.Fatalf("undeclared path not blocked: %v", plan.Blocked)
	}
}

func TestPlanUpgradeDowngradeRequiresAck(t *testing.T) {
	plan, err := PlanUpgrade(UpgradeInput{From: ProfileStandardHA, To: ProfileMinimal})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "DOWNGRADE_NOT_ACKNOWLEDGED") {
		t.Fatalf("silent downgrade not blocked: %v", plan.Blocked)
	}
}

func TestPlanUpgradeUnknownProfile(t *testing.T) {
	if _, err := PlanUpgrade(UpgradeInput{From: Profile("DoesNotExist"), To: ProfileLiteHA}); err == nil {
		t.Fatal("unknown source profile accepted")
	}
}

func TestPlanUpgradeReleaseTrainStepsAreOrdered(t *testing.T) {
	plan, err := PlanUpgrade(UpgradeInput{
		From: ProfileMinimal, To: ProfileStandardHA,
		Lock:     &ReleaseLock{SupportedUpgrades: []LockUpgradePath{{From: "Minimal", To: "StandardHA"}}},
		Evidence: certifiedProfiles(ProfileStandardHA),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized {
		t.Fatalf("authorized upgrade blocked: %v", plan.Blocked)
	}
	if last := plan.Steps[len(plan.Steps)-1].Kind; last != "verify" {
		t.Fatalf("plan must end with verify, got %s", last)
	}
	if first := plan.Steps[0].Kind; first != "components" && first != "capabilities" {
		t.Fatalf("plan must start with a diff step, got %s", first)
	}
}
