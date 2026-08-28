package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

const storageTestTarget = "32684d2c-fca8-4f28-a946-fb267363fd6c"

type fakeStorageStore struct {
	overview      storageOverviewProjection
	overviewErr   error
	owned         bool
	ownedErr      error
	rows          []storageProjectionRow
	registrations map[string]bool
	snapshot      *storageSnapshotProjection
	inventoryErr  error
	filter        storageInventoryQuery
	metrics       []storageMetricProjection
	metricsErr    error
}

func (f *fakeStorageStore) Metrics(context.Context, string, string) ([]storageMetricProjection, error) {
	return f.metrics, f.metricsErr
}

func (f *fakeStorageStore) Overview(context.Context, string) (storageOverviewProjection, error) {
	return f.overview, f.overviewErr
}
func (f *fakeStorageStore) TargetOwned(context.Context, string, string) (bool, error) {
	return f.owned, f.ownedErr
}
func (f *fakeStorageStore) Inventory(_ context.Context, _, _ string, filter storageInventoryQuery) ([]storageProjectionRow, map[string]bool, *storageSnapshotProjection, error) {
	f.filter = filter
	return f.rows, f.registrations, f.snapshot, f.inventoryErr
}

func storageRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	trusted := iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", PolicyVersion: "default:1"}
	return req.WithContext(iam.WithTrustedContext(req.Context(), trusted))
}

func TestStorageDesiredStateListsAreContractShapedAndEmpty(t *testing.T) {
	h := NewStorageHandler(&fakeStorageStore{})
	for name, invoke := range map[string]func(http.ResponseWriter, *http.Request){
		"backends": h.Backends, "offerings": h.Offerings, "driver installations": h.DriverInstallations,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			invoke(recorder, storageRequest(http.MethodGet, "/api/v1/storage/"+strings.ReplaceAll(name, " ", "-")))
			if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
			}
			for _, want := range []string{`"schemaVersion":"1.0.0"`, `"items":[]`, `"total":0`} {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body missing %s: %s", want, recorder.Body.String())
				}
			}
		})
	}
}

func TestStorageReadAPIsRequireTrustedTenant(t *testing.T) {
	h := NewStorageHandler(&fakeStorageStore{})
	for name, invoke := range map[string]func(http.ResponseWriter, *http.Request){
		"overview": h.Overview, "backends": h.Backends, "offerings": h.Offerings,
		"driver installations": h.DriverInstallations, "target inventory": h.TargetInventory,
		"offering bindings": h.OfferingBindings, "target metrics": h.TargetMetrics,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/storage/overview", nil)
			recorder := httptest.NewRecorder()
			invoke(recorder, req)
			if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("Content-Type") != "application/problem+json" ||
				!strings.Contains(recorder.Body.String(), `"code":"UNAUTHORIZED"`) {
				t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStorageMetricsUsesTenantTargetProjectionAndMarksExpiredRowsStale(t *testing.T) {
	store := &fakeStorageStore{owned: true, metrics: []storageMetricProjection{{
		ProviderID: "provider-a", ResourceKind: "StorageBackend", ResourceUID: "backend-a", StaleAfter: time.Now().Add(-time.Minute),
		Metrics: []byte(`[{"kind":"capacity","unit":"By","status":"Known","applicability":"Applicable","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh","value":1},{"kind":"usage","unit":"By","status":"NotReported","applicability":"Applicable","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh"},{"kind":"iops","unit":"1/s","status":"NotReported","applicability":"Unsupported","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh"},{"kind":"throughput","unit":"By/s","status":"NotReported","applicability":"Unsupported","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh"},{"kind":"latency","unit":"s","status":"NotReported","applicability":"Unsupported","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh"},{"kind":"health","unit":"1","status":"Known","applicability":"Applicable","source":"adapter-a","observedAt":"2026-08-14T00:00:00Z","freshness":"Fresh","value":1}]`),
	}}}
	req := storageRequest(http.MethodGet, "/api/v1/storage/targets/"+storageTestTarget+"/metrics")
	req.SetPathValue("targetId", storageTestTarget)
	recorder := httptest.NewRecorder()
	NewStorageHandler(store).TargetMetrics(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"providerId":"provider-a"`) ||
		strings.Count(recorder.Body.String(), `"freshness":"Stale"`) != 6 {
		t.Fatalf("unexpected metrics response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStorageInventoryUsesProjectionAndMissingDriverEvidence(t *testing.T) {
	observedAt := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	store := &fakeStorageStore{owned: true, registrations: map[string]bool{}, rows: []storageProjectionRow{{
		Kind: "StorageClass", UID: "sc-uid", ResourceVersion: "12", Name: "fast", DriverName: "missing.csi.example",
		Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt, StaleAfter: time.Now().Add(time.Hour),
		Attributes: []byte(`{"provisioner":"missing.csi.example","reclaimPolicy":"Retain","volumeBindingMode":"WaitForFirstConsumer","allowVolumeExpansion":true,"isDefault":false}`),
	}}}
	h := NewStorageHandler(store)
	req := storageRequest(http.MethodGet, "/api/v1/storage/targets/"+storageTestTarget+"/inventory?kind=StorageClass&page=2&pageSize=25")
	req.SetPathValue("targetId", storageTestTarget)
	recorder := httptest.NewRecorder()

	h.TargetInventory(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if store.filter.Kind != "StorageClass" || store.filter.Limit != 25 || store.filter.Offset != 25 {
		t.Fatalf("unexpected filter: %+v", store.filter)
	}
	for _, want := range []string{`"tenantId":"tenant-a"`, `"provisioner":"missing.csi.example"`, `"reason":"MissingDriverRegistration"`, `"csiDrivers":[]`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestStorageInventoryReportsStaleKnownAndNotReportedCapacity(t *testing.T) {
	observedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	known := int64(5368709120)
	store := &fakeStorageStore{owned: true, registrations: map[string]bool{}, rows: []storageProjectionRow{
		{Kind: "CSIStorageCapacity", UID: "capacity-known", ResourceVersion: "1", Name: "fast-zone-a", Namespace: "storage-system",
			Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt, StaleAfter: observedAt.Add(5 * time.Minute),
			Attributes: []byte(`{"storageClassName":"fast","capacityBytes":5368709120}`)},
		{Kind: "CSIStorageCapacity", UID: "capacity-unreported", ResourceVersion: "1", Name: "elastic-zone-b", Namespace: "storage-system",
			Source: "kubernetes.storage.k8s.io/v1", ObservedAt: observedAt, StaleAfter: observedAt.Add(5 * time.Minute),
			Attributes: []byte(`{"storageClassName":"elastic"}`)},
	}}
	h := NewStorageHandler(store)
	req := storageRequest(http.MethodGet, "/api/v1/storage/targets/"+storageTestTarget+"/inventory")
	req.SetPathValue("targetId", storageTestTarget)
	recorder := httptest.NewRecorder()

	h.TargetInventory(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{`"freshness":"Stale"`, `"status":"Known"`, `"value":` + strconv.FormatInt(known, 10),
		`"status":"NotReported"`, `"storageClassName":"elastic"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestStorageInventoryTargetOwnershipIsNonEnumerating(t *testing.T) {
	h := NewStorageHandler(&fakeStorageStore{owned: false})
	req := storageRequest(http.MethodGet, "/api/v1/storage/targets/"+storageTestTarget+"/inventory")
	req.SetPathValue("targetId", storageTestTarget)
	recorder := httptest.NewRecorder()
	h.TargetInventory(recorder, req)
	if recorder.Code != http.StatusNotFound || recorder.Header().Get("Content-Type") != "application/problem+json" ||
		!strings.Contains(recorder.Body.String(), `"code":"STORAGE_TARGET_NOT_FOUND"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStorageInvalidFilterReturnsStableProblem(t *testing.T) {
	h := NewStorageHandler(&fakeStorageStore{owned: true})
	req := storageRequest(http.MethodGet, "/api/v1/storage/targets/"+storageTestTarget+"/inventory?kind=Secret")
	req.Header.Set("X-Correlation-ID", "95ee3540-b162-4c05-b65c-7f909b29cb11")
	req.SetPathValue("targetId", storageTestTarget)
	recorder := httptest.NewRecorder()
	h.TargetInventory(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"INVALID_QUERY"`) ||
		!strings.Contains(recorder.Body.String(), `"correlationId":"95ee3540-b162-4c05-b65c-7f909b29cb11"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStorageOverviewDoesNotLeakStoreErrors(t *testing.T) {
	h := NewStorageHandler(&fakeStorageStore{overviewErr: errors.New("password=secret")})
	recorder := httptest.NewRecorder()
	h.Overview(recorder, storageRequest(http.MethodGet, "/api/v1/storage/overview"))
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "password") ||
		!strings.Contains(recorder.Body.String(), `"code":"STORAGE_PROJECTION_READ_FAILED"`) {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
