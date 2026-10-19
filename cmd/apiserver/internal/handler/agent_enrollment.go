package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

const (
	agentEnrollmentSubject     = "cluster-agent"
	maxAgentEnrollmentBodySize = 32 << 10
	DefaultAgentEnrollmentTTL  = iam.DefaultEnrollmentTTL
	DefaultAgentSessionTTL     = 5 * time.Minute
	MaxAgentSessionTTL         = 10 * time.Minute
)

var (
	ErrAgentTargetNotFound      = errors.New("agent target not found")
	ErrAgentTargetNotKubernetes = errors.New("agent onboarding is only available for Kubernetes clusters")
)

// AgentEnrollmentStore is the narrow single-use enrollment persistence
// boundary. The full ServiceIdentityDBStore satisfies it.
type AgentEnrollmentStore interface {
	CreateEnrollmentCredential(context.Context, iam.EnrollmentCredentialRecord) error
	ConsumeEnrollmentCredential(context.Context, string, string, string, string, string, time.Time) (*iam.EnrollmentCredentialRecord, error)
	RevokeEnrollmentCredential(context.Context, string, string, string, string, time.Time) error
}

type AgentTargetResolver interface {
	ResolveAgentTarget(context.Context, string, string) error
}

type SQLAgentTargetResolver struct{ db *sql.DB }

func NewSQLAgentTargetResolver(db *sql.DB) *SQLAgentTargetResolver {
	return &SQLAgentTargetResolver{db: db}
}

func (r *SQLAgentTargetResolver) ResolveAgentTarget(ctx context.Context, tenantID, targetID string) error {
	var targetType string
	err := r.db.QueryRowContext(ctx, `
		SELECT target_type FROM runtime_targets WHERE id = $1 AND tenant_id = $2`, targetID, tenantID).Scan(&targetType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAgentTargetNotFound
	}
	if err != nil {
		return err
	}
	if targetType != "kubernetes" {
		return ErrAgentTargetNotKubernetes
	}
	return nil
}

type AgentEnrollmentAuditEvent struct {
	Action           string
	ActorSubjectID   string
	ActorSubjectType string
	ServiceSubjectID string
	TenantID         string
	TargetID         string
	EnrollmentID     string
	SessionID        string
}

type AgentEnrollmentAuditFunc func(context.Context, AgentEnrollmentAuditEvent)

// ObserverTokenIssuer is the narrow signing boundary for target-bound observer
// identities. The Agent receives the token with its session so observation
// cannot outlive or escape the session's tenant/target/generation.
type ObserverTokenIssuer interface {
	Sign(context.Context, string, iam.ObserverIdentity, string, int64) (string, error)
}

// AgentEnrollmentHandler issues one-time bootstrap credentials, exchanges them
// for renewable Agent sessions, rotates sessions, and revokes them. Secret
// material is returned once and never written to audit.
type AgentEnrollmentHandler struct {
	store          AgentEnrollmentStore
	sessions       iam.AgentSessionStore
	targets        AgentTargetResolver
	signer         *iam.AgentTunnelTokenSigner
	verifier       *iam.AgentTunnelTokenVerifier
	sessionTTL     time.Duration
	enrollmentTTL  time.Duration
	now            func() time.Time
	audit          AgentEnrollmentAuditFunc
	revokeConn     func(tenantID, targetID string)
	observerSigner ObserverTokenIssuer
	observerTTL    time.Duration
}

func NewAgentEnrollmentHandler(
	store AgentEnrollmentStore,
	sessions iam.AgentSessionStore,
	targets AgentTargetResolver,
	signer *iam.AgentTunnelTokenSigner,
	verifier *iam.AgentTunnelTokenVerifier,
	sessionTTL time.Duration,
	revokeConnection func(tenantID, targetID string),
	now func() time.Time,
	audit AgentEnrollmentAuditFunc,
) *AgentEnrollmentHandler {
	if sessionTTL <= 0 {
		sessionTTL = DefaultAgentSessionTTL
	}
	if sessionTTL > MaxAgentSessionTTL {
		sessionTTL = MaxAgentSessionTTL
	}
	if now == nil {
		now = time.Now
	}
	if audit == nil {
		audit = func(context.Context, AgentEnrollmentAuditEvent) {}
	}
	return &AgentEnrollmentHandler{
		store: store, sessions: sessions, targets: targets, signer: signer, verifier: verifier,
		sessionTTL: sessionTTL, enrollmentTTL: DefaultAgentEnrollmentTTL, now: now,
		audit: audit, revokeConn: revokeConnection,
	}
}

// ConfigureObserverTokenSigner binds observer issuance to the session lifecycle.
// Without a signer the legacy response remains unchanged.
func (h *AgentEnrollmentHandler) ConfigureObserverTokenSigner(signer ObserverTokenIssuer, ttl time.Duration) {
	h.observerSigner = signer
	h.observerTTL = ttl
}

func (h *AgentEnrollmentHandler) issueObserverToken(ctx context.Context, session *iam.AgentSessionRecord) (string, time.Time, error) {
	if h.observerSigner == nil || h.observerTTL <= 0 {
		return "", time.Time{}, nil
	}
	token, err := h.observerSigner.Sign(ctx, agentEnrollmentSubject, iam.ObserverIdentity{
		TenantID: session.TenantID, TargetID: session.TargetID, TargetKind: "KubernetesTarget",
		ObserverID: "agent-" + session.TargetID, ObserverKind: "Agent",
	}, session.ID, session.Generation)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, h.now().UTC().Add(h.observerTTL), nil
}

func (h *AgentEnrollmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	targetID := r.PathValue("id")
	if err := h.targets.ResolveAgentTarget(r.Context(), trusted.TenantID, targetID); err != nil {
		writeAgentTargetError(w, err)
		return
	}
	token, tokenHash, err := iam.NewEnrollmentSecret()
	if err != nil {
		response.InternalError(w, "agent enrollment creation failed")
		return
	}
	now := h.now().UTC()
	record := iam.EnrollmentCredentialRecord{
		ID: uuid.NewString(), TokenHash: tokenHash, TenantID: trusted.TenantID,
		TargetID: targetID, ExpectedSubject: agentEnrollmentSubject,
		IssuedBySubjectID: trusted.SubjectID, ExpiresAt: now.Add(h.enrollmentTTL), CreatedAt: now,
	}
	if err := h.store.CreateEnrollmentCredential(r.Context(), record); err != nil {
		response.InternalError(w, "agent enrollment creation failed")
		return
	}
	h.audit(r.Context(), AgentEnrollmentAuditEvent{
		Action: "agentEnrollment.create", ActorSubjectID: trusted.SubjectID,
		ActorSubjectType: trusted.SubjectType, TenantID: trusted.TenantID,
		TargetID: targetID, EnrollmentID: record.ID,
	})
	writeAgentSecretJSON(w, http.StatusOK, map[string]any{
		"enrollmentId":     record.ID,
		"enrollmentSecret": token,
		"expiresAt":        record.ExpiresAt.UTC().Format(time.RFC3339),
		"tenantId":         record.TenantID,
		"targetId":         record.TargetID,
		"subject":          record.ExpectedSubject,
	})
}

// Exchange is public: the single-use enrollment credential is the proof. It is
// bound to the exact tenant and target that created it, so replay, expiry,
// cross-target, and cross-tenant presentations all collapse into one rejection.
func (h *AgentEnrollmentHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	var request struct {
		EnrollmentSecret string `json:"enrollmentSecret"`
		TenantID         string `json:"tenantId"`
		TargetID         string `json:"targetId"`
		Subject          string `json:"subject"`
	}
	if !decodeAgentEnrollmentBody(w, r, &request) {
		return
	}
	if request.EnrollmentSecret == "" || strings.TrimSpace(request.TenantID) == "" || strings.TrimSpace(request.TargetID) == "" {
		response.BadRequest(w, "enrollmentSecret, tenantId, and targetId are required")
		return
	}
	subject := request.Subject
	if subject == "" {
		subject = agentEnrollmentSubject
	}
	now := h.now().UTC()
	record, err := h.store.ConsumeEnrollmentCredential(
		r.Context(), iam.HashEnrollmentSecret(request.EnrollmentSecret), request.TenantID, request.TargetID,
		subject, agentEnrollmentSubject, now)
	if err != nil {
		response.Unauthorized(w, "enrollment credential rejected")
		return
	}
	if err := h.targets.ResolveAgentTarget(r.Context(), record.TenantID, record.TargetID); err != nil {
		response.Unauthorized(w, "enrollment credential rejected")
		return
	}
	session, err := h.sessions.CreateAgentSession(r.Context(), iam.AgentSessionRecord{
		ID: uuid.NewString(), TenantID: record.TenantID, TargetID: record.TargetID,
		Subject: agentEnrollmentSubject, IssuedAt: now, ExpiresAt: now.Add(h.sessionTTL),
	})
	if err != nil {
		response.ServiceUnavailable(w, "agent session issuance unavailable")
		return
	}
	credential, expiry, err := h.signer.SignSession(r.Context(), session.TenantID, session.TargetID, session.ID, session.Generation, h.sessionTTL)
	if err != nil {
		response.ServiceUnavailable(w, "agent session issuance unavailable")
		return
	}
	observerToken, observerExpiry, err := h.issueObserverToken(r.Context(), session)
	if err != nil {
		response.ServiceUnavailable(w, "observer identity issuance unavailable")
		return
	}
	h.audit(r.Context(), AgentEnrollmentAuditEvent{
		Action: "agentEnrollment.exchange", ServiceSubjectID: agentEnrollmentSubject,
		TenantID: record.TenantID, TargetID: record.TargetID, EnrollmentID: record.ID, SessionID: session.ID,
	})
	payload := map[string]any{
		"credential": credential, "expiresAt": expiry.UTC().Format(time.RFC3339),
		"tenantId": record.TenantID, "targetId": record.TargetID, "subject": record.ExpectedSubject,
		"sessionId": session.ID, "generation": session.Generation,
	}
	if observerToken != "" {
		payload["observerToken"] = observerToken
		payload["observerTokenExpiresAt"] = observerExpiry.Format(time.RFC3339)
	}
	writeAgentSecretJSON(w, http.StatusOK, payload)
}

// Renew is public but requires a still-valid session credential. It keeps the
// same session ID and generation; a revoked session can never rotate again.
func (h *AgentEnrollmentHandler) Renew(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r)
	if !ok {
		response.Unauthorized(w, "agent session credential required")
		return
	}
	tenantID, targetID, sessionID, err := iam.ParseAgentSessionLookup(token)
	if err != nil {
		response.Unauthorized(w, "agent session credential rejected")
		return
	}
	now := h.now().UTC()
	session, err := h.sessions.GetAgentSession(r.Context(), tenantID, targetID, sessionID)
	if err != nil || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		response.Unauthorized(w, "agent session credential rejected")
		return
	}
	claims, err := h.verifier.VerifyClaims(r.Context(), token, tenantID, targetID)
	if err != nil || claims.SessionID != session.ID || claims.Generation != session.Generation {
		response.Unauthorized(w, "agent session credential rejected")
		return
	}
	rotated, err := h.sessions.RotateAgentSession(r.Context(), tenantID, targetID, sessionID, now, now.Add(h.sessionTTL))
	if err != nil {
		response.Unauthorized(w, "agent session credential rejected")
		return
	}
	credential, expiry, err := h.signer.SignSession(r.Context(), tenantID, targetID, rotated.ID, rotated.Generation, h.sessionTTL)
	if err != nil {
		response.ServiceUnavailable(w, "agent session issuance unavailable")
		return
	}
	observerToken, observerExpiry, err := h.issueObserverToken(r.Context(), rotated)
	if err != nil {
		response.ServiceUnavailable(w, "observer identity issuance unavailable")
		return
	}
	h.audit(r.Context(), AgentEnrollmentAuditEvent{
		Action: "agentEnrollment.renew", ServiceSubjectID: agentEnrollmentSubject,
		TenantID: tenantID, TargetID: targetID, SessionID: rotated.ID,
	})
	payload := map[string]any{
		"credential": credential, "expiresAt": expiry.UTC().Format(time.RFC3339),
		"tenantId": tenantID, "targetId": targetID, "subject": rotated.Subject,
		"sessionId": rotated.ID, "generation": rotated.Generation,
	}
	if observerToken != "" {
		payload["observerToken"] = observerToken
		payload["observerTokenExpiresAt"] = observerExpiry.Format(time.RFC3339)
	}
	writeAgentSecretJSON(w, http.StatusOK, payload)
}

func (h *AgentEnrollmentHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	enrollmentID := r.PathValue("enrollmentId")
	if _, err := uuid.Parse(enrollmentID); err != nil {
		response.BadRequest(w, "enrollmentId must be a UUID")
		return
	}
	targetID := r.PathValue("id")
	if err := h.store.RevokeEnrollmentCredential(r.Context(), trusted.TenantID, targetID, enrollmentID, trusted.SubjectID, h.now().UTC()); err != nil {
		if errors.Is(err, iam.ErrEnrollmentUnavailable) {
			response.NotFound(w, "enrollment not found")
			return
		}
		response.InternalError(w, "agent enrollment revocation failed")
		return
	}
	h.audit(r.Context(), AgentEnrollmentAuditEvent{
		Action: "agentEnrollment.revoke", ActorSubjectID: trusted.SubjectID,
		ActorSubjectType: trusted.SubjectType, TenantID: trusted.TenantID,
		TargetID: targetID, EnrollmentID: enrollmentID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// RevokeSessions revokes every active Agent session for the target and asks the
// tunnel server to close the persistent connection. The Agent cannot rotate a
// revoked session and stops with a stable diagnostic instead of reconnecting.
func (h *AgentEnrollmentHandler) RevokeSessions(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	targetID := r.PathValue("id")
	count, err := h.sessions.RevokeAgentSessions(r.Context(), trusted.TenantID, targetID, trusted.SubjectID, h.now().UTC())
	if err != nil {
		response.InternalError(w, "agent session revocation failed")
		return
	}
	if h.revokeConn != nil {
		h.revokeConn(trusted.TenantID, targetID)
	}
	h.audit(r.Context(), AgentEnrollmentAuditEvent{
		Action: "agentEnrollment.revokeSessions", ActorSubjectID: trusted.SubjectID,
		ActorSubjectType: trusted.SubjectType, TenantID: trusted.TenantID, TargetID: targetID,
	})
	response.Success(w, map[string]any{"revoked": count})
}

func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func decodeAgentEnrollmentBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentEnrollmentBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(w, "invalid body")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		response.BadRequest(w, "invalid body")
		return false
	}
	return true
}

func writeAgentTargetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAgentTargetNotFound):
		response.NotFound(w, "cluster not found")
	case errors.Is(err, ErrAgentTargetNotKubernetes):
		response.BadRequest(w, "agent enrollment is only available for Kubernetes clusters")
	default:
		response.InternalError(w, "agent target lookup failed")
	}
}

func writeAgentSecretJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
