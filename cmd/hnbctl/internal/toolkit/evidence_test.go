package toolkit

import "testing"

func TestProfileCertifiedForBacking(t *testing.T) {
	m := &EvidenceMatrix{Profiles: []EvidenceProfile{
		{Name: "Minimal", Status: "certified", Backings: []string{"bundled"}},
		{Name: "LiteHA", Status: "blocked", Backings: []string{"in-cluster-ha", "external"}},
		{Name: "StandardHA", Status: "certified"}, // no backing set: matches any
	}}
	if !m.ProfileCertifiedFor(ProfileMinimal, "bundled") {
		t.Fatal("bundled Minimal must be certified")
	}
	if m.ProfileCertifiedFor(ProfileMinimal, "external") {
		t.Fatal("external Minimal must not be certified")
	}
	if m.ProfileCertifiedFor(ProfileLiteHA, "in-cluster-ha") {
		t.Fatal("blocked LiteHA must not be certified for any backing")
	}
	if !m.ProfileCertifiedFor(ProfileStandardHA, "external") || !m.ProfileCertifiedFor(ProfileStandardHA, "") {
		t.Fatal("no-backing-set entry must match any backing")
	}
}

func TestUpgradePathCertifiedForBacking(t *testing.T) {
	m := &EvidenceMatrix{ProfilePaths: []EvidencePath{
		{From: "Minimal", To: "LiteHA", Status: "certified", Backings: []string{"external"}},
		{From: "LiteHA", To: "StandardHA", Status: "blocked", Backings: []string{"in-cluster-ha"}},
	}}
	if !m.UpgradePathCertifiedFor(ProfileMinimal, ProfileLiteHA, "external") {
		t.Fatal("external Minimal->LiteHA must be certified")
	}
	if m.UpgradePathCertifiedFor(ProfileMinimal, ProfileLiteHA, "in-cluster-ha") {
		t.Fatal("in-cluster-ha must not match an external-only path")
	}
	if m.UpgradePathCertifiedFor(ProfileLiteHA, ProfileStandardHA, "in-cluster-ha") {
		t.Fatal("blocked path must not be certified for its backing")
	}
	if m.UpgradePathCertifiedFor(ProfileMinimal, ProfileLiteHA, "") == false {
		t.Fatal("empty backing must match a certified path")
	}
	if m.UpgradePathCertifiedFor(ProfileMinimal, ProfileStandardHA, "") {
		t.Fatal("undeclared path must not be certified")
	}
}

func TestCurrentEvidenceDeclaresBlockedHAPaths(t *testing.T) {
	if CurrentEvidence.ProfileCertifiedFor(ProfileMinimal, "bundled") != true {
		t.Fatal("Minimal/bundled must be certified")
	}
	for _, p := range []Profile{ProfileLiteHA, ProfileStandardHA, ProfileEnterprise} {
		if CurrentEvidence.ProfileCertifiedFor(p, "in-cluster-ha") {
			t.Fatalf("%s must remain uncertified", p)
		}
	}
	if CurrentEvidence.UpgradePathCertifiedFor(ProfileMinimal, ProfileLiteHA, "in-cluster-ha") {
		t.Fatal("Minimal->LiteHA in-cluster-ha must remain blocked")
	}
	if len(CurrentEvidence.ProfilePaths) == 0 {
		t.Fatal("HA upgrade paths must be declared (even if blocked)")
	}
}
