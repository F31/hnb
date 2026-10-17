package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
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

func TestParseRuntimeIntentRejectsNestedParameters(t *testing.T) {
	// A caller must not be able to smuggle execution semantics as a nested
	// object/array value under an otherwise-allowed parameter name.
	cases := []string{
		`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"n1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"config":{"steps":["kubectl apply"]}}}}`,
		`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"n2"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"hooks":[{"command":"rm -rf /"}]}}}`,
		`{"apiVersion":"hnb.io/v1","kind":"CreateKubernetesTarget","metadata":{"idempotencyKey":"n3"},"spec":{"targetKind":"KubernetesTarget","displayName":"edge","credentialSecretRef":{"provider":"vault","scope":"tenant-a","name":"kubeconfig"},"parameters":{"nested":{"providerId":"p"}}}}`,
	}
	for _, body := range cases {
		_, err := ParseRuntimeIntent([]byte(body))
		if err == nil || !strings.Contains(err.Error(), "only scalar parameter values are allowed") {
			t.Fatalf("expected nested-parameter rejection, got: %v", err)
		}
	}
}

func TestParseRuntimeIntentAcceptsScalarParameters(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"s1"},"spec":{"releaseId":"r","targetRef":"t","scopeRef":"s","parameters":{"replicas":2,"enabled":true,"tier":"gold","optional":null}}}`)
	if _, err := ParseRuntimeIntent(body); err != nil {
		t.Fatalf("scalar parameters must be accepted: %v", err)
	}
	// AddNodesRuntimeTarget keeps its typed `nodes` list.
	nodeAdd := []byte(`{"apiVersion":"hnb.io/v1","kind":"AddNodesRuntimeTarget","metadata":{"idempotencyKey":"s2"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1,"sshSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-ssh"},"parameters":{"nodes":[{"name":"worker-1","role":"worker"}]}}}`)
	if _, err := ParseRuntimeIntent(nodeAdd); err != nil {
		t.Fatalf("node-add nodes list must be accepted: %v", err)
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
	} else if kind == "AddNodesRuntimeTarget" {
		spec = `"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1,"sshSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-ssh","version":"1"},"parameters":{"nodes":[{"name":"worker-1","role":"worker"},{"name":"worker-2.internal","role":"worker"}]}`
	}
	return []byte(`{"apiVersion":"hnb.io/v1","kind":"` + kind + `","metadata":{"idempotencyKey":"` + idem + `","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{` + spec + `}}`)
}

func TestParseRuntimeIntentAcceptsClusterKindsWithoutReleaseID(t *testing.T) {
	for _, kind := range []string{"CreateKubernetesTarget", "ImportRuntimeTarget", "UpgradeRuntimeTarget", "DeleteRuntimeTarget", "AddNodesRuntimeTarget"} {
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

func TestParseRuntimeIntentRejectsInvalidNodeAdditions(t *testing.T) {
	target := `"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1,"sshSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-ssh","version":"1"}`
	cases := []struct {
		name       string
		parameters string
		field      string
	}{
		{"missing nodes", `"parameters":{}`, "spec.parameters"},
		{"extra parameter", `"parameters":{"nodes":[{"name":"w1","role":"worker"}],"registryMirror":"m"}`, "spec.parameters"},
		{"empty nodes", `"parameters":{"nodes":[]}`, "spec.parameters.nodes"},
		{"nodes not a list", `"parameters":{"nodes":"worker-1"}`, "spec.parameters.nodes"},
		{"entry not an object", `"parameters":{"nodes":["worker-1"]}`, "spec.parameters.nodes[0]"},
		{"extra node field", `"parameters":{"nodes":[{"name":"w1","role":"worker","providerId":"p"}]}`, "spec.parameters.nodes[0]"},
		{"steps authored by client", `"parameters":{"nodes":[{"name":"w1","role":"worker","steps":[]}]}`, "spec.parameters.nodes[0]"},
		{"missing role", `"parameters":{"nodes":[{"name":"w1"}]}`, "spec.parameters.nodes[0]"},
		{"control-plane role", `"parameters":{"nodes":[{"name":"w1","role":"control-plane"}]}`, "spec.parameters.nodes[0].role"},
		{"invalid name", `"parameters":{"nodes":[{"name":"Worker_1!","role":"worker"}]}`, "spec.parameters.nodes[0].name"},
		{"name too long", `"parameters":{"nodes":[{"name":"` + strings.Repeat("a", 254) + `","role":"worker"}]}`, "spec.parameters.nodes[0].name"},
		{"duplicate names", `"parameters":{"nodes":[{"name":"w1","role":"worker"},{"name":"w1","role":"worker"}]}`, "spec.parameters.nodes[1].name"},
		{"too many nodes", `"parameters":{"nodes":[` + strings.Repeat(`{"name":"w","role":"worker"},`, 100) + `{"name":"w","role":"worker"}]}`, "spec.parameters.nodes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"apiVersion":"hnb.io/v1","kind":"AddNodesRuntimeTarget","metadata":{"idempotencyKey":"node-add-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{` + target + `,` + tc.parameters + `}}`)
			intent, err := ParseRuntimeIntent(body)
			if err != nil {
				validation, ok := err.(*ValidationError)
				if !ok || validation.Field != tc.field {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err := NewIntentValidator().Validate(intent); err == nil {
				t.Fatalf("expected validation failure for %s", tc.name)
			} else if validation, ok := err.(*ValidationError); !ok || validation.Field != tc.field {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
	noSSH := []byte(`{"apiVersion":"hnb.io/v1","kind":"AddNodesRuntimeTarget","metadata":{"idempotencyKey":"node-add-nossh","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":1,"parameters":{"nodes":[{"name":"worker-1","role":"worker"}]}}}`)
	if _, err := ParseRuntimeIntent(noSSH); err == nil {
		t.Fatal("node-add without sshSecretRef must be rejected")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "spec.sshSecretRef" {
		t.Fatalf("unexpected validation error: %v", err)
	}
	valid := []byte(`{"apiVersion":"hnb.io/v1","kind":"AddNodesRuntimeTarget","metadata":{"idempotencyKey":"node-add-2","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{` + target + `,"parameters":{"nodes":[{"name":"worker-1.pool.internal","role":"worker"}]}}}`)
	intent, err := ParseRuntimeIntent(valid)
	if err != nil {
		t.Fatalf("valid node-add intent rejected: %v", err)
	}
	if intent.Spec.TargetRef != intent.Spec.TargetID {
		t.Fatalf("targetRef not bound to targetId: %q", intent.Spec.TargetRef)
	}
	if digest := intent.ComputeIntentDigest(); !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest missing: %q", digest)
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

// nodeAddMatrix flips the node-add cell to REQUIRED so the plan factory can be
// exercised before provider conformance (task 6.3) activates it in production.
func nodeAddMatrix(t *testing.T) *CompatibilityMatrix {
	t.Helper()
	var document map[string]any
	raw, err := os.ReadFile("runtime-target-compatibility-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &document) != nil {
		t.Fatal("decode embedded matrix")
	}
	rows := document["rows"].([]any)
	for _, row := range rows {
		row.(map[string]any)["actions"].(map[string]any)["node-add"] = "REQUIRED"
	}
	updated, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := NewCompatibilityMatrix(updated, func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	return matrix
}

func TestPlannerRefusesNodeAddOnDefaultMatrix(t *testing.T) {
	intent, err := ParseRuntimeIntent(clusterIntentBody("AddNodesRuntimeTarget", "node-add-refused"))
	if err != nil {
		t.Fatal(err)
	}
	planner := NewPlanner(testLifecycleResolver{})
	_, err = planner.Plan(intent, "tenant-a", "", "", "", "user-1")
	if code, ok := CompatibilityErrorCode(err); !ok || code != CodeTargetActionUnsupported {
		t.Fatalf("expected TARGET_ACTION_UNSUPPORTED, got %v", err)
	}
}

func TestPlannerShapesNodeAddStepUnderRequiredMatrix(t *testing.T) {
	intent, err := ParseRuntimeIntent(clusterIntentBody("AddNodesRuntimeTarget", "node-add-planned"))
	if err != nil {
		t.Fatal(err)
	}
	planner := NewPlannerWithCompatibility(nodeAddMatrix(t), testLifecycleResolver{})
	plan, err := planner.Plan(intent, "tenant-a", "", "", "", "user-1")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected a single step, got %+v", plan.Steps)
	}
	step := plan.Steps[0]
	if step.StepType != "runtime_target.kubernetes.add-nodes" || step.StepID != "runtime_target.kubernetes.add-nodes" {
		t.Fatalf("unexpected step type: %+v", step)
	}
	if step.ProviderID != "runtime-target.lifecycle.kubernetes" || step.ProviderProtocolVersion != "2.0.0" {
		t.Fatalf("unexpected provider pinning: %+v", step)
	}
	if step.FencingPolicy != "monotonic-worker-lease-v2" || step.TimeoutSeconds != 1800 {
		t.Fatalf("unexpected fencing/timeout: %+v", step)
	}
	if step.Compensation == nil || step.Compensation.Type != "none" {
		t.Fatalf("node-add compensation must be none: %+v", step.Compensation)
	}
	if !strings.HasPrefix(step.IdempotencyKey, "step:") {
		t.Fatalf("idempotency key not step-scoped: %q", step.IdempotencyKey)
	}
	nodes, ok := step.Inputs["parameters"].(map[string]any)["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("nodes payload not carried into step inputs: %+v", step.Inputs)
	}
	if step.Inputs["action"] != "node-add" || step.Inputs["fencingGeneration"] != int64(1) {
		t.Fatalf("unexpected step action/fencing: %+v", step.Inputs)
	}
	if plan.CompatibilityDecision == nil || plan.CompatibilityDecision.Action != "node-add" || plan.CompatibilityDecision.Status != "REQUIRED" {
		t.Fatalf("unexpected compatibility decision: %+v", plan.CompatibilityDecision)
	}
}
