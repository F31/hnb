package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

func TestIssueClusterKubeconfigSuccess(t *testing.T) {
	srv, st, cipher := newSecretTestServer(t)
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{ID: id, TenantID: "tenant-a", Name: id, TargetType: "kubernetes"}
	kc := "apiVersion: v1\nkind: Config\nclusters: []\nusers: []\n"
	sealed, err := cipher.Encrypt([]byte(kc))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RegisterSecretReference(t.Context(), store.RegisterSecretReferenceRequest{
		TenantID: "tenant-a", Scope: "tenant-a", Name: "cluster-a-credential", Purpose: "kubeconfig",
		AllowedLifecycleProviderID: "runtime-target.lifecycle.kubernetes", EncryptedValue: sealed,
	}); err != nil {
		t.Fatal(err)
	}

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var payload struct {
		Kubeconfig string `json:"kubeconfig"`
		Filename   string `json:"filename"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response %q: %v", rec.Body.String(), err)
	}
	if payload.Kubeconfig != kc {
		t.Fatalf("kubeconfig = %q, want %q", payload.Kubeconfig, kc)
	}
	if payload.Filename != id+".kubeconfig" {
		t.Fatalf("filename = %q, want %q", payload.Filename, id+".kubeconfig")
	}
}

func TestIssueClusterKubeconfigEdgeTargetConflict(t *testing.T) {
	srv, st, _ := newSecretTestServer(t)
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{ID: id, TenantID: "tenant-a", Name: id, TargetType: "edge_runtime"}

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestIssueClusterKubeconfigMissingSecretNotFound(t *testing.T) {
	srv, st, _ := newSecretTestServer(t)
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{ID: id, TenantID: "tenant-a", Name: id, TargetType: "kubernetes"}

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestIssueClusterKubeconfigMissingTargetNotFound(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	id := uuid.NewString()

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestIssueClusterKubeconfigResolvesExactCredentialRef(t *testing.T) {
	srv, st, cipher := newSecretTestServer(t)
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{
		ID: id, TenantID: "tenant-a", Name: id, TargetType: "kubernetes",
		CredentialRef: &store.CredentialRef{Provider: "local-aes", Scope: "tenant-a", Name: "cluster-b-credential"},
	}
	// Register cluster-b first, cluster-a second: the tenant-latest heuristic
	// would pick cluster-a, so a correct exact-ref resolution must return b.
	registerKubeconfigSecret(t, st, cipher, "tenant-a", "cluster-b-credential", "apiVersion: v1\nkind: Config\nclusters: [b]\n")
	registerKubeconfigSecret(t, st, cipher, "tenant-a", "cluster-a-credential", "apiVersion: v1\nkind: Config\nclusters: [a]\n")

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var payload struct {
		Kubeconfig string `json:"kubeconfig"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kubeconfig != "apiVersion: v1\nkind: Config\nclusters: [b]\n" {
		t.Fatalf("resolved wrong secret: %q", payload.Kubeconfig)
	}
}

func TestIssueClusterKubeconfigMissingRefSecretFailsClosed(t *testing.T) {
	srv, st, cipher := newSecretTestServer(t)
	id := uuid.NewString()
	st.targets[id] = &store.RuntimeTarget{
		ID: id, TenantID: "tenant-a", Name: id, TargetType: "kubernetes",
		CredentialRef: &store.CredentialRef{Provider: "local-aes", Scope: "tenant-a", Name: "deleted-credential"},
	}
	// The tenant has another kubeconfig secret; a target with a broken ref
	// must NOT fall back to it (would issue the wrong cluster's credentials).
	registerKubeconfigSecret(t, st, cipher, "tenant-a", "other-credential", "apiVersion: v1\nkind: Config\nclusters: [other]\n")

	rec := issueKubeconfig(t, srv, id)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func registerKubeconfigSecret(t *testing.T, st *fakeStore, cipher *testSecretCipher, scope, name, content string) {
	t.Helper()
	sealed, err := cipher.Encrypt([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RegisterSecretReference(t.Context(), store.RegisterSecretReferenceRequest{
		TenantID: "tenant-a", Scope: scope, Name: name, Purpose: "kubeconfig",
		AllowedLifecycleProviderID: "runtime-target.lifecycle.kubernetes", EncryptedValue: sealed,
	}); err != nil {
		t.Fatal(err)
	}
}

func issueKubeconfig(t *testing.T, srv *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	signer := testDelegationSignerFor(t, srv)
	token, err := signer.Sign(t.Context(), delegatedTrustedContext(), iam.DelegationEvidence{
		Scope:         iam.DelegationScope{ResourceKind: string(iam.ResourceClusterMetadata), ResourceID: id},
		Action:        iam.ActionRead,
		CorrelationID: testCorrelationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/clusters/"+id+"/kubeconfig:issue", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Correlation-ID", testCorrelationID)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}
