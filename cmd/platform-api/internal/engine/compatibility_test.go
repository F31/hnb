package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type testLifecycleResolver struct{}

func (testLifecycleResolver) ResolveLifecycleProvider(_ context.Context, decision CompatibilityDecision) (ProviderResolution, error) {
	return ProviderResolution{
		ProviderID: decision.ProviderID, ProviderVersion: "1.2.3",
		ProviderDigest: "sha256:" + strings.Repeat("a", 64), EvidenceRef: "evidence://" + decision.Action,
	}, nil
}

func TestEmbeddedCompatibilityMatrixMatchesCanonicalContract(t *testing.T) {
	canonical, err := os.ReadFile("../../../../contracts/schema/runtime-target/v1/compatibility-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(canonical), bytes.TrimSpace(defaultCompatibilityMatrixJSON)) {
		t.Fatal("embedded runtime matrix drifted from the canonical contract")
	}
}

func TestCompatibilityMatrixEvaluatesEveryCell(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) }
	matrix, err := NewCompatibilityMatrix(defaultCompatibilityMatrixJSON, now)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		kind       IntentKind
		targetKind string
		action     string
		supported  bool
	}{
		{IntentCreateKubernetesTarget, "KubernetesTarget", "create", true},
		{IntentImportRuntimeTarget, "KubernetesTarget", "import", true},
		{IntentUpgradeRuntimeTarget, "KubernetesTarget", "upgrade", true},
		{IntentDeleteRuntimeTarget, "KubernetesTarget", "unmanage", true},
		{IntentCreateKubernetesTarget, "EdgeRuntimeTarget", "create", false},
		{IntentImportRuntimeTarget, "EdgeRuntimeTarget", "import", true},
		{IntentUpgradeRuntimeTarget, "EdgeRuntimeTarget", "upgrade", true},
		{IntentDeleteRuntimeTarget, "EdgeRuntimeTarget", "unmanage", true},
	}
	for _, test := range tests {
		t.Run(test.targetKind+"/"+test.action, func(t *testing.T) {
			decision, err := matrix.Evaluate(test.kind, test.targetKind)
			if test.supported && err != nil {
				t.Fatal(err)
			}
			if !test.supported {
				if code, ok := CompatibilityErrorCode(err); !ok || code != CodeTargetActionUnsupported {
					t.Fatalf("error=%v code=%q", err, code)
				}
				return
			}
			if decision.Action != test.action || decision.Status != "REQUIRED" || decision.ProviderID == "" || decision.MatrixVersion != "1.0.0" {
				t.Fatalf("unexpected decision: %+v", decision)
			}
		})
	}
}

func TestCompatibilityMatrixFailsClosedOutsideValidityWindow(t *testing.T) {
	for _, now := range []time.Time{
		time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2027, 8, 1, 0, 0, 0, 0, time.UTC),
	} {
		matrix, err := NewCompatibilityMatrix(defaultCompatibilityMatrixJSON, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if _, err := matrix.Evaluate(IntentImportRuntimeTarget, "KubernetesTarget"); err == nil {
			t.Fatalf("matrix accepted at %s", now)
		} else if code, _ := CompatibilityErrorCode(err); code != CodeProviderRouteNotFound {
			t.Fatalf("code=%q err=%v", code, err)
		}
		if got := matrix.Decisions(); len(got) != 0 {
			t.Fatalf("published %d decisions outside validity window", len(got))
		}
	}
}

func TestCompatibilityMatrixRejectsMissingAndDuplicateCells(t *testing.T) {
	var document compatibilityMatrixDocument
	if err := json.Unmarshal(defaultCompatibilityMatrixJSON, &document); err != nil {
		t.Fatal(err)
	}
	duplicate := document
	duplicate.Rows = append([]compatibilityMatrixRow(nil), document.Rows...)
	duplicate.Rows[1].TargetKind = duplicate.Rows[0].TargetKind
	data, _ := json.Marshal(duplicate)
	if _, err := NewCompatibilityMatrix(data, time.Now); err == nil {
		t.Fatal("duplicate target-kind row was accepted")
	}

	missing := document
	missing.Rows = append([]compatibilityMatrixRow(nil), document.Rows...)
	missing.Rows[0].Actions = map[string]string{"create": "REQUIRED", "import": "REQUIRED", "upgrade": "REQUIRED"}
	data, _ = json.Marshal(missing)
	if _, err := NewCompatibilityMatrix(data, time.Now); err == nil {
		t.Fatal("missing action cell was accepted")
	}
}

func TestPlannerPinsMatrixAndProviderResolution(t *testing.T) {
	intent, err := ParseRuntimeIntent(clusterIntentBody("ImportRuntimeTarget", "matrix-plan"))
	if err != nil {
		t.Fatal(err)
	}
	planner := NewPlanner(testLifecycleResolver{})
	plan, err := planner.PlanContext(context.Background(), intent, "tenant-a", "", "", "", "actor-a")
	if err != nil {
		t.Fatal(err)
	}
	if plan.CompatibilityDecision == nil || plan.CompatibilityDecision.MatrixVersion != "1.0.0" ||
		plan.ProviderVersions["runtime-target.lifecycle.kubernetes"] != "1.2.3" || len(plan.ArtifactDigests) != 1 {
		t.Fatalf("compatibility evidence was not pinned: %+v", plan)
	}
	if !strings.HasPrefix(plan.CapabilitySnapshotDigest, "sha256:") || plan.TargetRef == "" {
		t.Fatalf("target/capability snapshot not pinned: target=%q digest=%q", plan.TargetRef, plan.CapabilitySnapshotDigest)
	}
	for _, step := range plan.Steps {
		if step.ProviderID != "runtime-target.lifecycle.kubernetes" {
			t.Fatalf("step has no exact provider route: %+v", step)
		}
	}
}
