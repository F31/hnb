package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

func TestSessionBootstrapReturnsTrustedContextContract(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session/bootstrap", nil)
	req = req.WithContext(iam.WithTrustedContext(req.Context(), iam.TrustedContext{SubjectID: "u1", TenantID: "tenant-a", MembershipID: "m1", PolicyVersion: "p1", ScopedPermissions: []iam.ScopedPermission{{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionRead}}}))
	recorder := httptest.NewRecorder()

	NewSessionHandler("http://platform-api:8080").Bootstrap(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["selectedTenantId"] != "tenant-a" || body["permissionVersion"] != "p1" {
		t.Fatalf("unexpected bootstrap body: %#v", body)
	}
}
