package market

import (
	"testing"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestCanPromote(t *testing.T) {
	tests := []struct {
		from, to ChannelType
		want     bool
	}{
		{ChDev, ChStaging, true},
		{ChStaging, ChStable, true},
		{ChStaging, ChDev, true},
		{ChStable, ChDeprecated, true},
		{ChStable, ChStaging, true},
		{ChDeprecated, ChWithdrawn, true},
		{ChDeprecated, ChStable, true},
		{ChDev, ChStable, false},
		{ChDev, ChWithdrawn, false},
		{ChWithdrawn, ChDev, false},
		{ChWithdrawn, ChStable, false},
		{ChStable, ChDev, false},
	}
	for _, tt := range tests {
		got := CanPromote(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("CanPromote(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestReleaseManager_CreateRelease(t *testing.T) {
	rm := NewReleaseManager()
	manifest := &ReleaseManifest{
		ReleaseID: "rel-1",
		ProductID: "prod-1",
		Version:   "1.0.0",
		Packages:  []PackageRef{{Name: "app", PackageType: "helm"}},
		Artifacts: []ArtifactRef{{Name: "app-image", Digest: digestA, Registry: "registry.example.com"}},
	}

	release, err := rm.CreateRelease("prod-1", "1.0.0", "Initial release", "alice", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.0.0" {
		t.Errorf("version = %s, want 1.0.0", release.Version)
	}
	if release.Status != "draft" {
		t.Errorf("status = %s, want draft", release.Status)
	}
	if release.ManifestDigest == "" {
		t.Error("manifest_digest should be set")
	}
}

func TestReleaseManager_CreateRelease_MissingVersion(t *testing.T) {
	rm := NewReleaseManager()
	_, err := rm.CreateRelease("prod-1", "", "notes", "alice", &ReleaseManifest{ReleaseID: "rel-1"})
	if err == nil {
		t.Error("expected error for empty version")
	}
}

func TestReleaseManager_PublishRelease(t *testing.T) {
	rm := NewReleaseManager()
	manifest := &ReleaseManifest{
		ReleaseID: "rel-1", ProductID: "prod-1", Version: "1.0.0",
		Packages: []PackageRef{{Name: "app", PackageType: "helm"}},
	}
	release, err := rm.CreateRelease("prod-1", "1.0.0", "", "alice", manifest)
	if err != nil {
		t.Fatal(err)
	}

	if err := rm.PublishRelease(release); err != nil {
		t.Fatal(err)
	}
	if release.Status != "published" {
		t.Errorf("status = %s, want published", release.Status)
	}
	if release.PublishedAt == nil {
		t.Error("published_at should be set")
	}
}

func TestReleaseManager_PublishNonDraft(t *testing.T) {
	rm := NewReleaseManager()
	release := &Release{Status: "published"}
	if err := rm.PublishRelease(release); err == nil {
		t.Error("expected error for non-draft publish")
	}
}

func TestValidateManifest(t *testing.T) {
	rm := NewReleaseManager()

	err := rm.ValidateManifest(&ReleaseManifest{ReleaseID: "", Version: "1.0", Packages: []PackageRef{{Name: "app"}}})
	if err == nil {
		t.Error("expected error for empty release_id")
	}

	err = rm.ValidateManifest(&ReleaseManifest{ReleaseID: "rel-1", Version: ""})
	if err == nil {
		t.Error("expected error for empty version")
	}

	err = rm.ValidateManifest(&ReleaseManifest{ReleaseID: "rel-1", Version: "1.0"})
	if err == nil {
		t.Error("expected error for empty packages")
	}

	err = rm.ValidateManifest(&ReleaseManifest{
		ReleaseID: "rel-1", Version: "1.0",
		Artifacts: []ArtifactRef{{Name: "img", Digest: ""}},
	})
	if err == nil {
		t.Error("expected error for empty artifact digest")
	}
}

func TestReleaseManager_ValidatePass(t *testing.T) {
	rm := NewReleaseManager()
	manifest := &ReleaseManifest{
		ReleaseID: "rel-1", Version: "1.0",
		Packages:  []PackageRef{{Name: "app", PackageType: "helm"}},
		Artifacts: []ArtifactRef{{Name: "img", Digest: digestA, Registry: "reg.io"}},
	}
	if err := rm.ValidateManifest(manifest); err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestChannelPipeline_Promote(t *testing.T) {
	cp := NewChannelPipeline()
	cp.AddChannel(&Channel{
		ID: "ch-dev", ProductID: "prod-1",
		ChannelType: ChDev, PromotionOrder: 1,
	})

	newCh, err := cp.Promote("ch-dev", ChStaging, "rel-1")
	if err != nil {
		t.Fatal(err)
	}
	if newCh.ChannelType != ChStaging {
		t.Errorf("type = %s, want staging", newCh.ChannelType)
	}
	if newCh.ReleaseID != "rel-1" {
		t.Errorf("release_id = %s, want rel-1", newCh.ReleaseID)
	}
}

func TestChannelPipeline_PromoteInvalid(t *testing.T) {
	cp := NewChannelPipeline()
	cp.AddChannel(&Channel{
		ID: "ch-dev", ProductID: "prod-1",
		ChannelType: ChDev, PromotionOrder: 1,
	})

	_, err := cp.Promote("ch-dev", ChStable, "rel-1")
	if err == nil {
		t.Error("expected error for dev→stable promotion")
	}
}

func TestChannelPipeline_GetReleaseForChannel(t *testing.T) {
	cp := NewChannelPipeline()
	cp.AddChannel(&Channel{
		ID: "ch-stable", ProductID: "prod-1",
		ChannelType: ChStable, ReleaseID: "rel-1",
	})

	relID, ok := cp.GetReleaseForChannel("prod-1", ChStable)
	if !ok || relID != "rel-1" {
		t.Errorf("got release %s (ok=%v)", relID, ok)
	}
}

func TestEntitlementChecker_CheckAccess(t *testing.T) {
	ec := NewEntitlementChecker()
	ec.AddEntitlement(&Entitlement{
		ID: "ent-1", ProductID: "prod-1", TenantID: "tenant-1",
		EntitlementType: EntStandard, IsActive: true,
	})
	ec.AddSubscription(&Subscription{
		ID: "sub-1", TenantID: "tenant-1", ProductID: "prod-1",
		EntitlementID: "ent-1", Status: "active",
	})

	result := ec.CheckAccess("tenant-1", "prod-1")
	if !result.Allowed {
		t.Errorf("expected allowed, got: %s", result.Reason)
	}
}

func TestEntitlementChecker_CheckAccess_NoSub(t *testing.T) {
	ec := NewEntitlementChecker()
	result := ec.CheckAccess("tenant-2", "prod-1")
	if result.Allowed {
		t.Error("expected not allowed for unsubscribed tenant")
	}
}

func TestEntitlementChecker_CheckAccess_InactiveSub(t *testing.T) {
	ec := NewEntitlementChecker()
	ec.AddSubscription(&Subscription{
		ID: "sub-1", TenantID: "tenant-1", ProductID: "prod-1",
		EntitlementID: "ent-1", Status: "cancelled",
	})
	result := ec.CheckAccess("tenant-1", "prod-1")
	if result.Allowed {
		t.Error("expected not allowed for cancelled subscription")
	}
}

func TestEntitlementChecker_CheckDeploymentLimit(t *testing.T) {
	ec := NewEntitlementChecker()
	ec.AddEntitlement(&Entitlement{
		ID: "ent-1", ProductID: "prod-1", TenantID: "tenant-1",
		EntitlementType: EntStandard, MaxDeployments: 3, IsActive: true,
	})
	ec.AddSubscription(&Subscription{
		ID: "sub-1", TenantID: "tenant-1", ProductID: "prod-1",
		EntitlementID: "ent-1", Status: "active",
	})

	result := ec.CheckDeploymentLimit("tenant-1", "prod-1", 2)
	if !result.Allowed {
		t.Errorf("expected allowed for 2/3 deployments, got: %s", result.Reason)
	}

	result = ec.CheckDeploymentLimit("tenant-1", "prod-1", 3)
	if result.Allowed {
		t.Error("expected blocked at max deployments")
	}
}

func TestManifestBridge_ToExecutionPlan(t *testing.T) {
	mb := NewManifestBridge()
	manifest := &ReleaseManifest{
		ReleaseID: "rel-1",
		ProductID: "prod-1",
		Version:   "2.0.0",
		Packages: []PackageRef{
			{Name: "postgresql", PackageType: "helm"},
			{Name: "app-backend", PackageType: "container"},
		},
		Artifacts: []ArtifactRef{
			{Name: "pg-image", Digest: digestB, Registry: "reg.io/pg"},
			{Name: "app-image", Digest: digestA, Registry: "reg.io/app"},
		},
		Dependencies: []DependencySpec{
			{ProductID: "postgresql", Version: "14", Required: true},
		},
	}

	plan, err := mb.ToExecutionPlan(manifest, &PolicyResult{Passed: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanDigest == "" {
		t.Error("plan digest should be set")
	}
	if len(plan.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(plan.Steps))
	}
	if len(plan.ArtifactDigests) != 2 || plan.ArtifactDigests[0] != digestA || plan.ArtifactDigests[1] != digestB {
		t.Fatalf("artifact digests not sorted and pinned: %#v", plan.ArtifactDigests)
	}
}

func TestManifestBridgePlanDigestIgnoresTagMovement(t *testing.T) {
	mb := NewManifestBridge()
	base := &ReleaseManifest{
		ReleaseID: "rel-1", ProductID: "prod-1", Version: "2.0.0",
		Packages:  []PackageRef{{Name: "app", PackageType: "container"}},
		Artifacts: []ArtifactRef{{Name: "app-image", Digest: digestA, Registry: "reg.io/app:stable"}},
	}
	movedTag := &ReleaseManifest{
		ReleaseID: "rel-1", ProductID: "prod-1", Version: "2.0.0",
		Packages:  []PackageRef{{Name: "app", PackageType: "container"}},
		Artifacts: []ArtifactRef{{Name: "app-image", Digest: digestA, Registry: "reg.io/app:latest"}},
	}
	newDigest := &ReleaseManifest{
		ReleaseID: "rel-1", ProductID: "prod-1", Version: "2.0.0",
		Packages:  []PackageRef{{Name: "app", PackageType: "container"}},
		Artifacts: []ArtifactRef{{Name: "app-image", Digest: digestB, Registry: "reg.io/app:latest"}},
	}

	basePlan, err := mb.ToExecutionPlan(base, &PolicyResult{Passed: true})
	if err != nil {
		t.Fatal(err)
	}
	movedTagPlan, err := mb.ToExecutionPlan(movedTag, &PolicyResult{Passed: true})
	if err != nil {
		t.Fatal(err)
	}
	newDigestPlan, err := mb.ToExecutionPlan(newDigest, &PolicyResult{Passed: true})
	if err != nil {
		t.Fatal(err)
	}
	if basePlan.PlanDigest != movedTagPlan.PlanDigest {
		t.Fatalf("tag movement changed plan digest: %s != %s", basePlan.PlanDigest, movedTagPlan.PlanDigest)
	}
	if basePlan.PlanDigest == newDigestPlan.PlanDigest {
		t.Fatal("new artifact digest did not change plan digest")
	}
}

func TestManifestBridge_ToExecutionPlan_EmptyManifest(t *testing.T) {
	mb := NewManifestBridge()
	_, err := mb.ToExecutionPlan(&ReleaseManifest{}, &PolicyResult{Passed: true})
	if err == nil {
		t.Error("expected error for empty manifest")
	}
}

func TestManifestBridge_ToExecutionPlan_PolicyFail(t *testing.T) {
	mb := NewManifestBridge()
	manifest := &ReleaseManifest{ReleaseID: "rel-1", Version: "1.0", Packages: []PackageRef{{Name: "app", PackageType: "helm"}}}
	_, err := mb.ToExecutionPlan(manifest, &PolicyResult{Passed: false})
	if err == nil {
		t.Error("expected error for failing policy result")
	}
}

func TestChannelOrder(t *testing.T) {
	if channelOrder[ChDev] != 1 {
		t.Errorf("dev order = %d, want 1", channelOrder[ChDev])
	}
	if channelOrder[ChWithdrawn] != 5 {
		t.Errorf("withdrawn order = %d, want 5", channelOrder[ChWithdrawn])
	}
}

func TestProductCategory(t *testing.T) {
	categories := []ProductCategory{CatApplication, CatDatabase, CatMiddleware, CatAI, CatEdge, CatTool, CatOther}
	if len(categories) != 7 {
		t.Errorf("expected 7 categories, got %d", len(categories))
	}
}

func TestPublisherStatus(t *testing.T) {
	statuses := []PublisherStatus{PubActive, PubSuspended, PubDecommissioned}
	if len(statuses) != 3 {
		t.Errorf("expected 3 statuses, got %d", len(statuses))
	}
}
