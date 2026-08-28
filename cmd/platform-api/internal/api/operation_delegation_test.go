package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

func signOperationDelegation(t *testing.T, signer *iam.DelegationSigner, correlationID string, action iam.AuthorizationAction, resourceID string) string {
	t.Helper()
	token, err := signer.Sign(nil, iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2",
	}, iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceOperation), ResourceID: resourceID},
		Action:        action,
		CorrelationID: correlationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// TestOperationDelegationListAndDetail verifies the BFF forwarding path: a
// trusted service delegation with operation scope reaches the tenant-scoped
// operation handlers.
func TestOperationDelegationListAndDetail(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keys := tokenTestKeys{key: key}
	config := iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}
	signer, _ := iam.NewDelegationSigner(config, keys)
	verifier, _ := iam.NewDelegationVerifier(config, keys)

	srv, st := newTestServer()
	srv.ConfigureIntentDelegation(verifier)

	opID := "356886f1-1b73-49a8-9275-1a85773cf973"
	st.ops[opID] = &store.Operation{ID: opID, TenantID: "tenant-a", OperationType: "upgrade", Status: "queued", TotalSteps: 2, CompletedSteps: 0}

	correlationID := "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"

	token := signOperationDelegation(t, signer, correlationID, iam.ActionList, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/operations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}

	token = signOperationDelegation(t, signer, correlationID, iam.ActionRead, opID)
	req = httptest.NewRequest(http.MethodGet, "/v1/operations/"+opID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestOperationDelegationEvidenceMismatch verifies a delegation whose
// operationId does not match the path is rejected with 401.
func TestOperationDelegationEvidenceMismatch(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keys := tokenTestKeys{key: key}
	config := iam.DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second}
	signer, _ := iam.NewDelegationSigner(config, keys)
	verifier, _ := iam.NewDelegationVerifier(config, keys)

	srv, _ := newTestServer()
	srv.ConfigureIntentDelegation(verifier)

	correlationID := "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"
	token := signOperationDelegation(t, signer, correlationID, iam.ActionRead, "356886f1-1b73-49a8-9275-1a85773cf973")
	req := httptest.NewRequest(http.MethodGet, "/v1/operations/99999999-9999-4999-8999-999999999999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", correlationID)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("mismatch status = %d body=%s", rec.Code, rec.Body.String())
	}
}
