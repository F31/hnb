package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	schemaapp "github.com/F31/hnb/cmd/apiserver/internal/application/schema"
	"github.com/F31/hnb/pkg/iam"
)

func TestSchemaPageReturnsTrustedEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema/page/cluster-list", nil)
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Page(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var got schemaapp.Page
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.APIVersion != "ui.hnb.io/v1" || got.Kind != "PageSchema" || got.Metadata.ID != "cluster-list" {
		t.Fatalf("unexpected schema page: %+v", got)
	}
	if len(got.Spec.Endpoints) == 0 || len(got.Spec.DataSources) == 0 || len(got.Spec.Actions) == 0 || len(got.Spec.Regions) == 0 {
		t.Fatalf("schema page missing runtime declarations: %+v", got.Spec)
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"private_key", "kubeconfig", "access_token", "https://", "http://"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("schema response contains forbidden fragment %q: %s", forbidden, body)
		}
	}
}

func TestSchemaPageNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema/page/missing", nil)
	req.SetPathValue("id", "missing")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Page(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPageForbiddenWithoutSchemaRead(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema/page/cluster-list", nil)
	req.SetPathValue("id", "cluster-list")
	trusted := schemaTrustedContext()
	trusted.ScopedPermissions = nil
	req = req.WithContext(iam.WithTrustedContext(req.Context(), trusted))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Page(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPageNotModifiedWithMatchingEtag(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema/page/cluster-list", nil)
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	req.Header.Set("If-None-Match", "page-cluster-list-r1")
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Page(recorder, req)

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSchemaPageServesEtagHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/schema/page/cluster-list", nil)
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Page(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if etag := recorder.Header().Get("ETag"); etag != "page-cluster-list-r1" {
		t.Fatalf("etag = %q", etag)
	}
}

func TestSchemaPagePublishForbiddenWithoutUpdate(t *testing.T) {
	body := strings.NewReader(`{"apiVersion":"ui.hnb.io/v1","kind":"PageSchema","metadata":{"id":"cluster-list"},"spec":{"template":"list","regions":[{"id":"r1","componentType":"DataTable"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/pages/cluster-list/publish", body)
	req.SetPathValue("id", "cluster-list")
	trusted := schemaTrustedContext()
	trusted.ScopedPermissions = nil
	req = req.WithContext(iam.WithTrustedContext(req.Context(), trusted))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Publish(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPagePublishRejectsInvalidEnvelope(t *testing.T) {
	body := strings.NewReader(`{"apiVersion":"ui.hnb.io/v1","kind":"WrongKind","metadata":{"id":"cluster-list"},"spec":{"template":"list","regions":[{"id":"r1","componentType":"DataTable"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/pages/cluster-list/publish", body)
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Publish(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPagePublishRejectsBodyPathMismatch(t *testing.T) {
	body := strings.NewReader(`{"apiVersion":"ui.hnb.io/v1","kind":"PageSchema","metadata":{"id":"other.page"},"spec":{"template":"list","regions":[{"id":"r1","componentType":"DataTable"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/pages/cluster-list/publish", body)
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Publish(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPageRollbackForbiddenWithoutUpdate(t *testing.T) {
	body := strings.NewReader(`{"revision":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/pages/cluster-list/rollback", body)
	req.SetPathValue("id", "cluster-list")
	trusted := schemaTrustedContext()
	trusted.ScopedPermissions = nil
	req = req.WithContext(iam.WithTrustedContext(req.Context(), trusted))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Rollback(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSchemaPageRollbackRejectsInvalidBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/pages/cluster-list/rollback", strings.NewReader(`not-json`))
	req.SetPathValue("id", "cluster-list")
	req = req.WithContext(iam.WithTrustedContext(req.Context(), schemaTrustedContext()))
	recorder := httptest.NewRecorder()

	NewSchemaHandler().Rollback(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func schemaTrustedContext() iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", TenantID: "tenant-a", PolicyVersion: "p1", ScopedPermissions: []iam.ScopedPermission{
		{TenantID: "tenant-a", ResourceKind: string(iam.ResourceSchema), Action: iam.ActionRead},
		{TenantID: "tenant-a", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionList},
	}}
}
