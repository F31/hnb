package toolkit

import (
	"strings"
	"testing"
)

func TestPlanScaleReplicasUp(t *testing.T) {
	lite := ProfileShapeData()[ProfileLiteHA]
	plan, err := PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"apiserver": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].Kind != "components" {
		t.Fatalf("scale up not authorized: %+v", plan)
	}
}

func TestPlanScaleBelowQuorumRefused(t *testing.T) {
	standard := ProfileShapeData()[ProfileStandardHA]
	plan, err := PlanScale(ScaleInput{Shape: standard, Replicas: map[string]int{"apiserver": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_BELOW_QUORUM") {
		t.Fatalf("below-quorum scale not refused: %+v", plan)
	}
}

func TestPlanScaleStorageShrinkRefusedAndExpandAllowed(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	shrink, err := PlanScale(ScaleInput{Shape: minimal, StorageGi: map[string]int{"postgresql": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if shrink.Authorized || !strings.Contains(strings.Join(shrink.Blocked, ";"), "STORAGE_SHRINK_REFUSED") {
		t.Fatalf("storage shrink not refused: %+v", shrink)
	}
	grow, err := PlanScale(ScaleInput{Shape: minimal, StorageGi: map[string]int{"postgresql": 4}})
	if err != nil {
		t.Fatal(err)
	}
	if !grow.Authorized || grow.Steps[0].Reversible {
		t.Fatalf("storage expansion should be authorized and irreversible: %+v", grow)
	}
}

func TestPlanScaleInFlightUpgradeBlocked(t *testing.T) {
	lite := ProfileShapeData()[ProfileLiteHA]
	plan, err := PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"apiserver": 5}, InFlightUpgrade: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_UPGRADE_IN_FLIGHT") {
		t.Fatalf("in-flight upgrade should block scaling: %+v", plan)
	}
}

func TestPlanScaleUnknownComponent(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err := PlanScale(ScaleInput{Shape: minimal, Replicas: map[string]int{"not-a-component": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_UNKNOWN_COMPONENT") {
		t.Fatalf("unknown component not refused: %+v", plan)
	}
}

func TestUpgradePathEvidenceGate(t *testing.T) {
	m := certifiedProfiles(ProfileMinimal, ProfileLiteHA)
	if m.UpgradePathCertified(ProfileMinimal, ProfileLiteHA) {
		t.Fatal("no path declared yet must not be certified")
	}
	m.ProfilePaths = []EvidencePath{{From: "Minimal", To: "LiteHA", Status: "certified"}}
	if !m.UpgradePathCertified(ProfileMinimal, ProfileLiteHA) {
		t.Fatal("declared certified path not recognised")
	}
	if m.UpgradePathCertified(ProfileLiteHA, ProfileStandardHA) {
		t.Fatal("undeclared path must not be certified")
	}
}

// TestPlanScaleRefusesExternalOwners proves existing-route scaling can only touch
// HNB-managed components: an external CNI/owner component is not in a profile's
// component set and is refused rather than adopted.
func TestPlanScaleRefusesExternalOwners(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	for _, external := range []string{"cilium", "kube-proxy", "coredns", "calico", "kube-ovn"} {
		plan, err := PlanScale(ScaleInput{Shape: minimal, Replicas: map[string]int{external: 2}})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_UNKNOWN_COMPONENT") {
			t.Fatalf("external owner %s was not refused: %+v", external, plan)
		}
	}
}

func TestPlanScaleUsesObservedBaseline(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err := PlanScale(ScaleInput{Shape: minimal, Replicas: map[string]int{"apiserver": 1}, Observed: map[string]int{"apiserver": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].From != "3" || plan.Steps[0].To != "1" {
		t.Fatalf("observed baseline not used for scale-down: %+v", plan)
	}
}

func TestPlanScaleUsesObservedStorageBaseline(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err := PlanScale(ScaleInput{Shape: minimal, StorageGi: map[string]int{"nats": 1}, ObservedStorage: map[string]int{"nats": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "STORAGE_SHRINK_REFUSED") {
		t.Fatalf("observed storage shrink not refused: %+v", plan)
	}
}

func TestPlanScaleVerticalResources(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err := PlanScale(ScaleInput{Shape: minimal, Resources: map[string]ResourceProfile{"portal": {Memory: "128Mi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].Kind != "resources" || plan.Steps[0].To != "25m,128Mi" {
		t.Fatalf("vertical scale plan wrong: %+v", plan)
	}
	bad, err := PlanScale(ScaleInput{Shape: minimal, Resources: map[string]ResourceProfile{"not-a-component": {CPU: "100m"}}})
	if err != nil {
		t.Fatal(err)
	}
	if bad.Authorized || !strings.Contains(strings.Join(bad.Blocked, ";"), "SCALE_UNKNOWN_COMPONENT") {
		t.Fatalf("unknown component resources accepted: %+v", bad)
	}
}

func TestPlanScaleUsesObservedResourceBaseline(t *testing.T) {
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err := PlanScale(ScaleInput{Shape: minimal, Resources: map[string]ResourceProfile{"portal": {Memory: "64Mi"}}, ObservedResources: map[string]ResourceProfile{"portal": {CPU: "25m", Memory: "96Mi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].From != "25m,96Mi" || plan.Steps[0].To != "25m,64Mi" {
		t.Fatalf("observed resource baseline not used: %+v", plan)
	}
}

func TestPlanScaleBlocksInClusterHAPreconditions(t *testing.T) {
	lite := ProfileShapeData()[ProfileLiteHA]
	plan, err := PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"apiserver": 5}, ControlPlaneDomains: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Blocked, ";"), "HA_CONTROL_NODES_INSUFFICIENT") {
		t.Fatalf("insufficient control domains must block scale: %v", plan.Blocked)
	}
	notReady := false
	plan, err = PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"apiserver": 5}, ControlPlaneDomains: 3, OperatorReady: &notReady})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Blocked, ";"), "OPERATOR_NOT_INSTALLED") {
		t.Fatalf("missing operator must block scale: %v", plan.Blocked)
	}
}

func TestPlanScaleDataPlaneReplicas(t *testing.T) {
	lite := ProfileShapeData()[ProfileLiteHA]
	plan, err := PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"nats": 5}, Observed: map[string]int{"nats": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Steps) != 1 || plan.Steps[0].Target != "nats" ||
		!strings.Contains(strings.Join(plan.Steps[0].Requires, ";"), "StatefulSet.spec.replicas") {
		t.Fatalf("nats data-plane scale not planned/mapped: %+v", plan)
	}
	plan, err = PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"postgresql": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || plan.Steps[0].From != "3" || plan.Steps[0].To != "5" ||
		!strings.Contains(strings.Join(plan.Steps[0].Requires, ";"), "Cluster.spec.instances") {
		t.Fatalf("postgresql data-plane scale not planned/mapped: %+v", plan)
	}
	plan, err = PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"nats": 4}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_EVEN_QUORUM") {
		t.Fatalf("even JetStream replica accepted: %v", plan.Blocked)
	}
	plan, err = PlanScale(ScaleInput{Shape: lite, Replicas: map[string]int{"postgresql": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_BELOW_QUORUM") {
		t.Fatalf("below-quorum data plane accepted: %v", plan.Blocked)
	}
	minimal := ProfileShapeData()[ProfileMinimal]
	plan, err = PlanScale(ScaleInput{Shape: minimal, Replicas: map[string]int{"nats": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Authorized || !strings.Contains(strings.Join(plan.Blocked, ";"), "SCALE_UNKNOWN_COMPONENT") {
		t.Fatalf("bundled data-plane scale accepted: %v", plan.Blocked)
	}
}
