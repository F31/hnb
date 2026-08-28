package engine

import (
	"context"
	"strings"
	"testing"
)

type storagePlannerResolver struct{}

func (storagePlannerResolver) ResolveLifecycleProvider(context.Context, CompatibilityDecision) (ProviderResolution, error) {
	return ProviderResolution{}, nil
}
func (storagePlannerResolver) ResolveStorageProvider(_ context.Context, action string) (ProviderResolution, error) {
	return ProviderResolution{ProviderID: "kubernetes-provider", ProviderVersion: "0.2.0", ProviderDigest: "sha256:" + strings.Repeat("a", 64), EvidenceRef: "evidence://storage.binding." + action}, nil
}
func (storagePlannerResolver) ResolveStorageDriverProvider(_ context.Context, request StorageDriverRequest) (ProviderResolution, error) {
	return ProviderResolution{ProviderID: request.PackageID, ProviderVersion: request.PackageVersion, ProviderDigest: "sha256:" + strings.Repeat("a", 64), EvidenceRef: "evidence://storage.driver." + request.Action, PackageID: request.PackageID, PackageVersion: request.PackageVersion, PackageDigest: "sha256:" + strings.Repeat("b", 64), Provisioners: []string{"example.csi.io"}, CapabilityClaims: map[string]any{"expansion": "Supported"}, RollbackVersion: request.CurrentVersion}, nil
}
func (storagePlannerResolver) ResolveRetainedVolumeProvider(_ context.Context, request RetainedVolumeProviderRequest) (ProviderResolution, error) {
	return ProviderResolution{ProviderID: request.ProviderID, ProviderVersion: "1.0.0", ProviderDigest: "sha256:" + strings.Repeat("c", 64), EvidenceRef: "evidence://storage.retained-volume." + request.Action}, nil
}

func TestPlanDeterminism(t *testing.T) {
	planner := NewPlanner()
	intent := &RuntimeIntent{
		Kind: IntentInstallRelease,
		Metadata: IntentMetadata{
			IdempotencyKey: "test-key",
			CorrelationID:  "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
		},
		Spec: IntentSpec{
			ReleaseID: "rel-42",
			TargetRef: "target-a",
			ScopeRef:  "ns-prod",
		},
	}

	// Same intent 100 times must produce identical digest.
	var firstDigest string
	for i := 0; i < 100; i++ {
		plan, err := planner.Plan(intent, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
		if err != nil {
			t.Fatalf("plan iteration %d: %v", i, err)
		}
		if i == 0 {
			firstDigest = plan.SemanticDigest
		} else if plan.SemanticDigest != firstDigest {
			t.Fatalf("iteration %d: digest %q differs from first %q", i, plan.SemanticDigest, firstDigest)
		}
	}
}

func TestPlanDigestTracksParameters(t *testing.T) {
	planner := NewPlanner()

	base := &RuntimeIntent{
		Kind: IntentInstallRelease,
		Metadata: IntentMetadata{
			IdempotencyKey: "test-key",
			CorrelationID:  "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
		},
		Spec: IntentSpec{
			ReleaseID: "rel-42",
			TargetRef: "target-a",
			ScopeRef:  "ns-prod",
			Parameters: map[string]any{
				"replicas": float64(3),
			},
		},
	}

	plan3, err := planner.Plan(base, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}

	base.Spec.Parameters["replicas"] = float64(5)
	plan5, err := planner.Plan(base, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}

	if plan3.SemanticDigest == plan5.SemanticDigest {
		t.Fatal("digest should differ when replicas changes from 3 to 5")
	}
}

func TestPlanDigestIgnoresTrackingFields(t *testing.T) {
	planner := NewPlanner()

	intentA := &RuntimeIntent{
		Kind: IntentInstallRelease,
		Metadata: IntentMetadata{
			IdempotencyKey: "key-1",
			CorrelationID:  "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
		},
		Spec: IntentSpec{
			ReleaseID: "rel-42",
			TargetRef: "target-a",
			ScopeRef:  "ns-prod",
		},
	}

	intentB := &RuntimeIntent{
		Kind: IntentInstallRelease,
		Metadata: IntentMetadata{
			IdempotencyKey: "key-2",
			CorrelationID:  "11111111-1111-1111-1111-111111111111",
		},
		Spec: IntentSpec{
			ReleaseID: "rel-42",
			TargetRef: "target-a",
			ScopeRef:  "ns-prod",
		},
	}

	planA, err := planner.Plan(intentA, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}
	planB, err := planner.Plan(intentB, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}

	if planA.SemanticDigest != planB.SemanticDigest {
		t.Fatal("digest should be identical when only idempotencyKey and correlationId differ")
	}
}

func TestPlanDigestIncludesTenantID(t *testing.T) {
	planner := NewPlanner()

	intent := &RuntimeIntent{
		Kind: IntentInstallRelease,
		Metadata: IntentMetadata{
			IdempotencyKey: "test-key",
		},
		Spec: IntentSpec{
			ReleaseID: "rel-42",
			TargetRef: "target-a",
			ScopeRef:  "ns-prod",
		},
	}

	planA, err := planner.Plan(intent, "tenant-1", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}
	planB, err := planner.Plan(intent, "tenant-2", "proj-1", "env-1", "ns-1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}

	if planA.SemanticDigest == planB.SemanticDigest {
		t.Fatal("digest should differ across tenants")
	}
}

func TestStorageBindingPlansPinIdentityProviderIdempotencyAndFences(t *testing.T) {
	planner := NewPlanner(storagePlannerResolver{})
	base := RuntimeIntent{APIVersion: "hnb.io/v1", Kind: IntentImportStorageClassBinding, Metadata: IntentMetadata{IdempotencyKey: "import-fast"}, Spec: IntentSpec{
		BindingID: "73000000-0000-0000-0000-000000000001", OfferingID: "72000000-0000-0000-0000-000000000001", OfferingVersion: 4,
		TargetID: "71000000-0000-0000-0000-000000000001", TargetKind: "KubernetesTarget", ExpectedVersion: 12,
		StorageClassName: "fast", StorageClassUID: "sc-uid-a", StorageClassResourceVersion: "1843",
	}}
	for _, kind := range []IntentKind{IntentImportStorageClassBinding, IntentReconcileStorageClassBinding} {
		intent := base
		intent.Kind = kind
		if kind == IntentReconcileStorageClassBinding {
			intent.Spec.BindingVersion = 3
		}
		plan, err := planner.Plan(&intent, "tenant-a", "", "", "", "actor-a")
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReleaseRef != "storage-binding:"+intent.Spec.BindingID || plan.TargetRef != intent.Spec.TargetID || len(plan.Steps) != 1 {
			t.Fatalf("unfixed plan: %#v", plan)
		}
		step := plan.Steps[0]
		if step.ProviderID != "kubernetes-provider" || step.ProviderVersion != "0.2.0" || step.TargetRef != intent.Spec.TargetID || step.FencingPolicy != "target-projection-and-storageclass-resource-version" {
			t.Fatalf("unfixed route/fence: %#v", step)
		}
		if step.Inputs["storageClassUid"] != "sc-uid-a" || step.Inputs["storageClassResourceVersion"] != "1843" || step.Inputs["targetProjectionVersion"] != int64(12) || step.IdempotencyKey == "" {
			t.Fatalf("identity not pinned: %#v", step.Inputs)
		}
		if len(step.SecretReferences) != 0 || len(plan.SecretReferences) != 0 || len(plan.ApprovedParameters) != 0 {
			t.Fatalf("unsanitized plan: %#v", plan)
		}
	}
}

func TestStorageBindingPlanRejectsCallerProviderAndSecretInputs(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"ImportStorageClassBinding","metadata":{"idempotencyKey":"x"},"spec":{"bindingId":"73000000-0000-0000-0000-000000000001","offeringId":"72000000-0000-0000-0000-000000000001","offeringVersion":1,"targetId":"71000000-0000-0000-0000-000000000001","targetKind":"KubernetesTarget","expectedVersion":1,"storageClassName":"fast","storageClassUid":"uid","storageClassResourceVersion":"1","parameters":{"providerId":"evil"},"secretReferences":[{"provider":"p","scope":"s","name":"n"}]}}`)
	if _, err := ParseRuntimeIntent(body); err == nil {
		t.Fatal("caller-authored provider/secret input was accepted")
	}
}

func TestStorageDriverPlansPinPackageProviderFenceAndRollback(t *testing.T) {
	planner := NewPlanner(storagePlannerResolver{})
	for _, kind := range []IntentKind{IntentInstallStorageDriver, IntentUpgradeStorageDriver, IntentUninstallStorageDriver} {
		intent := RuntimeIntent{APIVersion: "hnb.io/v1", Kind: kind, Metadata: IntentMetadata{IdempotencyKey: "driver-op"}, Spec: IntentSpec{
			InstallationID: "73000000-0000-0000-0000-000000000001", PackageID: "storage.example/driver", PackageVersion: "1.0.0",
			TargetID: "71000000-0000-0000-0000-000000000001", TargetKind: "KubernetesTarget", ExpectedVersion: 12, KubernetesVersion: "1.32.0",
			SecretReferences: []SecretReferenceEntry{{Provider: "vault", Scope: "tenant:tenant-a", Name: "driver-secret"}},
		}}
		if kind == IntentUpgradeStorageDriver {
			intent.Spec.CurrentVersion = "0.9.0"
		}
		plan, err := planner.Plan(&intent, "tenant-a", "", "", "", "actor-a")
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReleaseRef != "storage-driver:"+intent.Spec.InstallationID || len(plan.Steps) != 1 || len(plan.ArtifactDigests) != 2 {
			t.Fatalf("unfixed plan: %#v", plan)
		}
		step := plan.Steps[0]
		if step.ProviderID != intent.Spec.PackageID || step.FencingPolicy != "monotonic-worker-lease-v2" || step.Inputs["packageDigest"] == "" || step.Inputs["fencingGeneration"] != int64(12) {
			t.Fatalf("unpinned step: %#v", step)
		}
		if kind == IntentUpgradeStorageDriver && (step.Inputs["rollbackVersion"] != "0.9.0" || step.Compensation.Type != "rollback") {
			t.Fatalf("rollback metadata missing: %#v", step)
		}
	}
}

func TestStorageDriverIntentRejectsServerOwnedInputs(t *testing.T) {
	body := []byte(`{"apiVersion":"hnb.io/v1","kind":"InstallStorageDriver","metadata":{"idempotencyKey":"x"},"spec":{"installationId":"73000000-0000-0000-0000-000000000001","packageId":"storage.example/driver","packageVersion":"1.0.0","targetId":"71000000-0000-0000-0000-000000000001","targetKind":"KubernetesTarget","expectedVersion":1,"kubernetesVersion":"1.32.0","parameters":{"providerId":"evil"}}}`)
	if _, err := ParseRuntimeIntent(body); err == nil {
		t.Fatal("caller-selected provider was accepted")
	}
}

func TestRetainedVolumePlanPinsDependenciesApprovalProviderAndFence(t *testing.T) {
	intent := retainedVolumeTestIntent(IntentSanitizeRetainedVolume)
	plan, err := NewPlanner(storagePlannerResolver{}).Plan(intent, "tenant-a", "", "", "", "actor-a")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReleaseRef != "retained-volume:volume-a" || len(plan.Steps) != 1 || len(plan.PolicyDecisionRefs) != 2 {
		t.Fatalf("plan=%#v", plan)
	}
	step := plan.Steps[0]
	if step.ProviderID != "storage.example/sanitizer" || step.StepType != "storage.retained-volume.sanitize" || step.FencingPolicy != "target-pv-pvc-dependency-snapshot-and-worker-lease" || step.IdempotencyKey == "" {
		t.Fatalf("step=%#v", step)
	}
	if step.Inputs["approvalPolicy"] != "explicit-operation-approval" || step.Inputs["providerConformanceEvidenceRef"] == "" || step.Inputs["fencingGeneration"] != int64(12) {
		t.Fatalf("inputs=%#v", step.Inputs)
	}
	if _, forbidden := step.Inputs["claimRef"]; forbidden {
		t.Fatalf("generic claimRef sanitization leaked into plan: %#v", step.Inputs)
	}
}

func TestRetainedVolumeIntentFailsClosedOnDependenciesAndReclaimPolicy(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeIntent){
		"delete policy": func(i *RuntimeIntent) { i.Spec.PersistentVolume.ReclaimPolicy = "Delete" },
		"bound pv":      func(i *RuntimeIntent) { i.Spec.PersistentVolume.Phase = "Bound" },
		"live pvc":      func(i *RuntimeIntent) { i.Spec.PersistentVolumeClaim.DeletionObserved = false },
		"pod dependency": func(i *RuntimeIntent) {
			i.Spec.PodDependencies = []RetainedVolumeDependency{{Namespace: "ns", Name: "pod", UID: "pod-uid", ResourceVersion: "1"}}
		},
		"statefulset dependency": func(i *RuntimeIntent) {
			i.Spec.StatefulSetDependencies = []RetainedVolumeDependency{{Namespace: "ns", Name: "db", UID: "sts-uid", ResourceVersion: "1"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			intent := retainedVolumeTestIntent(IntentSanitizeRetainedVolume)
			mutate(intent)
			if err := NewIntentValidator().Validate(intent); err == nil {
				t.Fatal("unsafe retained-volume intent accepted")
			}
		})
	}
}

func retainedVolumeTestIntent(kind IntentKind) *RuntimeIntent {
	return &RuntimeIntent{APIVersion: "hnb.io/v1", Kind: kind, Metadata: IntentMetadata{IdempotencyKey: "sanitize-a"}, Spec: IntentSpec{
		TargetID: "71000000-0000-0000-0000-000000000001", TargetKind: "KubernetesTarget", ExpectedVersion: 12, VolumeID: "volume-a", WorkflowProviderRef: "storage.example/sanitizer",
		PersistentVolume:      RetainedVolumeResource{Name: "pv-a", UID: "pv-uid", ResourceVersion: "9", Phase: "Released", ReclaimPolicy: "Retain"},
		PersistentVolumeClaim: RetainedVolumeResource{Namespace: "ns-a", Name: "claim-a", UID: "pvc-uid", ResourceVersion: "8", DeletionObserved: true},
		PodDependencies:       []RetainedVolumeDependency{}, StatefulSetDependencies: []RetainedVolumeDependency{},
	}}
}
