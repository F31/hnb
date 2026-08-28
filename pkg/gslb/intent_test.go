package gslb

import (
	"strings"
	"testing"
)

func uuid() string { return "00000000-0000-4000-8000-000000000001" }

func baseIntent(kind IntentKind) *Intent {
	return &Intent{
		APIVersion: APIVersion,
		Kind:       kind,
		ServiceID:  uuid(),
		TenantID:   "tenant-a",
		Metadata: IntentMetadata{
			IdempotencyKey: "gslb-failover-1",
			CorrelationID:  "00000000-0000-4000-8000-000000000002",
		},
	}
}

func TestParseIntentValid(t *testing.T) {
	body := []byte(`{
		"apiVersion":"gslb.hnb.io/v1",
		"kind":"gslb.failover",
		"serviceId":"00000000-0000-4000-8000-000000000001",
		"tenantId":"tenant-a",
		"targetPoolId":"00000000-0000-4000-8000-000000000003",
		"metadata":{"idempotencyKey":"k1","correlationId":"00000000-0000-4000-8000-000000000002"}
	}`)
	intent, err := ParseIntent(body)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Kind != IntentFailover {
		t.Fatalf("kind = %s", intent.Kind)
	}
	if !intent.RequiresApproval() {
		t.Fatal("failover must require approval by default")
	}
	if intent.SemanticDigest() == "" {
		t.Fatal("missing digest")
	}
	if string(intent.RawBody()) != string(body) {
		t.Fatal("rawBody not preserved")
	}
}

func TestParseIntentRejectsExecutableFields(t *testing.T) {
	// 携带 providerId / command 的意图必须 fail-closed（GSLB-005）
	body := []byte(`{
		"apiVersion":"gslb.hnb.io/v1",
		"kind":"gslb.failover",
		"serviceId":"00000000-0000-4000-8000-000000000001",
		"tenantId":"tenant-a",
		"targetPoolId":"00000000-0000-4000-8000-000000000003",
		"providerId":"dns-provider-x",
		"command":"apply",
		"metadata":{"idempotencyKey":"k1","correlationId":"00000000-0000-4000-8000-000000000002"}
	}`)
	if _, err := ParseIntent(body); err == nil {
		t.Fatal("intent with providerId/command must be rejected")
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	i := baseIntent(IntentKind("gslb.self-destruct"))
	if err := i.Validate(); err == nil {
		t.Fatal("unknown kind must be rejected")
	}
}

func TestValidateRejectsBadIdempotency(t *testing.T) {
	i := baseIntent(IntentFailover)
	i.TargetPoolID = uuid()
	i.Metadata.IdempotencyKey = ""
	if err := i.Validate(); err == nil {
		t.Fatal("empty idempotencyKey must be rejected")
	}
}

func TestValidateFailoverRequiresTargetPool(t *testing.T) {
	i := baseIntent(IntentFailover)
	if err := i.Validate(); err == nil {
		t.Fatal("failover without targetPoolId must be rejected")
	}
}

func TestValidateWeightUpdate(t *testing.T) {
	i := baseIntent(IntentWeightUpdate)
	i.Weights = map[string]int{"cluster-a": 70, "cluster-b": 30}
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	if i.RequiresApproval() {
		t.Fatal("weight-update must not require approval by default")
	}
	// 权重总和为 0 拒绝
	i.Weights = map[string]int{"cluster-a": 0}
	if err := i.Validate(); err == nil {
		t.Fatal("zero total weight must be rejected")
	}
	// 越界权重拒绝
	i.Weights = map[string]int{"cluster-a": 101}
	if err := i.Validate(); err == nil {
		t.Fatal("weight > 100 must be rejected")
	}
}

func TestDrillIsReadOnly(t *testing.T) {
	i := baseIntent(IntentDrill)
	i.TargetPoolID = uuid()
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	if !i.IsDrill() || i.IsExecutable() {
		t.Fatal("drill must be read-only and non-executable")
	}
}

func TestSemanticDigestDeterministic(t *testing.T) {
	a := baseIntent(IntentWeightUpdate)
	a.Weights = map[string]int{"cluster-b": 30, "cluster-a": 70}
	b := baseIntent(IntentWeightUpdate)
	b.Weights = map[string]int{"cluster-a": 70, "cluster-b": 30}
	if a.SemanticDigest() != b.SemanticDigest() {
		t.Fatal("digest must be key-order independent")
	}
	c := baseIntent(IntentWeightUpdate)
	c.Weights = map[string]int{"cluster-a": 60, "cluster-b": 40}
	if a.SemanticDigest() == c.SemanticDigest() {
		t.Fatal("digest must differ for different weights")
	}
}

func TestBuildPlanFailoverDAG(t *testing.T) {
	i := baseIntent(IntentFailover)
	i.TargetPoolID = "00000000-0000-4000-8000-000000000003"
	plan, err := i.BuildPlan(PlanInput{
		ServiceID:       i.ServiceID,
		TenantID:        i.TenantID,
		Domain:          "app.hnb.cloud",
		TargetPoolID:    i.TargetPoolID,
		Targets:         []string{"10.0.1.10", "10.0.2.10"},
		PreviousTargets: []string{"10.0.0.10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	if plan.Steps[0].StepType != StepDNSApply || plan.Steps[0].Compensation != "revert" {
		t.Fatalf("apply step: %+v", plan.Steps[0])
	}
	if plan.Steps[1].StepType != StepDNSVerify || len(plan.Steps[1].DependsOn) != 1 {
		t.Fatalf("verify step: %+v", plan.Steps[1])
	}
	if plan.Steps[2].StepType != StepDNSRevert {
		t.Fatalf("revert step: %+v", plan.Steps[2])
	}
	// 计划确定性摘要
	d1 := plan.CanonicalDigest()
	d2, _ := i.BuildPlan(PlanInput{
		ServiceID: i.ServiceID, TenantID: i.TenantID, Domain: "app.hnb.cloud",
		TargetPoolID: i.TargetPoolID, Targets: []string{"10.0.1.10", "10.0.2.10"},
		PreviousTargets: []string{"10.0.0.10"},
	})
	if d1 != d2.CanonicalDigest() {
		t.Fatal("plan canonical digest must be deterministic")
	}
}

func TestBuildPlanDrillHasNoExecutableSteps(t *testing.T) {
	i := baseIntent(IntentDrill)
	i.TargetPoolID = "00000000-0000-4000-8000-000000000003"
	plan, err := i.BuildPlan(PlanInput{
		ServiceID: i.ServiceID, TenantID: i.TenantID, Domain: "app.hnb.cloud",
		TargetPoolID: i.TargetPoolID, Targets: []string{"10.0.1.10"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].StepType != StepDrillCompute {
		t.Fatalf("drill plan: %+v", plan.Steps)
	}
	if strings.Contains(string(plan.Steps[0].StepType), "dns_apply") {
		t.Fatal("drill must not contain dns_apply")
	}
}

func TestBuildPlanRejectsMismatchedService(t *testing.T) {
	i := baseIntent(IntentFailover)
	i.TargetPoolID = "00000000-0000-4000-8000-000000000003"
	_, err := i.BuildPlan(PlanInput{
		ServiceID: "00000000-0000-4000-8000-000000000099", TenantID: i.TenantID,
		Domain: "app.hnb.cloud", TargetPoolID: i.TargetPoolID, Targets: []string{"10.0.1.10"},
	})
	if err == nil {
		t.Fatal("plan with mismatched serviceId must be rejected")
	}
}

func TestDRGroupRefValidation(t *testing.T) {
	intent := baseIntent(IntentFailover)
	intent.TargetPoolID = "00000000-0000-4000-8000-000000000003"
	intent.DRGroupRef = "dr-group-region-east"
	if err := intent.Validate(); err != nil {
		t.Fatalf("valid drGroupRef rejected: %v", err)
	}
	for _, bad := range []string{"has space", "with/slash", "-leading-dash", strings.Repeat("x", 129)} {
		intent.DRGroupRef = bad
		if err := intent.Validate(); err == nil {
			t.Fatalf("drGroupRef %q must be rejected", bad)
		}
	}
}

func TestDRGroupRefAffectsDigest(t *testing.T) {
	a := baseIntent(IntentFailover)
	a.TargetPoolID = "00000000-0000-4000-8000-000000000003"
	b := *a
	b.DRGroupRef = "dr-group-1"
	if a.SemanticDigest() == b.SemanticDigest() {
		t.Fatal("drGroupRef must be part of the semantic digest")
	}
}
