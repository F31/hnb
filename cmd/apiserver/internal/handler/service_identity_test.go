package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type handlerTestKeys struct{ key *ecdsa.PrivateKey }

func (k handlerTestKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return "test-key-1", k.key, nil
}

func (k handlerTestKeys) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid != "test-key-1" {
		return nil, errors.New("unknown key")
	}
	return &k.key.PublicKey, nil
}

type handlerTestStore struct {
	mu          sync.Mutex
	identities  map[string]iam.ServiceIdentityRecord
	credentials map[string]iam.ServiceCredentialStatus
}

func newHandlerTestStore() *handlerTestStore {
	return &handlerTestStore{
		identities:  make(map[string]iam.ServiceIdentityRecord),
		credentials: make(map[string]iam.ServiceCredentialStatus),
	}
}

func handlerKey(tenantID, id string) string { return tenantID + "|" + id }

func (s *handlerTestStore) CreateServiceIdentity(_ context.Context, record iam.ServiceIdentityRecord) (*iam.ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identities[handlerKey(record.TenantID, record.ID)] = record
	copy := record
	return &copy, nil
}

func (s *handlerTestStore) GetServiceIdentity(_ context.Context, tenantID, id string) (*iam.ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.identities[handlerKey(tenantID, id)]
	if !ok {
		return nil, iam.ErrServiceIdentityNotFound
	}
	copy := record
	return &copy, nil
}

func (s *handlerTestStore) ListServiceIdentities(_ context.Context, tenantID string) ([]iam.ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []iam.ServiceIdentityRecord
	for _, record := range s.identities {
		if record.TenantID == tenantID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *handlerTestStore) RegisterServiceCredential(_ context.Context, record iam.ServiceCredentialRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, ok := s.identities[handlerKey(record.TenantID, record.ServiceIdentityID)]
	if !ok {
		return iam.ErrServiceIdentityNotFound
	}
	s.credentials[handlerKey(record.TenantID, record.ID)] = iam.ServiceCredentialStatus{Credential: record, Identity: identity}
	return nil
}

func (s *handlerTestStore) GetServiceCredentialStatus(_ context.Context, tenantID, credentialID string) (*iam.ServiceCredentialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.credentials[handlerKey(tenantID, credentialID)]
	if !ok {
		return nil, iam.ErrServiceCredentialNotFound
	}
	if identity, ok := s.identities[handlerKey(status.Credential.TenantID, status.Credential.ServiceIdentityID)]; ok {
		status.Identity = identity
	}
	return &status, nil
}

func (s *handlerTestStore) RevokeServiceIdentity(_ context.Context, tenantID, id string, expectedVersion int64, actorSubjectID, reason string, now time.Time) (*iam.ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.identities[handlerKey(tenantID, id)]
	if !ok {
		return nil, iam.ErrServiceIdentityNotFound
	}
	if record.Status == iam.ServiceIdentityStatusRevoked {
		return nil, iam.ErrServiceIdentityRevoked
	}
	if record.Version != expectedVersion {
		return nil, iam.ErrServiceIdentityVersionConflict
	}
	record.Status = iam.ServiceIdentityStatusRevoked
	record.Version++
	record.RevokedAt = &now
	record.RevocationReason = reason
	s.identities[handlerKey(tenantID, id)] = record
	copy := record
	return &copy, nil
}

func (s *handlerTestStore) RevokeServiceCredential(_ context.Context, tenantID, identityID, credentialID, actorSubjectID, reason string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := handlerKey(tenantID, credentialID)
	status, ok := s.credentials[key]
	if !ok || status.Credential.ServiceIdentityID != identityID {
		return iam.ErrServiceCredentialNotFound
	}
	if status.Credential.RevokedAt != nil {
		return nil
	}
	status.Credential.RevokedAt = &now
	s.credentials[key] = status
	return nil
}

func (s *handlerTestStore) CreateEnrollmentCredential(context.Context, iam.EnrollmentCredentialRecord) error {
	return nil
}

func (s *handlerTestStore) ConsumeEnrollmentCredential(context.Context, string, string, string, string, string, time.Time) (*iam.EnrollmentCredentialRecord, error) {
	return nil, iam.ErrEnrollmentUnavailable
}

func (s *handlerTestStore) RevokeEnrollmentCredential(context.Context, string, string, string, string, time.Time) error {
	return iam.ErrEnrollmentUnavailable
}

type handlerAuditRecorder struct {
	mu     sync.Mutex
	events []iam.ServiceCredentialAuditEvent
}

func (r *handlerAuditRecorder) Record(_ context.Context, event iam.ServiceCredentialAuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func newServiceIdentityHandlerForTest(t *testing.T) (*ServiceIdentityHandler, *iam.ServiceCredentialAuthority, *handlerTestStore, *handlerAuditRecorder, iam.TrustedContext) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := handlerTestKeys{key: key}
	store := newHandlerTestStore()
	audit := &handlerAuditRecorder{}
	now := time.Unix(2_000_000_000, 0).UTC()
	authority, err := iam.NewServiceCredentialAuthority(store, keys, keys, "https://issuer.example", func() time.Time { return now }, audit.Record)
	if err != nil {
		t.Fatal(err)
	}
	actor := iam.TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-a", PolicyVersion: "default:1"}
	return NewServiceIdentityHandler(authority, store), authority, store, audit, actor
}

func withServiceIdentityTrusted(r *http.Request, actor iam.TrustedContext) *http.Request {
	return r.WithContext(iam.WithTrustedContext(r.Context(), actor))
}

func createServiceIdentityForTest(t *testing.T, h *ServiceIdentityHandler, actor iam.TrustedContext) string {
	t.Helper()
	body := `{"subjectId":"` + uuid.NewString() + `","audience":"provider-audience","permission":{"resourceKind":"runtimeProvider","resourceId":"provider-a","action":"execute","tenantId":"tenant-a"}}`
	req := withServiceIdentityTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/service-identities", strings.NewReader(body)), actor)
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.ID == "" {
		t.Fatal("create returned no identity ID")
	}
	return response.Data.ID
}

func TestServiceIdentityHandlerIssueRenewAndRevoke(t *testing.T) {
	h, authority, _, audit, actor := newServiceIdentityHandlerForTest(t)
	identityID := createServiceIdentityForTest(t, h, actor)

	issueReq := withServiceIdentityTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/service-identities/"+identityID+"/credentials", strings.NewReader(`{}`)), actor)
	issueReq.SetPathValue("id", identityID)
	issueRec := httptest.NewRecorder()
	h.Issue(issueRec, issueReq)
	if issueRec.Code != http.StatusOK {
		t.Fatalf("issue failed: %d %s", issueRec.Code, issueRec.Body.String())
	}
	var issued struct {
		Credential   string `json:"credential"`
		CredentialID string `json:"credentialId"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := json.Unmarshal(issueRec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Credential == "" || issued.CredentialID == "" {
		t.Fatal("issue returned no credential")
	}
	if _, err := time.Parse(time.RFC3339, issued.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Verify(context.Background(), issued.Credential); err != nil {
		t.Fatal(err)
	}

	renewReq := httptest.NewRequest(http.MethodPost, "/api/v1/service-credentials:renew", nil)
	renewReq.Header.Set("Authorization", "Bearer "+issued.Credential)
	renewRec := httptest.NewRecorder()
	h.Renew(renewRec, renewReq)
	if renewRec.Code != http.StatusOK {
		t.Fatalf("renew failed: %d %s", renewRec.Code, renewRec.Body.String())
	}
	var renewed struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(renewRec.Body.Bytes(), &renewed); err != nil {
		t.Fatal(err)
	}
	if renewed.Credential == "" || renewed.Credential == issued.Credential {
		t.Fatal("renew did not rotate the credential")
	}

	revokeReq := withServiceIdentityTrusted(httptest.NewRequest(http.MethodDelete, "/api/v1/service-identities/"+identityID, strings.NewReader(`{"version":1,"reason":"retired"}`)), actor)
	revokeReq.SetPathValue("id", identityID)
	revokeRec := httptest.NewRecorder()
	h.RevokeIdentity(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke failed: %d %s", revokeRec.Code, revokeRec.Body.String())
	}
	if _, err := authority.Verify(context.Background(), renewed.Credential); !errors.Is(err, iam.ErrServiceIdentityRevoked) {
		t.Fatalf("credential survived identity revocation: %v", err)
	}

	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.events) < 3 {
		t.Fatalf("expected issue/renew/revoke audit events: %+v", audit.events)
	}
	for _, event := range audit.events {
		if event.ActorSubjectID == "" || event.ServiceSubjectID == "" || event.TenantID != "tenant-a" {
			t.Fatalf("audit event did not distinguish actor and service subject: %+v", event)
		}
	}
}

func TestServiceIdentityHandlerRejectsCrossTenantAndInvalidRenewal(t *testing.T) {
	h, _, _, _, actor := newServiceIdentityHandlerForTest(t)
	identityID := createServiceIdentityForTest(t, h, actor)
	otherTenant := iam.TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-b"}

	getReq := withServiceIdentityTrusted(httptest.NewRequest(http.MethodGet, "/api/v1/service-identities/"+identityID, nil), otherTenant)
	getReq.SetPathValue("id", identityID)
	getRec := httptest.NewRecorder()
	h.Get(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read was accepted: %d", getRec.Code)
	}

	issueReq := withServiceIdentityTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/service-identities/"+identityID+"/credentials", strings.NewReader(`{}`)), otherTenant)
	issueReq.SetPathValue("id", identityID)
	issueRec := httptest.NewRecorder()
	h.Issue(issueRec, issueReq)
	if issueRec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant issue was accepted: %d", issueRec.Code)
	}

	renewReq := httptest.NewRequest(http.MethodPost, "/api/v1/service-credentials:renew", nil)
	renewReq.Header.Set("Authorization", "Bearer not-a-credential")
	renewRec := httptest.NewRecorder()
	h.Renew(renewRec, renewReq)
	if renewRec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid renewal was accepted: %d", renewRec.Code)
	}
	if strings.Contains(renewRec.Body.String(), "not-a-credential") {
		t.Fatal("renewal error leaked the presented credential")
	}
}
