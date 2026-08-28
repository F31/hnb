package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

const testCorrelationID = "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"

func TestUpdateClusterDescriptionSucceeds(t *testing.T) {
	srv, st := newTestServer()
	id := newTargetID(t, st, "tenant-a")

	rec := patchDescription(t, srv, id, `{"description":"edge cluster"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := st.targets[id].Description; got != "edge cluster" {
		t.Fatalf("description = %q, want %q", got, "edge cluster")
	}
}

func TestUpdateClusterDescriptionClearsToEmpty(t *testing.T) {
	srv, st := newTestServer()
	id := newTargetID(t, st, "tenant-a")
	if err := st.UpdateRuntimeTargetDescription(t.Context(), id, "tenant-a", "old note"); err != nil {
		t.Fatalf("seed description: %v", err)
	}

	rec := patchDescription(t, srv, id, `{"description":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got := st.targets[id].Description; got != "" {
		t.Fatalf("description = %q, want empty", got)
	}
}

func TestUpdateClusterDescriptionMissingTargetIsNotFound(t *testing.T) {
	srv, _ := newTestServer()
	id := uuid.NewString()

	rec := patchDescription(t, srv, id, `{"description":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestUpdateClusterDescriptionTooLong(t *testing.T) {
	srv, st := newTestServer()
	id := newTargetID(t, st, "tenant-a")
	tooLong := strings.Repeat("a", maxClusterDescriptionLen+1)

	rec := patchDescription(t, srv, id, `{"description":"`+tooLong+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestUpdateClusterDescriptionWrongResourceKindIsUnauthorized(t *testing.T) {
	srv, st := newTestServer()
	id := newTargetID(t, st, "tenant-a")

	// A valid delegation scoped to a different resource kind (secret) passes
	// token verification but must be rejected by the handler's claim check.
	signer := testDelegationSignerFor(t, srv)
	token, err := signer.Sign(t.Context(), delegatedTrustedContext(), iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceSecret), ResourceID: id},
		Action:        iam.ActionUpdate,
		CorrelationID: testCorrelationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/clusters/"+id+"/description", strings.NewReader(`{"description":"x"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", testCorrelationID)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func patchDescription(t *testing.T, srv *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/clusters/"+id+"/description", strings.NewReader(body))
	setTestDescriptionDelegation(t, srv, req, id)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func newTargetID(t *testing.T, st *fakeStore, tenant string) string {
	t.Helper()
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{ID: id, TenantID: tenant, Name: id, TargetType: "kubernetes"}
	return id
}

func testDelegationSignerFor(t *testing.T, srv *Server) *iam.DelegationSigner {
	t.Helper()
	// setTestSecretDelegation configures the signer/verifier on first use.
	setTestSecretDelegation(t, srv, httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(`{}`)))
	value, ok := testDelegationSigners.Load(srv)
	if !ok {
		t.Fatal("delegation signer not configured")
	}
	return value.(*iam.DelegationSigner)
}

func delegatedTrustedContext() iam.TrustedContext {
	return iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", MembershipID: "membership-a",
		TenantID: "tenant-a", PolicyVersion: "default:2",
	}
}
