package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type goldenLifecycleResolver struct{}

func (goldenLifecycleResolver) ResolveLifecycleProvider(_ context.Context, decision CompatibilityDecision) (ProviderResolution, error) {
	kind := "kubernetes"
	if decision.TargetKind == "EdgeRuntimeTarget" {
		kind = "edge"
	}
	return ProviderResolution{
		ProviderID:      "runtime-target.lifecycle." + kind,
		ProviderVersion: "1.0.0",
		ProviderDigest:  "sha256:" + strings.Repeat("a", 64),
		EvidenceRef:     "evidence://" + kind + "/" + decision.Action,
	}, nil
}

func TestLifecyclePlanGoldenFixtures(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		body      string
		fixture   string
		wantStep  string
		wantError string
	}{
		{
			name:     "kubernetes-create",
			kind:     "CreateKubernetesTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"CreateKubernetesTarget","metadata":{"idempotencyKey":"k8s-create-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"prod-cluster","desiredVersion":"v1.31.0","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`,
			fixture:  "kubernetes-create.json",
			wantStep: "runtime_target.kubernetes.provision-and-register",
		},
		{
			name:     "kubernetes-import",
			kind:     "ImportRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"k8s-import-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"prod-cluster","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`,
			fixture:  "kubernetes-import.json",
			wantStep: "runtime_target.kubernetes.register",
		},
		{
			name:     "kubernetes-upgrade",
			kind:     "UpgradeRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"UpgradeRuntimeTarget","metadata":{"idempotencyKey":"k8s-upgrade-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":7,"desiredVersion":"v1.32.0"}}`,
			fixture:  "kubernetes-upgrade.json",
			wantStep: "runtime_target.kubernetes.upgrade",
		},
		{
			name:     "kubernetes-unmanage",
			kind:     "DeleteRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"DeleteRuntimeTarget","metadata":{"idempotencyKey":"k8s-unmanage-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c2","targetKind":"KubernetesTarget","expectedVersion":7}}`,
			fixture:  "kubernetes-unmanage.json",
			wantStep: "runtime_target.kubernetes.unregister",
		},
		{
			name:     "edge-import",
			kind:     "ImportRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"edge-import-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"EdgeRuntimeTarget","displayName":"edge-cluster","cloudCoreEndpoint":"https://cloudcore.internal:10002","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"edge-credential","version":"1"},"nodeGroupMappings":{"default":"group-a"}}}`,
			fixture:  "edge-import.json",
			wantStep: "runtime_target.edge.register",
		},
		{
			name:     "edge-upgrade",
			kind:     "UpgradeRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"UpgradeRuntimeTarget","metadata":{"idempotencyKey":"edge-upgrade-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c3","targetKind":"EdgeRuntimeTarget","expectedVersion":3,"desiredVersion":"v1.20.0"}}`,
			fixture:  "edge-upgrade.json",
			wantStep: "runtime_target.edge.upgrade",
		},
		{
			name:     "edge-unmanage",
			kind:     "DeleteRuntimeTarget",
			body:     `{"apiVersion":"hnb.io/v1","kind":"DeleteRuntimeTarget","metadata":{"idempotencyKey":"edge-unmanage-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c3","targetKind":"EdgeRuntimeTarget","expectedVersion":3}}`,
			fixture:  "edge-unmanage.json",
			wantStep: "runtime_target.edge.unregister",
		},
		{
			name:      "edge-create-unsupported",
			kind:      "CreateKubernetesTarget",
			body:      `{"apiVersion":"hnb.io/v1","kind":"CreateKubernetesTarget","metadata":{"idempotencyKey":"edge-create-1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"EdgeRuntimeTarget","displayName":"edge-create","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"edge-credential","version":"1"}}}`,
			wantError: CodeTargetActionUnsupported,
		},
		{
			name:      "provider-route-missing",
			kind:      "ImportRuntimeTarget",
			body:      `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"route-missing","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`,
			wantError: CodeProviderRouteNotFound,
		},
	}

	missingResolver := missingLifecycleProviderResolver{}
	fixedTime := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	normalizer := func(plan *ExecutionPlan) map[string]any {
		document := map[string]any{
			"planId":                   plan.PlanID,
			"intentId":                 plan.IntentID,
			"semanticDigest":           plan.SemanticDigest,
			"releaseRef":               plan.ReleaseRef,
			"artifactDigests":          plan.ArtifactDigests,
			"targetRef":                plan.TargetRef,
			"capabilitySnapshotDigest": plan.CapabilitySnapshotDigest,
			"providerVersions":         plan.ProviderVersions,
			"policyDecisionRefs":       plan.PolicyDecisionRefs,
			"approvedParameters":       plan.ApprovedParameters,
			"secretReferences":         plan.SecretReferences,
			"compatibilityDecision":    plan.CompatibilityDecision,
			"targetSnapshot":           plan.TargetSnapshot,
			"steps":                    plan.Steps,
		}
		return document
	}

	_ = missingResolver
	_ = fixedTime
	_ = normalizer

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := ParseRuntimeIntent([]byte(tc.body))
			if err != nil {
				t.Fatalf("parse %s: %v", tc.name, err)
			}
			resolver := LifecycleProviderResolver(goldenLifecycleResolver{})
			if tc.name == "provider-route-missing" {
				resolver = LifecycleProviderResolver(missingLifecycleProviderResolver{})
			}
			planner := NewPlanner(resolver)
			planner.now = func() time.Time { return fixedTime }
			plan, err := planner.PlanContext(context.Background(), intent, "tenant-a", "", "", "", "actor-1")
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error %q, got plan", tc.wantError)
				}
				if code, ok := CompatibilityErrorCode(err); !ok || code != tc.wantError {
					t.Fatalf("expected error code %q, got %v", tc.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan %s: %v", tc.name, err)
			}
			if len(plan.Steps) != 1 || plan.Steps[0].StepType != tc.wantStep {
				t.Fatalf("%s: expected step %q, got %+v", tc.name, tc.wantStep, plan.Steps)
			}
			step := plan.Steps[0]
			if step.ProviderID == "" || step.ProviderVersion == "" || step.ProviderDigest == "" ||
				step.ProviderProtocolVersion == "" || step.InputSchema == "" || step.Inputs == nil ||
				step.IdempotencyKey == "" || step.FencingPolicy == "" || step.RetryPolicy == nil ||
				step.TimeoutSeconds == 0 || step.Compensation == nil || step.TargetRef == "" {
				t.Fatalf("%s: step missing immutable metadata: %+v", tc.name, step)
			}
			if step.ProviderProtocolVersion != "2.0.0" {
				t.Fatalf("%s: protocol %q", tc.name, step.ProviderProtocolVersion)
			}
			if err := assertNoSecretValues(plan); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if tc.fixture != "" {
				document := normalizer(plan)
				got, _ := json.MarshalIndent(document, "", "  ")
				fixturePath := filepath.Join("testdata", tc.fixture)
				if os.Getenv("HNB_UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(fixturePath, append(got, '\n'), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				expected, err := os.ReadFile(fixturePath)
				if err != nil {
					t.Fatalf("read golden %s: %v (run with HNB_UPDATE_GOLDEN=1 to create)", fixturePath, err)
				}
				if string(expected) != string(append(got, '\n')) {
					t.Fatalf("golden mismatch %s:\n--- expected ---\n%s\n--- got ---\n%s", tc.fixture, expected, got)
				}
			}
		})
	}
}

type missingLifecycleProviderResolver struct{}

func (missingLifecycleProviderResolver) ResolveLifecycleProvider(context.Context, CompatibilityDecision) (ProviderResolution, error) {
	return ProviderResolution{}, &CompatibilityError{Code: CodeProviderRouteNotFound, Reason: "no provider"}
}

func assertNoSecretValues(plan *ExecutionPlan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	doc := string(data)
	lowered := strings.ToLower(doc)
	for _, forbidden := range []string{"token:", "secretvalue", "kubeconfig:", "privatekey", "-----begin"} {
		if strings.Contains(lowered, forbidden) {
			return &CompatibilityError{Code: "SECRET_LEAK", Reason: "plan contains forbidden secret marker: " + forbidden}
		}
	}
	return nil
}

func TestLifecyclePlanDigestStabilityAndProviderPin(t *testing.T) {
	intent, err := ParseRuntimeIntent([]byte(`{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"digest-stability","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	planner := NewPlanner(goldenLifecycleResolver{})
	a, err := planner.PlanContext(context.Background(), intent, "tenant-a", "", "", "", "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	intent2, _ := ParseRuntimeIntent([]byte(`{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"digest-stability","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`))
	b, err := planner.PlanContext(context.Background(), intent2, "tenant-a", "", "", "", "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if a.SemanticDigest != b.SemanticDigest {
		t.Fatalf("digest not stable: %q vs %q", a.SemanticDigest, b.SemanticDigest)
	}
	mutated, _ := ParseRuntimeIntent([]byte(`{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"digest-stability","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-b","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`))
	c, err := planner.PlanContext(context.Background(), mutated, "tenant-a", "", "", "", "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if a.SemanticDigest == c.SemanticDigest {
		t.Fatalf("digest did not change when display name changed")
	}
	if c.Steps[0].ProviderID != "runtime-target.lifecycle.kubernetes" {
		t.Fatalf("unexpected provider pin: %q", c.Steps[0].ProviderID)
	}
}
