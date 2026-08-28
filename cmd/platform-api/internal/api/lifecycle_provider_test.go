package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/core"
)

func kubernetesImportDecision() engine.CompatibilityDecision {
	return engine.CompatibilityDecision{
		MatrixVersion: "1.0.0", ProviderProtocolVersion: "2.0.0", TargetKind: "KubernetesTarget",
		Action: "import", Status: "REQUIRED", ProviderID: "runtime-target.lifecycle.kubernetes",
	}
}

func TestLifecycleProviderResolverFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*fakeStore)
		wantCode string
	}{
		{name: "missing", mutate: func(st *fakeStore) { delete(st.manifests, "runtime-target.lifecycle.kubernetes") }, wantCode: engine.CodeProviderRouteNotFound},
		{name: "wrong protocol", mutate: func(st *fakeStore) { st.manifests["runtime-target.lifecycle.kubernetes"].ProtocolVersion = "1.0.0" }, wantCode: engine.CodeProviderIncompatible},
		{name: "not production ready", mutate: func(st *fakeStore) { st.manifests["runtime-target.lifecycle.kubernetes"].ConformanceLevel = "basic" }, wantCode: engine.CodeProviderIncompatible},
		{name: "missing expiry", mutate: func(st *fakeStore) { st.manifests["runtime-target.lifecycle.kubernetes"].ConformanceExpiresAt = nil }, wantCode: engine.CodeProviderIncompatible},
		{name: "expired", mutate: func(st *fakeStore) {
			expired := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
			st.manifests["runtime-target.lifecycle.kubernetes"].ConformanceExpiresAt = &expired
		}, wantCode: engine.CodeProviderIncompatible},
		{name: "missing action", mutate: func(st *fakeStore) { st.manifests["runtime-target.lifecycle.kubernetes"].Actions = []string{"create"} }, wantCode: engine.CodeProviderIncompatible},
		{name: "missing cell evidence", mutate: func(st *fakeStore) { st.manifests["runtime-target.lifecycle.kubernetes"].ConformanceEvidence = nil }, wantCode: engine.CodeProviderIncompatible},
		{name: "failed cell evidence", mutate: func(st *fakeStore) {
			st.manifests["runtime-target.lifecycle.kubernetes"].ConformanceEvidence[1].Passed = false
		}, wantCode: engine.CodeProviderIncompatible},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := newFakeStore()
			test.mutate(st)
			resolver := lifecycleProviderResolver{store: st, now: func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) }}
			_, err := resolver.ResolveLifecycleProvider(context.Background(), kubernetesImportDecision())
			if code, ok := engine.CompatibilityErrorCode(err); !ok || code != test.wantCode {
				t.Fatalf("code=%q err=%v", code, err)
			}
		})
	}
}

func TestLifecycleProviderResolverPinsEligibleManifest(t *testing.T) {
	resolver := lifecycleProviderResolver{store: newFakeStore(), now: func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) }}
	provider, err := resolver.ResolveLifecycleProvider(context.Background(), kubernetesImportDecision())
	if err != nil {
		t.Fatal(err)
	}
	if provider.ProviderID != "runtime-target.lifecycle.kubernetes" || provider.ProviderVersion != "1.0.0" ||
		!strings.HasPrefix(provider.ProviderDigest, "sha256:") || provider.EvidenceRef == "" {
		t.Fatalf("unexpected provider resolution: %+v", provider)
	}
}

func TestConsoleBootstrapPublishesOnlyEligibleMatrixCells(t *testing.T) {
	srv, st := newTestServer()
	delete(st.manifests, "runtime-target.lifecycle.edge")
	rec := doRequest(t, srv, http.MethodGet, "/v1/console/bootstrap", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Capabilities []map[string]string `json:"capabilities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	seenKubernetes := false
	for _, capability := range response.Capabilities {
		id := capability["id"]
		if id == "runtime-target.lifecycle.kubernetes.import" && capability["version"] == "1.0.0" {
			seenKubernetes = true
		}
		if strings.HasPrefix(id, "runtime-target.lifecycle.edge.") {
			t.Fatalf("ineligible Edge capability was published: %+v", capability)
		}
	}
	if !seenKubernetes {
		t.Fatalf("eligible Kubernetes capability missing: %+v", response.Capabilities)
	}
}

func TestEdgeCreateIsRejectedByMatrixWithoutMutation(t *testing.T) {
	srv, st := newTestServer()
	body := `{"apiVersion":"hnb.io/v1","kind":"CreateKubernetesTarget","metadata":{"idempotencyKey":"edge-create-matrix","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"EdgeRuntimeTarget","displayName":"edge-a"}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), engine.CodeTargetActionUnsupported) || len(st.ops) != 0 {
		t.Fatalf("status=%d operations=%d body=%s", rec.Code, len(st.ops), rec.Body.String())
	}
}

func TestMissingLifecycleProviderFailsBeforeMutation(t *testing.T) {
	srv, st := newTestServer()
	delete(st.manifests, "runtime-target.lifecycle.kubernetes")
	body := `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"missing-provider","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"vault","scope":"tenant","name":"cluster-credential","version":"1"}}}`
	rec := doRequest(t, srv, http.MethodPost, "/v1/intents", body)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), engine.CodeProviderRouteNotFound) || len(st.ops) != 0 {
		t.Fatalf("status=%d operations=%d body=%s", rec.Code, len(st.ops), rec.Body.String())
	}
}

type duplicateManifestReader struct{}

func (duplicateManifestReader) GetManifest(context.Context, string) (*store.ProviderManifest, error) {
	return nil, errors.New("ambiguous provider registration")
}

func TestAmbiguousProviderRegistrationFailsClosed(t *testing.T) {
	resolver := lifecycleProviderResolver{store: duplicateManifestReader{}}
	_, err := resolver.ResolveLifecycleProvider(context.Background(), kubernetesImportDecision())
	if code, _ := engine.CompatibilityErrorCode(err); code != engine.CodeProviderRouteNotFound {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func eligibleStorageDriverManifest(now time.Time) *store.ProviderManifest {
	digest := "sha256:" + strings.Repeat("a", 64)
	expires := now.Add(24 * time.Hour)
	return &store.ProviderManifest{ProviderID: "storage.example/driver", Name: "driver", Version: "1.0.0", ProtocolVersion: "2.0.0",
		Actions: []string{"storage.driver.install", "storage.driver.upgrade", "storage.driver.uninstall"}, Compatibility: map[string]any{"licensed": true, "healthy": true},
		ConformanceLevel: "production_ready", ConformanceExpiresAt: &expires, ConformanceEvidence: []store.ConformanceEvidence{{TestName: "storage.driver.install", Category: "storage-driver-lifecycle", Passed: true, EvidenceRef: "evidence://install"}, {TestName: "storage.driver.upgrade", Category: "storage-driver-lifecycle", Passed: true, EvidenceRef: "evidence://upgrade"}, {TestName: "storage.driver.uninstall", Category: "storage-driver-lifecycle", Passed: true, EvidenceRef: "evidence://uninstall"}},
		StorageDriverPackage: &core.StorageDriverPackage{SchemaVersion: "1.0.0", PackageID: "storage.example/driver", PackageVersion: "1.0.0", Provisioners: []string{"example.csi.io"}, Compatibility: core.StorageDriverCompatibility{KubernetesVersions: []core.VersionRange{{MinInclusive: "1.30.0", MaxExclusive: "1.34.0"}}, UpgradeFromVersions: []string{"0.9.0"}, RollbackToVersions: []string{"0.9.0"}}, Capabilities: core.StorageDriverCapabilities{VolumeModes: []string{"Block"}, AccessModes: []string{"ReadWriteOnce"}, Topology: "Supported", CapacityTracking: []string{"CSIStorageCapacity"}, Expansion: "Supported", Clone: "Unsupported", Snapshot: "Supported", Ephemeral: "Unsupported", Health: "Supported"}, PackageDigest: digest, Signature: core.PackageSignature{Format: "Cosign", KeyID: "key", SignedDigest: digest, EvidenceRef: "oci://signature"}, ConformanceEvidence: []core.StorageConformanceEvidence{{PackageVersion: "1.0.0", KubernetesVersion: "1.32.0", SuiteVersion: "1.0.0", PassedAt: now.Add(-time.Hour), ExpiresAt: expires, EvidenceRef: "oci://evidence", EvidenceDigest: "sha256:" + strings.Repeat("b", 64)}}}}
}

func TestStorageDriverProviderFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	request := engine.StorageDriverRequest{Action: "upgrade", PackageID: "storage.example/driver", PackageVersion: "1.0.0", CurrentVersion: "0.9.0", KubernetesVersion: "1.32.0"}
	for _, test := range []struct {
		name   string
		mutate func(*fakeStore, *engine.StorageDriverRequest)
	}{
		{"unknown package", func(st *fakeStore, req *engine.StorageDriverRequest) { delete(st.manifests, req.PackageID) }},
		{"unlicensed", func(st *fakeStore, req *engine.StorageDriverRequest) {
			st.manifests[req.PackageID].Compatibility["licensed"] = false
		}},
		{"unhealthy", func(st *fakeStore, req *engine.StorageDriverRequest) {
			st.manifests[req.PackageID].Compatibility["healthy"] = false
		}},
		{"incompatible Kubernetes", func(_ *fakeStore, req *engine.StorageDriverRequest) { req.KubernetesVersion = "1.35.0" }},
		{"undeclared upgrade", func(_ *fakeStore, req *engine.StorageDriverRequest) { req.CurrentVersion = "0.8.0" }},
		{"expired package evidence", func(st *fakeStore, req *engine.StorageDriverRequest) {
			st.manifests[req.PackageID].StorageDriverPackage.ConformanceEvidence[0].ExpiresAt = now.Add(-time.Minute)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			localRequest := request
			st := newFakeStore()
			st.manifests[localRequest.PackageID] = eligibleStorageDriverManifest(now)
			test.mutate(st, &localRequest)
			_, err := (lifecycleProviderResolver{store: st, now: func() time.Time { return now }}).ResolveStorageDriverProvider(context.Background(), localRequest)
			if err == nil {
				t.Fatal("ineligible package was resolved")
			}
		})
	}
}

func TestStorageDriverProviderPinsManifestAuthority(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.manifests["storage.example/driver"] = eligibleStorageDriverManifest(now)
	resolved, err := (lifecycleProviderResolver{store: st, now: func() time.Time { return now }}).ResolveStorageDriverProvider(context.Background(), engine.StorageDriverRequest{Action: "upgrade", PackageID: "storage.example/driver", PackageVersion: "1.0.0", CurrentVersion: "0.9.0", KubernetesVersion: "1.32.0"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.PackageDigest == "" || resolved.RollbackVersion != "0.9.0" || resolved.CapabilityClaims["expansion"] != "Supported" {
		t.Fatalf("manifest authority not pinned: %+v", resolved)
	}
}

func TestRetainedVolumeProviderRequiresExactCurrentConformanceEvidence(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	manifest := &store.ProviderManifest{ProviderID: "storage.example/sanitizer", Version: "1.0.0", ProtocolVersion: "2.0.0", Actions: []string{"storage.retained-volume.sanitize"}, ConformanceLevel: "production_ready", ConformanceExpiresAt: &expires, ConformanceEvidence: []store.ConformanceEvidence{{Category: "storage-retained-volume", TestName: "storage.retained-volume.sanitize", Passed: true, EvidenceRef: "evidence://sanitize-v1"}}}
	st := newFakeStore()
	st.manifests[manifest.ProviderID] = manifest
	resolver := lifecycleProviderResolver{store: st, now: func() time.Time { return now }}
	resolved, err := resolver.ResolveRetainedVolumeProvider(context.Background(), engine.RetainedVolumeProviderRequest{Action: "sanitize", ProviderID: manifest.ProviderID})
	if err != nil || resolved.EvidenceRef == "" || resolved.ProviderDigest == "" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	manifest.ConformanceEvidence = nil
	if _, err := resolver.ResolveRetainedVolumeProvider(context.Background(), engine.RetainedVolumeProviderRequest{Action: "sanitize", ProviderID: manifest.ProviderID}); err == nil {
		t.Fatal("provider without sanitization evidence was accepted")
	}
}
