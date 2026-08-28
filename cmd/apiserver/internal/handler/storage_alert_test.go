package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/alert"
	"github.com/F31/hnb/pkg/iam"
)

type fakeStorageAlertStore struct {
	validationErr error
	created       *alert.StorageAlertRule
	tenant        string
}

func (f *fakeStorageAlertStore) ValidateStorageMetric(_ context.Context, ref alert.ResourceReference, _ alert.StorageMetricCondition) error {
	f.tenant = ref.TenantID
	return f.validationErr
}
func (f *fakeStorageAlertStore) ValidateChannelReferences(context.Context, string, []alert.ChannelReference) error {
	return nil
}
func (f *fakeStorageAlertStore) CreateStorageRule(_ context.Context, rule alert.StorageAlertRule) error {
	f.created = &rule
	return nil
}
func (f *fakeStorageAlertStore) ListStorageRules(_ context.Context, tenant string) ([]alert.StorageAlertRule, error) {
	f.tenant = tenant
	return nil, nil
}

func storageAlertRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/storage/alert-rules", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", "storage-alert-rule-a")
	trusted := iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a"}
	return req.WithContext(iam.WithTrustedContext(req.Context(), trusted))
}

func TestStorageAlertAPIRequiresIdempotencyKey(t *testing.T) {
	req := storageAlertRequest(`{}`)
	req.Header.Del("Idempotency-Key")
	recorder := httptest.NewRecorder()
	NewStorageAlertHandler(&fakeStorageAlertStore{}).CreateRule(recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"IDEMPOTENCY_KEY_REQUIRED"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStorageAlertAPIRejectsUnavailableIOPSBeforeSave(t *testing.T) {
	store := &fakeStorageAlertStore{validationErr: alert.ErrMetricUnavailable}
	recorder := httptest.NewRecorder()
	NewStorageAlertHandler(store).CreateRule(recorder, storageAlertRequest(`{
		"name":"NFS IOPS","severity":"warning","resource":{"targetId":"32684d2c-fca8-4f28-a946-fb267363fd6c","kind":"StorageBackend","uid":"backend-a"},
		"metric":{"providerId":"nfs","kind":"iops","unit":"1/s","source":"nfs_exporter","freshFor":"5m","operator":"gt","threshold":100},"duration":"5m"}`))
	if recorder.Code != http.StatusUnprocessableEntity || store.created != nil || store.tenant != "tenant-a" {
		t.Fatalf("status=%d tenant=%q created=%v body=%s", recorder.Code, store.tenant, store.created != nil, recorder.Body.String())
	}
}

func TestStorageAlertAPIRejectsClientTenant(t *testing.T) {
	store := &fakeStorageAlertStore{}
	recorder := httptest.NewRecorder()
	body := `{"name":"Cross tenant","severity":"warning","resource":{"tenantId":"tenant-b","targetId":"32684d2c-fca8-4f28-a946-fb267363fd6c","kind":"StorageBackend","uid":"backend-a"},"metric":{"providerId":"nfs","kind":"health","unit":"1","source":"nfs_exporter","freshFor":"5m","operator":"lt","threshold":1},"duration":"5m"}`
	NewStorageAlertHandler(store).CreateRule(recorder, storageAlertRequest(body))
	if recorder.Code != http.StatusBadRequest || store.created != nil || store.tenant != "" {
		t.Fatalf("status=%d tenant=%q body=%s", recorder.Code, store.tenant, recorder.Body.String())
	}
}

func TestStorageAlertAPIPreservesPVCNavigationAndDoesNotLeakSecrets(t *testing.T) {
	store := &fakeStorageAlertStore{}
	recorder := httptest.NewRecorder()
	body := `{
		"name":"PVC Pending","severity":"warning","resource":{"targetId":"32684d2c-fca8-4f28-a946-fb267363fd6c","kind":"PersistentVolumeClaim","uid":"pvc-uid-a","namespace":"payments","name":"ledger-data"},
		"metric":{"providerId":"kubernetes","kind":"health","unit":"1","source":"kube_state_metrics","freshFor":"5m","operator":"lt","threshold":1},"duration":"10m",
		"context":{"bindingId":"42684d2c-fca8-4f28-a946-fb267363fd6c","offeringId":"52684d2c-fca8-4f28-a946-fb267363fd6c","operationId":"62684d2c-fca8-4f28-a946-fb267363fd6c","runbookRef":"runbook://pvc-pending","navigationRef":"/container/storage/pvcs"},
		"channels":[{"type":"webhook","configReference":"channel-a","secretReference":{"provider":"platform-secrets","scope":"tenant:tenant-a","name":"storage-alert-hook","version":"3"}}]}`
	NewStorageAlertHandler(store).CreateRule(recorder, storageAlertRequest(body))
	response := recorder.Body.String()
	if recorder.Code != http.StatusCreated || store.created == nil || store.created.Resource.UID != "pvc-uid-a" || store.created.Context.OfferingID == "" {
		t.Fatalf("status=%d rule=%+v body=%s", recorder.Code, store.created, response)
	}
	for _, secret := range []string{"password", "token", "secretValue"} {
		if strings.Contains(response, secret) {
			t.Fatalf("response leaked %q: %s", secret, response)
		}
	}
}

func TestStorageAlertAPIRejectsInlineChannelSecret(t *testing.T) {
	store := &fakeStorageAlertStore{}
	recorder := httptest.NewRecorder()
	body := `{"name":"PVC Pending","severity":"warning","resource":{"targetId":"32684d2c-fca8-4f28-a946-fb267363fd6c","kind":"PersistentVolumeClaim","uid":"pvc-a"},"metric":{"providerId":"kubernetes","kind":"health","unit":"1","source":"kube_state_metrics","freshFor":"5m","operator":"lt","threshold":1},"duration":"5m","channels":[{"type":"webhook","configReference":"channel-a","secretReference":{"provider":"platform-secrets","scope":"tenant:tenant-a","name":"hook","value":"raw"}}]}`
	NewStorageAlertHandler(store).CreateRule(recorder, storageAlertRequest(body))
	if recorder.Code != http.StatusBadRequest || store.created != nil {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
