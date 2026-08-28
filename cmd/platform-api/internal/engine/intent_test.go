package engine

import (
	"fmt"
	"strings"
	"testing"
)

func validRuntimeIntentBody() []byte {
	return []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"test-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"releaseId":"rel-42","targetRef":"target-a","scopeRef":"ns-prod"}}`)
}

func TestParseRuntimeIntentValid(t *testing.T) {
	intent, err := ParseRuntimeIntent(validRuntimeIntentBody())
	if err != nil {
		t.Fatalf("parse valid intent: %v", err)
	}
	if intent.APIVersion != "hnb.io/v1" {
		t.Fatalf("apiVersion = %q", intent.APIVersion)
	}
	if intent.Kind != IntentInstallRelease {
		t.Fatalf("kind = %q", intent.Kind)
	}
	if intent.Spec.ReleaseID != "rel-42" {
		t.Fatalf("releaseId = %q", intent.Spec.ReleaseID)
	}
	if intent.Spec.TargetRef != "target-a" {
		t.Fatalf("targetRef = %q", intent.Spec.TargetRef)
	}
	if intent.Metadata.CorrelationID != "018f6c2a-4a64-7b58-9cc3-9f70462f36c1" {
		t.Fatalf("correlationId mismatch")
	}
}

func TestParseRuntimeIntentRejectsMissingKind(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","metadata":{"idempotencyKey":"k1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsMissingMetadata(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("expected metadata error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsMissingSpec(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k1"}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected spec error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsInvalidKind(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"DeleteRelease","metadata":{"idempotencyKey":"k1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind validation error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsStepsInParameters(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"steps":"kubectl apply"}}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden param error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsCredentialInParameters(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k2"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"credentials":"secret"}}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden credentials error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsPolicyResultInParameters(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k3"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"policyResult":"allow"}}}`)
	_, err := ParseRuntimeIntent(body)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden policyResult error, got: %v", err)
	}
}

func TestParseRuntimeIntentRejectsTooManyParameters(t *testing.T) {
	params := `{`
	for i := 0; i < 65; i++ {
		if i > 0 {
			params += ","
		}
		params += fmt.Sprintf(`"key%d":"val"`, i)
	}
	params += `}`
	bodyStr := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k4"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":` + params + `}}`
	_, err := ParseRuntimeIntent([]byte(bodyStr))
	if err == nil || !strings.Contains(err.Error(), "max 64") {
		t.Fatalf("expected max parameters error, got: %v", err)
	}
}

func TestParseRuntimeIntentUnknownTopLevelField(t *testing.T) {
	intent, err := ParseRuntimeIntent(validRuntimeIntentBody())
	if err != nil {
		t.Fatalf("parse valid: %v", err)
	}
	raw := `{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k5"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"},"extra":"bad"}`
	err = intent.ValidateNoExtraFields([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

func TestValidateDAGValid(t *testing.T) {
	steps := []Step{
		{StepID: "a", StepType: "validate"},
		{StepID: "b", StepType: "deploy", DependsOn: []string{"a"}},
		{StepID: "c", StepType: "verify", DependsOn: []string{"b"}},
	}
	if err := validateDAG(steps); err != nil {
		t.Fatalf("valid DAG rejected: %v", err)
	}
}

func TestValidateDAGCycleDetected(t *testing.T) {
	steps := []Step{
		{StepID: "a", StepType: "x", DependsOn: []string{"b"}},
		{StepID: "b", StepType: "y", DependsOn: []string{"a"}},
	}
	if err := validateDAG(steps); err == nil {
		t.Fatal("expected cycle detection error")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected 'cycle' in error, got: %v", err)
	}
}

func TestValidateDAGUnknownDependency(t *testing.T) {
	steps := []Step{
		{StepID: "a", StepType: "x", DependsOn: []string{"ghost"}},
	}
	if err := validateDAG(steps); err == nil {
		t.Fatal("expected unknown dependency error")
	} else if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected 'unknown' in error, got: %v", err)
	}
}

func TestValidatorAcceptsValidIntent(t *testing.T) {
	intent, _ := ParseRuntimeIntent(validRuntimeIntentBody())
	v := NewIntentValidator()
	if err := v.Validate(intent); err != nil {
		t.Fatalf("validator rejected valid intent: %v", err)
	}
}

func TestValidatorRejectsEmptyReleaseID(t *testing.T) {
	intent, _ := ParseRuntimeIntent(validRuntimeIntentBody())
	intent.Spec.ReleaseID = ""
	v := NewIntentValidator()
	err := v.Validate(intent)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected releaseId required error, got: %v", err)
	}
}

func TestEngineProcessReturnsPlan(t *testing.T) {
	intent, err := ParseRuntimeIntent(validRuntimeIntentBody())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	e := NewEngine()
	plan, err := e.Process(intent)
	if err != nil {
		t.Fatalf("engine process: %v", err)
	}
	if plan == nil {
		t.Fatal("nil plan returned")
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan has no steps")
	}
	if plan.SemanticDigest == "" {
		t.Fatal("empty semantic digest")
	}
}

func TestPlannerGeneratesInstallSteps(t *testing.T) {
	intent, _ := ParseRuntimeIntent(validRuntimeIntentBody())
	planner := NewPlanner()
	plan, err := planner.Plan(intent, "", "", "", "", "")
	if err != nil {
		t.Fatalf("plan install: %v", err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("expected 3 install steps, got %d", len(plan.Steps))
	}
	stepTypes := make(map[string]bool)
	for _, s := range plan.Steps {
		stepTypes[s.StepType] = true
	}
	if !stepTypes["validate"] || !stepTypes["helm"] || !stepTypes["verify"] {
		t.Fatalf("missing expected step types: %v", stepTypes)
	}
}

func TestPlannerGeneratesUninstallSteps(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"UninstallRelease","metadata":{"idempotencyKey":"u1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	intent, _ := ParseRuntimeIntent(body)
	planner := NewPlanner()
	plan, err := planner.Plan(intent, "", "", "", "", "")
	if err != nil {
		t.Fatalf("plan uninstall: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("uninstall plan has no steps")
	}
}

func TestPlannerGeneratesUpgradeSteps(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"UpgradeRelease","metadata":{"idempotencyKey":"up1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	intent, _ := ParseRuntimeIntent(body)
	planner := NewPlanner()
	plan, err := planner.Plan(intent, "", "", "", "", "")
	if err != nil {
		t.Fatalf("plan upgrade: %v", err)
	}
	if len(plan.Steps) < 3 {
		t.Fatalf("upgrade plan too few steps: %d", len(plan.Steps))
	}
}

func TestPlannerGeneratesRollbackSteps(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"RollbackRelease","metadata":{"idempotencyKey":"rb1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	intent, _ := ParseRuntimeIntent(body)
	planner := NewPlanner()
	plan, err := planner.Plan(intent, "", "", "", "", "")
	if err != nil {
		t.Fatalf("plan rollback: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("rollback plan has no steps")
	}
}

func TestPlannerGeneratesConfigChangeSteps(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"ChangeConfiguration","metadata":{"idempotencyKey":"cc1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s"}}`)
	intent, _ := ParseRuntimeIntent(body)
	planner := NewPlanner()
	plan, err := planner.Plan(intent, "", "", "", "", "")
	if err != nil {
		t.Fatalf("plan config change: %v", err)
	}
	if len(plan.Steps) < 3 {
		t.Fatalf("config change plan too few steps: %d", len(plan.Steps))
	}
}

func clusterIntentBody(kind, idem string) []byte {
	spec := `"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}`
	if kind == "UpgradeRuntimeTarget" {
		spec = `"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1,"desiredVersion":"v1.31.0"`
	} else if kind == "DeleteRuntimeTarget" {
		spec = `"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1`
	}
	return []byte(`{"apiVersion":"hnb.io/v1","kind":"` + kind + `","metadata":{"idempotencyKey":"` + idem + `","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{` + spec + `}}`)
}

func TestParseRuntimeIntentAcceptsClusterKindsWithoutReleaseID(t *testing.T) {
	for _, kind := range []string{"CreateKubernetesTarget", "ImportRuntimeTarget", "UpgradeRuntimeTarget", "DeleteRuntimeTarget"} {
		intent, err := ParseRuntimeIntent(clusterIntentBody(kind, "idem-"+kind))
		if err != nil {
			t.Fatalf("ParseRuntimeIntent(%s): %v", kind, err)
		}
		if intent.Spec.ReleaseID != "" {
			t.Fatalf("%s: unexpected releaseId %q", kind, intent.Spec.ReleaseID)
		}
		v := NewIntentValidator()
		if err := v.Validate(intent); err != nil {
			t.Fatalf("Validate(%s): %v", kind, err)
		}
	}
}

func TestValidatorRejectsUnknownKind(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"BogusKind","metadata":{"idempotencyKey":"k1"},"spec":{"targetRef":"t","scopeRef":"s"}}`)
	if _, err := ParseRuntimeIntent(body); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("expected unsupported kind error, got: %v", err)
	}
}

func TestPlannerGeneratesClusterKindSteps(t *testing.T) {
	cases := []struct{ kind, wantStepType string }{
		{"CreateKubernetesTarget", "runtime_target.kubernetes.provision-and-register"},
		{"ImportRuntimeTarget", "runtime_target.kubernetes.register"},
		{"UpgradeRuntimeTarget", "runtime_target.kubernetes.upgrade"},
		{"DeleteRuntimeTarget", "runtime_target.kubernetes.unregister"},
	}
	for _, tc := range cases {
		intent, err := ParseRuntimeIntent(clusterIntentBody(tc.kind, "idem-"+tc.kind))
		if err != nil {
			t.Fatalf("parse %s: %v", tc.kind, err)
		}
		planner := NewPlanner(testLifecycleResolver{})
		plan, err := planner.Plan(intent, "tenant-a", "", "", "", "user-1")
		if err != nil {
			t.Fatalf("plan %s: %v", tc.kind, err)
		}
		if len(plan.Steps) == 0 {
			t.Fatalf("%s: empty steps", tc.kind)
		}
		found := false
		for _, s := range plan.Steps {
			if s.StepType == tc.wantStepType {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: missing step type %q in %+v", tc.kind, tc.wantStepType, plan.Steps)
		}
		if plan.ReleaseRef == "" {
			t.Fatalf("%s: empty releaseRef must be placeholder", tc.kind)
		}
	}
}

func TestNormalizeCloudCoreEndpointRejectsCredentialBearingURLs(t *testing.T) {
	for _, endpoint := range []string{
		"https://user:pass@cloudcore.internal:10002/path",
		"https://cloudcore.internal/path?token=secret",
		"wss://cloudcore.internal/path#credential",
		"http://cloudcore.internal:10002",
		"https://cloudcore.internal:70000",
	} {
		if _, err := normalizeCloudCoreEndpoint(endpoint); err == nil {
			t.Fatalf("expected endpoint rejection: %s", endpoint)
		}
	}
	if got, err := normalizeCloudCoreEndpoint("wss://cloudcore.internal:10002/api"); err != nil || got != "wss://cloudcore.internal:10002/api" {
		t.Fatalf("valid endpoint got=%q err=%v", got, err)
	}
}
