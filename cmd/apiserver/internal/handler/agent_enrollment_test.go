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

type fakeAgentEnrollmentStore struct {
	mu          sync.Mutex
	enrollments map[string]iam.EnrollmentCredentialRecord
}

func newFakeAgentEnrollmentStore() *fakeAgentEnrollmentStore {
	return &fakeAgentEnrollmentStore{enrollments: make(map[string]iam.EnrollmentCredentialRecord)}
}

func (s *fakeAgentEnrollmentStore) CreateEnrollmentCredential(_ context.Context, record iam.EnrollmentCredentialRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.enrollments[record.TokenHash]; exists {
		return errors.New("duplicate")
	}
	s.enrollments[record.TokenHash] = record
	return nil
}

func (s *fakeAgentEnrollmentStore) ConsumeEnrollmentCredential(_ context.Context, tokenHash, tenantID, targetID, expectedSubject, consumedBySubject string, now time.Time) (*iam.EnrollmentCredentialRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.enrollments[tokenHash]
	if !ok || record.TenantID != tenantID || record.TargetID != targetID || record.ExpectedSubject != expectedSubject ||
		record.ConsumedAt != nil || record.RevokedAt != nil || !record.ExpiresAt.After(now) {
		return nil, iam.ErrEnrollmentUnavailable
	}
	record.ConsumedAt = &now
	record.ConsumedBySubject = consumedBySubject
	s.enrollments[tokenHash] = record
	return &record, nil
}

func (s *fakeAgentEnrollmentStore) RevokeEnrollmentCredential(_ context.Context, tenantID, targetID, enrollmentID, actorSubjectID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, record := range s.enrollments {
		if record.ID == enrollmentID && record.TenantID == tenantID && record.TargetID == targetID {
			if record.ConsumedAt != nil || record.RevokedAt != nil {
				return iam.ErrEnrollmentUnavailable
			}
			record.RevokedAt = &now
			s.enrollments[hash] = record
			return nil
		}
	}
	return iam.ErrEnrollmentUnavailable
}

type fakeAgentTargets map[string]map[string]string

func (f fakeAgentTargets) ResolveAgentTarget(_ context.Context, tenantID, targetID string) error {
	targetType, ok := f[tenantID][targetID]
	if !ok {
		return ErrAgentTargetNotFound
	}
	if targetType != "kubernetes" {
		return ErrAgentTargetNotKubernetes
	}
	return nil
}

type agentEnrollmentAuditRecorder struct {
	mu     sync.Mutex
	events []AgentEnrollmentAuditEvent
}

func (r *agentEnrollmentAuditRecorder) Record(_ context.Context, event AgentEnrollmentAuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

type fakeAgentSessionStore struct {
	mu         sync.Mutex
	sessions   map[string]iam.AgentSessionRecord
	generation map[string]int64
}

func newFakeAgentSessionStore() *fakeAgentSessionStore {
	return &fakeAgentSessionStore{
		sessions:   make(map[string]iam.AgentSessionRecord),
		generation: make(map[string]int64),
	}
}

func agentSessionKey(tenantID, targetID string) string { return tenantID + "|" + targetID }

func (s *fakeAgentSessionStore) CreateAgentSession(_ context.Context, record iam.AgentSessionRecord) (*iam.AgentSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := agentSessionKey(record.TenantID, record.TargetID)
	s.generation[key]++
	record.Generation = s.generation[key]
	s.sessions[record.ID] = record
	copy := record
	return &copy, nil
}

func (s *fakeAgentSessionStore) GetAgentSession(_ context.Context, tenantID, targetID, sessionID string) (*iam.AgentSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[sessionID]
	if !ok || record.TenantID != tenantID || record.TargetID != targetID {
		return nil, iam.ErrAgentSessionNotFound
	}
	copy := record
	return &copy, nil
}

func (s *fakeAgentSessionStore) RotateAgentSession(_ context.Context, tenantID, targetID, sessionID string, issuedAt, expiresAt time.Time) (*iam.AgentSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[sessionID]
	if !ok || record.TenantID != tenantID || record.TargetID != targetID || record.RevokedAt != nil {
		return nil, iam.ErrAgentSessionUnavailable
	}
	record.IssuedAt = issuedAt
	record.ExpiresAt = expiresAt
	s.sessions[sessionID] = record
	copy := record
	return &copy, nil
}

func (s *fakeAgentSessionStore) RevokeAgentSessions(_ context.Context, tenantID, targetID, actorSubjectID string, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for id, record := range s.sessions {
		if record.TenantID == tenantID && record.TargetID == targetID && record.RevokedAt == nil {
			record.RevokedAt = &now
			s.sessions[id] = record
			count++
		}
	}
	return count, nil
}

type agentSessionRevoker struct {
	mu    sync.Mutex
	calls [][2]string
}

func (r *agentSessionRevoker) Revoke(tenantID, targetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{tenantID, targetID})
}

func (r *agentSessionRevoker) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func newAgentEnrollmentHandlerForTest(t *testing.T) (*AgentEnrollmentHandler, *fakeAgentEnrollmentStore, *fakeAgentSessionStore, *agentEnrollmentAuditRecorder, *agentSessionRevoker, *time.Time, *iam.AgentTunnelTokenVerifier, iam.TrustedContext) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := handlerTestKeys{key: key}
	now := time.Unix(2_000_000_000, 0).UTC()
	config := iam.AgentTunnelTokenConfig{
		Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel",
		TTL: iam.MaxAgentTunnelTokenTTL, Now: func() time.Time { return now },
	}
	signer, err := iam.NewAgentTunnelTokenSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := iam.NewAgentTunnelTokenVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeAgentEnrollmentStore()
	sessions := newFakeAgentSessionStore()
	audit := &agentEnrollmentAuditRecorder{}
	revoker := &agentSessionRevoker{}
	targets := fakeAgentTargets{
		"tenant-a": {"target-a": "kubernetes", "edge-a": "edge"},
		"tenant-b": {"target-b": "kubernetes"},
	}
	handler := NewAgentEnrollmentHandler(store, sessions, targets, signer, verifier, DefaultAgentSessionTTL, revoker.Revoke, func() time.Time { return now }, audit.Record)
	actor := iam.TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-a"}
	return handler, store, sessions, audit, revoker, &now, verifier, actor
}

func withAgentTrusted(r *http.Request, actor iam.TrustedContext) *http.Request {
	return r.WithContext(iam.WithTrustedContext(r.Context(), actor))
}

func createAgentEnrollmentForTest(t *testing.T, h *AgentEnrollmentHandler, actor iam.TrustedContext, targetID string) (string, string) {
	t.Helper()
	req := withAgentTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/resources/clusters/"+targetID+"/agent-enrollments", nil), actor)
	req.SetPathValue("id", targetID)
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		EnrollmentID string `json:"enrollmentId"`
		Secret       string `json:"enrollmentSecret"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.EnrollmentID == "" || response.Secret == "" {
		t.Fatal("create returned no enrollment secret")
	}
	return response.EnrollmentID, response.Secret
}

func exchangeAgentEnrollment(h *AgentEnrollmentHandler, token, tenantID, targetID string) *httptest.ResponseRecorder {
	body := `{"enrollmentSecret":"` + token + `","tenantId":"` + tenantID + `","targetId":"` + targetID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-enrollments:exchange", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Exchange(rec, req)
	return rec
}

func TestAgentEnrollmentCreateExchangeReplayAndRevoke(t *testing.T) {
	h, _, _, audit, _, now, verifier, actor := newAgentEnrollmentHandlerForTest(t)
	enrollmentID, token := createAgentEnrollmentForTest(t, h, actor, "target-a")

	rec := exchangeAgentEnrollment(h, token, "tenant-a", "target-a")
	if rec.Code != http.StatusOK {
		t.Fatalf("exchange failed: %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Credential string `json:"credential"`
		ExpiresAt  string `json:"expiresAt"`
		TenantID   string `json:"tenantId"`
		TargetID   string `json:"targetId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	expiry, err := verifier.Verify(context.Background(), response.Credential, "tenant-a", "target-a")
	if err != nil {
		t.Fatalf("session credential failed verification: %v", err)
	}
	if expiry.Sub(*now) != DefaultAgentSessionTTL {
		t.Fatalf("session expiry = %v, want %v", expiry.Sub(*now), DefaultAgentSessionTTL)
	}

	if replay := exchangeAgentEnrollment(h, token, "tenant-a", "target-a"); replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay was accepted: %d", replay.Code)
	}

	_, wrongTenantToken := createAgentEnrollmentForTest(t, h, actor, "target-a")
	if wrong := exchangeAgentEnrollment(h, wrongTenantToken, "tenant-b", "target-a"); wrong.Code != http.StatusUnauthorized {
		t.Fatalf("cross-tenant exchange was accepted: %d", wrong.Code)
	}
	if wrong := exchangeAgentEnrollment(h, wrongTenantToken, "tenant-a", "target-b"); wrong.Code != http.StatusUnauthorized {
		t.Fatalf("cross-target exchange was accepted: %d", wrong.Code)
	}

	revokeID, revokeToken := createAgentEnrollmentForTest(t, h, actor, "target-a")
	revokeReq := withAgentTrusted(httptest.NewRequest(http.MethodDelete, "/api/v1/resources/clusters/target-a/agent-enrollments/"+revokeID, nil), actor)
	revokeReq.SetPathValue("id", "target-a")
	revokeReq.SetPathValue("enrollmentId", revokeID)
	revokeRec := httptest.NewRecorder()
	h.Revoke(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke failed: %d %s", revokeRec.Code, revokeRec.Body.String())
	}
	if rejected := exchangeAgentEnrollment(h, revokeToken, "tenant-a", "target-a"); rejected.Code != http.StatusUnauthorized {
		t.Fatalf("revoked enrollment was accepted: %d", rejected.Code)
	}

	expiredToken := func() string {
		_, value := createAgentEnrollmentForTest(t, h, actor, "target-a")
		*now = now.Add(DefaultAgentEnrollmentTTL + time.Second)
		return value
	}()
	if expired := exchangeAgentEnrollment(h, expiredToken, "tenant-a", "target-a"); expired.Code != http.StatusUnauthorized {
		t.Fatalf("expired enrollment was accepted: %d", expired.Code)
	}

	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.events) < 3 {
		t.Fatalf("expected create/exchange/revoke audit events: %+v", audit.events)
	}
	for _, event := range audit.events {
		if event.TenantID != "tenant-a" || event.TargetID == "" || event.EnrollmentID == "" {
			t.Fatalf("incomplete agent enrollment audit event: %+v", event)
		}
		if event.Action == "agentEnrollment.exchange" && event.ServiceSubjectID != "cluster-agent" {
			t.Fatalf("exchange audit lost the service subject: %+v", event)
		}
	}
	if enrollmentID != audit.events[0].EnrollmentID {
		t.Fatalf("create audit did not bind the enrollment: %+v", audit.events[0])
	}
}

func TestAgentEnrollmentRejectsWrongTargetTypeAndCrossTenantCreate(t *testing.T) {
	h, _, _, _, _, _, _, actor := newAgentEnrollmentHandlerForTest(t)

	edgeReq := withAgentTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/resources/clusters/edge-a/agent-enrollments", nil), actor)
	edgeReq.SetPathValue("id", "edge-a")
	edgeRec := httptest.NewRecorder()
	h.Create(edgeRec, edgeReq)
	if edgeRec.Code != http.StatusBadRequest {
		t.Fatalf("edge target enrollment was accepted: %d", edgeRec.Code)
	}

	otherTenant := iam.TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-b"}
	crossReq := withAgentTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/resources/clusters/target-a/agent-enrollments", nil), otherTenant)
	crossReq.SetPathValue("id", "target-a")
	crossRec := httptest.NewRecorder()
	h.Create(crossRec, crossReq)
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant enrollment was accepted: %d", crossRec.Code)
	}
}

func TestAgentEnrollmentSessionRenewAndRevocation(t *testing.T) {
	h, _, _, audit, revoker, _, verifier, actor := newAgentEnrollmentHandlerForTest(t)
	_, enrollmentToken := createAgentEnrollmentForTest(t, h, actor, "target-a")
	exchangeRec := exchangeAgentEnrollment(h, enrollmentToken, "tenant-a", "target-a")
	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("exchange failed: %d %s", exchangeRec.Code, exchangeRec.Body.String())
	}
	var first struct {
		Credential string `json:"credential"`
		SessionID  string `json:"sessionId"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(exchangeRec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.SessionID == "" || first.Generation != 1 || first.Credential == "" {
		t.Fatalf("unexpected session: %+v", first)
	}
	claims, err := verifier.VerifyClaims(context.Background(), first.Credential, "tenant-a", "target-a")
	if err != nil || claims.SessionID != first.SessionID || claims.Generation != first.Generation {
		t.Fatalf("session token claims mismatch: %+v err=%v", claims, err)
	}

	renewReq := httptest.NewRequest(http.MethodPost, "/api/v1/agent-sessions:renew", nil)
	renewReq.Header.Set("Authorization", "Bearer "+first.Credential)
	renewRec := httptest.NewRecorder()
	h.Renew(renewRec, renewReq)
	if renewRec.Code != http.StatusOK {
		t.Fatalf("renew failed: %d %s", renewRec.Code, renewRec.Body.String())
	}
	var renewed struct {
		Credential string `json:"credential"`
		SessionID  string `json:"sessionId"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(renewRec.Body.Bytes(), &renewed); err != nil {
		t.Fatal(err)
	}
	if renewed.SessionID != first.SessionID || renewed.Generation != first.Generation || renewed.Credential == first.Credential {
		t.Fatalf("renewal did not rotate within the same session: %+v", renewed)
	}
	if _, err := verifier.VerifyClaims(context.Background(), renewed.Credential, "tenant-a", "target-a"); err != nil {
		t.Fatalf("rotated token failed verification: %v", err)
	}

	revokeReq := withAgentTrusted(httptest.NewRequest(http.MethodDelete, "/api/v1/resources/clusters/target-a/agent-session", nil), actor)
	revokeReq.SetPathValue("id", "target-a")
	revokeRec := httptest.NewRecorder()
	h.RevokeSessions(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke sessions failed: %d %s", revokeRec.Code, revokeRec.Body.String())
	}
	if revoker.Count() != 1 {
		t.Fatalf("expected one tunnel close callback, got %d", revoker.Count())
	}

	for _, token := range []string{first.Credential, renewed.Credential} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-sessions:renew", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.Renew(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked session renewed: %d", rec.Code)
		}
	}

	audit.mu.Lock()
	defer audit.mu.Unlock()
	seen := map[string]bool{}
	for _, event := range audit.events {
		seen[event.Action] = true
		if event.Action == "agentEnrollment.renew" && event.ServiceSubjectID != "cluster-agent" {
			t.Fatalf("renew audit lost the service subject: %+v", event)
		}
	}
	if !seen["agentEnrollment.renew"] || !seen["agentEnrollment.revokeSessions"] {
		t.Fatalf("missing renew/revoke session audit events: %+v", audit.events)
	}
}
