package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type fakeStorageDesiredStore struct {
	tenantID            string
	backendInput        storageBackendInput
	backend             storageBackendRecord
	updateErr           error
	createCalls         int
	bindingCreateCalls  int
	offeringCreateCalls int
	offering            workloadStorageOfferingRecord
	binding             storageClassBindingRecord
	getBindingTenant    string
	secretValidationErr error
	validatedSecret     *secretReference
}

func (f *fakeStorageDesiredStore) ListBackends(_ context.Context, tenantID string) ([]storageBackendRecord, error) {
	f.tenantID = tenantID
	return []storageBackendRecord{f.backend}, nil
}
func (f *fakeStorageDesiredStore) GetBackend(context.Context, string, string) (storageBackendRecord, error) {
	return f.backend, nil
}
func (f *fakeStorageDesiredStore) CreateBackend(_ context.Context, tenantID, id string, input storageBackendInput) (storageBackendRecord, error) {
	f.createCalls++
	f.tenantID, f.backendInput = tenantID, input
	item := f.backend
	item.ID, item.TenantID, item.ProviderType, item.BackendID, item.DisplayName = id, tenantID, input.ProviderType, input.BackendID, input.DisplayName
	item.SecretReference, item.Version = input.SecretReference, 1
	item.CreatedAt, item.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	return item, nil
}
func (f *fakeStorageDesiredStore) UpdateBackend(context.Context, string, string, int64, storageBackendInput) (storageBackendRecord, error) {
	return f.backend, f.updateErr
}
func (f *fakeStorageDesiredStore) DeleteBackend(context.Context, string, string, int64) error {
	return nil
}
func (f *fakeStorageDesiredStore) ValidateSecretReference(_ context.Context, _ string, ref secretReference) error {
	f.validatedSecret = &ref
	return f.secretValidationErr
}
func (f *fakeStorageDesiredStore) ListOfferings(context.Context, string) ([]workloadStorageOfferingRecord, error) {
	return nil, nil
}
func (f *fakeStorageDesiredStore) GetOffering(context.Context, string, string) (workloadStorageOfferingRecord, error) {
	return f.offering, nil
}
func (f *fakeStorageDesiredStore) CreateOffering(context.Context, string, string, workloadStorageOfferingInput) (workloadStorageOfferingRecord, error) {
	f.offeringCreateCalls++
	return workloadStorageOfferingRecord{}, nil
}
func (f *fakeStorageDesiredStore) UpdateOffering(context.Context, string, string, int64, workloadStorageOfferingInput) (workloadStorageOfferingRecord, error) {
	return workloadStorageOfferingRecord{}, nil
}
func (f *fakeStorageDesiredStore) DeleteOffering(context.Context, string, string, int64) error {
	return nil
}
func (f *fakeStorageDesiredStore) ListBindings(context.Context, string, string) ([]storageClassBindingRecord, error) {
	return nil, nil
}
func (f *fakeStorageDesiredStore) GetBinding(_ context.Context, tenantID, _ string) (storageClassBindingRecord, error) {
	f.getBindingTenant = tenantID
	return f.binding, nil
}
func (f *fakeStorageDesiredStore) CreateBinding(context.Context, string, string, string, storageClassBindingInput) (storageClassBindingRecord, error) {
	f.bindingCreateCalls++
	return storageClassBindingRecord{}, nil
}
func (f *fakeStorageDesiredStore) UpdateBinding(context.Context, string, string, int64, storageClassBindingInput) (storageClassBindingRecord, error) {
	return storageClassBindingRecord{}, nil
}
func (f *fakeStorageDesiredStore) DeleteBinding(context.Context, string, string, int64) error {
	return nil
}

func TestCreateStorageBackendUsesTrustedTenantAndSecretReference(t *testing.T) {
	store := &fakeStorageDesiredStore{}
	handler := NewStorageDesiredHandler(store)
	req := storageRequest(http.MethodPost, "/api/v1/storage/backends")
	req.Header.Set("Idempotency-Key", "create-backend-a")
	req.Body = ioBody(`{"providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","secretReference":{"provider":"platform-secret","scope":"tenant:tenant-a","name":"array-a"},"attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate"}}`)
	recorder := httptest.NewRecorder()
	handler.CreateBackend(recorder, req)
	if recorder.Code != http.StatusCreated || recorder.Header().Get("ETag") != `"1"` || store.tenantID != "tenant-a" || store.createCalls != 1 {
		t.Fatalf("status=%d etag=%q tenant=%q calls=%d body=%s", recorder.Code, recorder.Header().Get("ETag"), store.tenantID, store.createCalls, recorder.Body.String())
	}
	if store.backendInput.SecretReference == nil || store.backendInput.SecretReference.Name != "array-a" || store.validatedSecret == nil || strings.Contains(recorder.Body.String(), "secretValue") {
		t.Fatalf("unexpected secret handling: input=%+v body=%s", store.backendInput.SecretReference, recorder.Body.String())
	}
}

func TestCreateStorageBackendRejectsSecretValuesAndClientTenant(t *testing.T) {
	for name, body := range map[string]string{
		"secret value":           `{"providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","secretReference":{"provider":"platform-secret","scope":"tenant:tenant-a","name":"array-a","value":"raw-secret"},"attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate"}}`,
		"client tenant":          `{"tenantId":"tenant-b","providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate"}}`,
		"other tenant secret":    `{"providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","secretReference":{"provider":"platform-secret","scope":"tenant:tenant-b","name":"array-a"},"attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate"}}`,
		"unknown attribute":      `{"providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate","script":"alert(1)"}}`,
		"unknown schema version": `{"providerType":"generic-csi","providerSchemaVersion":"2.0.0","backendId":"array-a","displayName":"Array A","attributes":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStorageDesiredStore{}
			handler := NewStorageDesiredHandler(store)
			req := storageRequest(http.MethodPost, "/api/v1/storage/backends")
			req.Header.Set("Idempotency-Key", "invalid-backend")
			req.Body = ioBody(body)
			recorder := httptest.NewRecorder()
			handler.CreateBackend(recorder, req)
			if recorder.Code != http.StatusBadRequest || store.createCalls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, store.createCalls, recorder.Body.String())
			}
		})
	}
}

func TestCreateStorageBackendRejectsUnknownTenantSecretReference(t *testing.T) {
	store := &fakeStorageDesiredStore{secretValidationErr: errStorageInvalidRef}
	req := storageRequest(http.MethodPost, "/api/v1/storage/backends")
	req.Header.Set("Idempotency-Key", "unknown-secret")
	req.Body = ioBody(`{"providerType":"nfs","providerSchemaVersion":"1.0.0","backendId":"nfs-a","displayName":"NFS A","secretReference":{"provider":"platform-secrets","scope":"tenant:tenant-a","name":"missing"},"attributes":{"server":"nfs.internal","exportPath":"/workloads"}}`)
	recorder := httptest.NewRecorder()
	NewStorageDesiredHandler(store).CreateBackend(recorder, req)
	if recorder.Code != http.StatusBadRequest || store.createCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, store.createCalls, recorder.Body.String())
	}
}

func TestUpdateStorageBackendRequiresAndChecksVersion(t *testing.T) {
	store := &fakeStorageDesiredStore{updateErr: errStorageVersionConflict}
	handler := NewStorageDesiredHandler(store)
	body := `{"providerType":"generic-csi","providerSchemaVersion":"1.0.0","backendId":"array-a","displayName":"Array A","attributes":{"provisioner":"example.csi.io","volumeBindingMode":"Immediate"}}`
	for name, ifMatch := range map[string]struct {
		value  string
		status int
	}{
		"missing": {"", http.StatusPreconditionRequired},
		"stale":   {`"1"`, http.StatusPreconditionFailed},
	} {
		t.Run(name, func(t *testing.T) {
			req := storageRequest(http.MethodPut, "/api/v1/storage/backends/32684d2c-fca8-4f28-a946-fb267363fd6c")
			req.SetPathValue("backendId", "32684d2c-fca8-4f28-a946-fb267363fd6c")
			req.Header.Set("If-Match", ifMatch.value)
			req.Header.Set("Idempotency-Key", "update-backend")
			req.Body = ioBody(body)
			recorder := httptest.NewRecorder()
			handler.UpdateBackend(recorder, req)
			if recorder.Code != ifMatch.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStorageProviderSchemasExposeOnlyTrustedDeclarativeForms(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewStorageDesiredHandler(&fakeStorageDesiredStore{}).ProviderSchemas(recorder, storageRequest(http.MethodGet, "/api/v1/storage/provider-schemas"))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, storageBackendComponentType) || strings.Contains(body, "http://") || strings.Contains(body, "https://") || strings.Contains(strings.ToLower(body), "javascript") {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
}

func TestStorageBackendListCannotSelectAnotherTenant(t *testing.T) {
	store := &fakeStorageDesiredStore{backend: storageBackendRecord{ID: "32684d2c-fca8-4f28-a946-fb267363fd6c", TenantID: "tenant-a", Version: 1}}
	recorder := httptest.NewRecorder()
	NewStorageDesiredHandler(store).Backends(recorder, storageRequest(http.MethodGet, "/api/v1/storage/backends"))
	if recorder.Code != http.StatusOK || store.tenantID != "tenant-a" {
		t.Fatalf("status=%d tenant=%q", recorder.Code, store.tenantID)
	}
}

func TestStorageDesiredAPIRejectsObjectAndArtifactSemantics(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		invoke func(*StorageDesiredHandler, http.ResponseWriter, *http.Request)
	}{
		{
			name: "object bucket offering", path: "/api/v1/storage/offerings",
			body:   `{"name":"artifacts","consumptionModel":"ObjectBucket","serviceMode":"File","accessModes":["ReadWriteMany"],"volumeExpansion":"Unknown","snapshots":"Unknown","clones":"Unknown","protectionClass":"standard"}`,
			invoke: (*StorageDesiredHandler).CreateOffering,
		},
		{
			name: "artifact profile field", path: "/api/v1/storage/offerings",
			body:   `{"name":"artifacts","consumptionModel":"KubernetesPersistentVolume","serviceMode":"File","accessModes":["ReadWriteMany"],"volumeExpansion":"Unknown","snapshots":"Unknown","clones":"Unknown","protectionClass":"standard","artifactStorageProfileId":"018f6c2a-4a64-7b58-9cc3-9f70462f4201"}`,
			invoke: (*StorageDesiredHandler).CreateOffering,
		},
		{
			name: "object bucket binding", path: "/api/v1/storage/offerings/72000000-0000-0000-0000-000000000001/bindings",
			body:   `{"offeringVersion":1,"targetId":"71000000-0000-0000-0000-000000000001","bindingTarget":"ObjectBucket","storageClassName":"artifacts","storageClassUid":"bucket-a","storageClassResourceVersion":"1","syncState":"Active","isDefault":false,"source":"object-api","observedAt":"2026-08-10T00:00:00Z","freshness":"Fresh"}`,
			invoke: (*StorageDesiredHandler).CreateBinding,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			store := &fakeStorageDesiredStore{}
			handler := NewStorageDesiredHandler(store)
			req := storageRequest(http.MethodPost, testCase.path)
			req.SetPathValue("offeringId", "72000000-0000-0000-0000-000000000001")
			req.Header.Set("Idempotency-Key", "boundary-test")
			req.Body = ioBody(testCase.body)
			recorder := httptest.NewRecorder()
			testCase.invoke(handler, recorder, req)
			if recorder.Code != http.StatusBadRequest || store.offeringCreateCalls != 0 || store.bindingCreateCalls != 0 {
				t.Fatalf("status=%d offeringCalls=%d bindingCalls=%d body=%s", recorder.Code, store.offeringCreateCalls, store.bindingCreateCalls, recorder.Body.String())
			}
		})
	}
}

func TestStorageDesiredModelsRequireOrdinaryVolumeSemantics(t *testing.T) {
	offering := workloadStorageOfferingInput{Name: "fast", ConsumptionModel: "KubernetesPersistentVolume", ServiceMode: "Block", AccessModes: []string{"ReadWriteOnce"}, VolumeExpansion: "Unknown", Snapshots: "Unknown", Clones: "Unknown", ProtectionClass: "standard"}
	if !validOfferingInput(offering) {
		t.Fatal("ordinary persistent-volume offering was rejected")
	}
	offering.ConsumptionModel = "ArtifactStorageProfile"
	if validOfferingInput(offering) {
		t.Fatal("ArtifactStorageProfile was accepted as a workload offering")
	}
	binding := storageClassBindingInput{OfferingVersion: 1, TargetID: "71000000-0000-0000-0000-000000000001", BindingTarget: "KubernetesStorageClass", StorageClassName: "fast", StorageClassUID: "uid", StorageClassResourceVersion: "1", SyncState: "Active", Source: "runtime-target-agent", ObservedAt: time.Now(), Freshness: "Fresh"}
	if !validBindingInput(binding) {
		t.Fatal("Kubernetes StorageClass binding was rejected")
	}
	binding.BindingTarget = "ObjectBucket"
	if validBindingInput(binding) {
		t.Fatal("object bucket was accepted as a StorageClass binding")
	}
}

func TestImportStorageBindingUsesDedicatedOperationPathWithoutMutationOrProxy(t *testing.T) {
	var forwarded bffIntentEnvelope
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" || strings.Contains(r.URL.Path, "proxy") {
			t.Fatalf("unexpected upstream path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&forwarded); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"intentId":"intent-a","operationId":"operation-a","planId":"plan-a","status":"queued","createdAt":"2026-08-10T00:00:00Z","semanticDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","replayed":false}`))
	}))
	defer platform.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signer, err := iam.NewDelegationSigner(iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}, resourceDelegationKeys{key: key})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStorageDesiredStore{offering: workloadStorageOfferingRecord{ID: "72000000-0000-0000-0000-000000000001", TenantID: "tenant-a", Version: 7}}
	handler := NewStorageDesiredHandler(store)
	handler.ConfigureOperations(platform.URL, signer)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/storage/offerings/72000000-0000-0000-0000-000000000001/bindings/intents/import", strings.NewReader(`{"targetId":"71000000-0000-0000-0000-000000000001","targetVersion":12,"storageClassName":"fast","storageClassUid":"sc-uid-a","storageClassResourceVersion":"1843"}`)))
	req.SetPathValue("offeringId", "72000000-0000-0000-0000-000000000001")
	req.Header.Set("Idempotency-Key", "import-fast")
	recorder := httptest.NewRecorder()
	handler.ImportBindingIntent(recorder, req)
	if recorder.Code != http.StatusAccepted || store.bindingCreateCalls != 0 {
		t.Fatalf("status=%d mutations=%d body=%s", recorder.Code, store.bindingCreateCalls, recorder.Body.String())
	}
	if forwarded.Kind != "ImportStorageClassBinding" || forwarded.Spec.OfferingVersion != 7 || forwarded.Spec.TargetID != "71000000-0000-0000-0000-000000000001" || forwarded.Spec.StorageClassUID != "sc-uid-a" {
		t.Fatalf("unfixed forwarded intent: %#v", forwarded)
	}
	if forwarded.Spec.Parameters != nil || len(forwarded.Spec.SecretReferences) != 0 || strings.Contains(recorder.Body.String(), "success") {
		t.Fatalf("unsanitized or synchronous-success response: %s", recorder.Body.String())
	}
	for _, reference := range []string{`"executionPlanId":"plan-a"`, `"operationId":"operation-a"`} {
		if !strings.Contains(recorder.Body.String(), reference) {
			t.Fatalf("missing %s: %s", reference, recorder.Body.String())
		}
	}
}

func TestReconcileStorageBindingPinsDesiredIdentityAndRejectsStaleBinding(t *testing.T) {
	binding := storageClassBindingRecord{ID: "73000000-0000-0000-0000-000000000001", TenantID: "tenant-a", OfferingID: "72000000-0000-0000-0000-000000000001", OfferingVersion: 7, TargetID: "71000000-0000-0000-0000-000000000001", StorageClassName: "fast", StorageClassUID: "desired-uid", StorageClassResourceVersion: "1843", Freshness: "Fresh", Version: 3}
	forwarded := bffIntentEnvelope{}
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" {
			t.Fatalf("generic proxy bypass: %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&forwarded)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"intentId":"intent-r","operationId":"operation-r","planId":"plan-r","status":"queued","createdAt":"2026-08-10T00:00:00Z","semanticDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","replayed":true}`))
	}))
	defer platform.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signer, _ := iam.NewDelegationSigner(iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}, resourceDelegationKeys{key: key})
	store := &fakeStorageDesiredStore{binding: binding}
	handler := NewStorageDesiredHandler(store)
	handler.ConfigureOperations(platform.URL, signer)
	request := func(version string) *httptest.ResponseRecorder {
		req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/storage/bindings/"+binding.ID+"/intents/reconcile", strings.NewReader(`{"targetVersion":12}`)))
		req.SetPathValue("bindingId", binding.ID)
		req.Header.Set("If-Match", version)
		req.Header.Set("Idempotency-Key", "reconcile-fast")
		recorder := httptest.NewRecorder()
		handler.ReconcileBindingIntent(recorder, req)
		return recorder
	}
	if recorder := request(`"2"`); recorder.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	accepted := request(`"3"`)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	if forwarded.Kind != "ReconcileStorageClassBinding" || forwarded.Spec.BindingVersion != 3 || forwarded.Spec.StorageClassUID != "desired-uid" || forwarded.Spec.StorageClassResourceVersion != "1843" || forwarded.Spec.ExpectedVersion != 12 {
		t.Fatalf("stored identity was not pinned: %#v", forwarded)
	}
	if store.bindingCreateCalls != 0 || store.getBindingTenant != "tenant-a" || !strings.Contains(accepted.Body.String(), `"replayed":true`) {
		t.Fatalf("handler synchronously mutated binding")
	}
}

func TestReconcileStorageBindingRejectsUnknownOrStaleObservation(t *testing.T) {
	base := storageClassBindingRecord{ID: "73000000-0000-0000-0000-000000000001", OfferingID: "72000000-0000-0000-0000-000000000001", OfferingVersion: 1, TargetID: "71000000-0000-0000-0000-000000000001", StorageClassName: "fast", Version: 1}
	for name, binding := range map[string]storageClassBindingRecord{
		"unknown": base,
		"stale": func() storageClassBindingRecord {
			value := base
			value.Freshness = "Stale"
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStorageDesiredStore{binding: binding}
			req := storageRequest(http.MethodPost, "/api/v1/storage/bindings/"+binding.ID+"/intents/reconcile")
			req.SetPathValue("bindingId", binding.ID)
			req.Header.Set("If-Match", `"1"`)
			req.Header.Set("Idempotency-Key", "reconcile-fast")
			req.Body = ioBody(`{"targetVersion":12}`)
			recorder := httptest.NewRecorder()
			NewStorageDesiredHandler(store).ReconcileBindingIntent(recorder, req)
			if recorder.Code != http.StatusConflict || store.getBindingTenant != "tenant-a" {
				t.Fatalf("status=%d tenant=%q body=%s", recorder.Code, store.getBindingTenant, recorder.Body.String())
			}
		})
	}
}

func TestStorageDriverLifecycleDelegatesToOperationWithoutSynchronousInstall(t *testing.T) {
	var forwarded bffIntentEnvelope
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" || strings.Contains(r.URL.Path, "proxy") {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&forwarded); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"intentId":"intent-d","operationId":"operation-d","planId":"plan-d","status":"queued","createdAt":"2026-08-10T00:00:00Z","semanticDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","replayed":false}`))
	}))
	defer platform.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signer, _ := iam.NewDelegationSigner(iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}, resourceDelegationKeys{key: key})
	handler := NewStorageDesiredHandler(&fakeStorageDesiredStore{})
	handler.ConfigureOperations(platform.URL, signer)
	installationID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("hnb.storage-driver-installation\x00tenant-a\x0071000000-0000-0000-0000-000000000001\x00storage.example/driver")).String()
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/storage/driver-installations/"+installationID+"/intents/upgrade", strings.NewReader(`{"targetId":"71000000-0000-0000-0000-000000000001","targetVersion":12,"packageId":"storage.example/driver","packageVersion":"1.0.0","currentVersion":"0.9.0","kubernetesVersion":"1.32.0","parameters":{"mode":"safe"},"secretReferences":[{"provider":"vault","scope":"tenant:tenant-a","name":"driver-secret"}]}`)))
	req.SetPathValue("installationId", installationID)
	req.Header.Set("Idempotency-Key", "upgrade-driver")
	recorder := httptest.NewRecorder()
	handler.UpgradeDriverIntent(recorder, req)
	if recorder.Code != http.StatusAccepted || forwarded.Kind != "UpgradeStorageDriver" || forwarded.Spec.InstallationID != installationID || forwarded.Spec.CurrentVersion != "0.9.0" {
		t.Fatalf("status=%d intent=%#v body=%s", recorder.Code, forwarded, recorder.Body.String())
	}
	if forwarded.Spec.SecretReferences[0].Name != "driver-secret" || strings.Contains(recorder.Body.String(), "driver-secret") || !strings.Contains(recorder.Body.String(), `"operationId":"operation-d"`) {
		t.Fatalf("secret leaked or operation missing: %s", recorder.Body.String())
	}
}

func TestStorageBindingResponseExposesProjectedDriftCondition(t *testing.T) {
	now := time.Now().UTC()
	response := bindingResponse(storageClassBindingRecord{ID: "binding", Conditions: []map[string]any{{"type": "Drifted", "status": "True", "reason": "StorageClassUIDChanged", "freshness": "Fresh"}}, ObservedAt: now, CreatedAt: now, UpdatedAt: now})
	conditions, ok := response["conditions"].([]map[string]any)
	if !ok || len(conditions) != 1 || conditions[0]["reason"] != "StorageClassUIDChanged" {
		t.Fatalf("conditions=%#v", response["conditions"])
	}
}

func TestRetainedVolumeHandlerRequiresApprovalAndDependencyClearance(t *testing.T) {
	base := `{"targetId":"71000000-0000-0000-0000-000000000001","targetVersion":12,"workflowProviderRef":"storage.example/sanitizer","persistentVolume":{"name":"pv-a","uid":"pv-uid","resourceVersion":"9","phase":"Released","reclaimPolicy":"Retain"},"persistentVolumeClaim":{"namespace":"ns-a","name":"claim-a","uid":"pvc-uid","resourceVersion":"8","deletionObserved":true},"podDependencies":[],"statefulSetDependencies":[],"approvalAcknowledged":true}`
	for name, body := range map[string]string{
		"approval missing":     strings.Replace(base, `"approvalAcknowledged":true`, `"approvalAcknowledged":false`, 1),
		"wrong reclaim policy": strings.Replace(base, `"reclaimPolicy":"Retain"`, `"reclaimPolicy":"Delete"`, 1),
		"pod dependency":       strings.Replace(base, `"podDependencies":[]`, `"podDependencies":[{"namespace":"ns-a","name":"pod-a","uid":"pod-uid","resourceVersion":"1"}]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			req := storageRequest(http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/sanitize")
			req.SetPathValue("volumeId", "volume-a")
			req.Header.Set("Idempotency-Key", "sanitize-a")
			req.Body = ioBody(body)
			recorder := httptest.NewRecorder()
			NewStorageDesiredHandler(&fakeStorageDesiredStore{}).SanitizeRetainedVolumeIntent(recorder, req)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRetainedVolumeHandlerSubmitsOnlyTypedProviderWorkflow(t *testing.T) {
	var forwarded bffIntentEnvelope
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&forwarded)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"intentId":"intent-v","operationId":"operation-v","planId":"plan-v","status":"pending_approval","createdAt":"2026-08-10T00:00:00Z","semanticDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","replayed":false}`))
	}))
	defer platform.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	signer, _ := iam.NewDelegationSigner(iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}, resourceDelegationKeys{key: key})
	handler := NewStorageDesiredHandler(&fakeStorageDesiredStore{})
	handler.ConfigureOperations(platform.URL, signer)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/sanitize", strings.NewReader(`{"targetId":"71000000-0000-0000-0000-000000000001","targetVersion":12,"workflowProviderRef":"storage.example/sanitizer","persistentVolume":{"name":"pv-a","uid":"pv-uid","resourceVersion":"9","phase":"Released","reclaimPolicy":"Retain"},"persistentVolumeClaim":{"namespace":"ns-a","name":"claim-a","uid":"pvc-uid","resourceVersion":"8","deletionObserved":true},"podDependencies":[],"statefulSetDependencies":[],"approvalAcknowledged":true}`)))
	req.SetPathValue("volumeId", "volume-a")
	req.Header.Set("Idempotency-Key", "sanitize-a")
	recorder := httptest.NewRecorder()
	handler.SanitizeRetainedVolumeIntent(recorder, req)
	encoded, _ := json.Marshal(forwarded)
	if recorder.Code != http.StatusAccepted || forwarded.Kind != "SanitizeRetainedVolume" || forwarded.Spec.WorkflowProviderRef != "storage.example/sanitizer" || strings.Contains(string(encoded), "claimRef") {
		t.Fatalf("status=%d intent=%s body=%s", recorder.Code, encoded, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"operationCommitted"`) || strings.Contains(strings.ToLower(recorder.Body.String()), "sanitized") {
		t.Fatalf("unsafe result: %s", recorder.Body.String())
	}
}

func ioBody(value string) io.ReadCloser { return io.NopCloser(strings.NewReader(value)) }
