package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentSessionCredential is one renewable Agent tunnel session. Rotation keeps
// the same session ID and generation; revocation makes every future renewal
// fail permanently.
type AgentSessionCredential struct {
	Credential string    `json:"credential"`
	ExpiresAt  time.Time `json:"expiresAt"`
	SessionID  string    `json:"sessionId"`
	Generation int64     `json:"generation"`
	TenantID   string    `json:"tenantId,omitempty"`
	TargetID   string    `json:"targetId,omitempty"`
	Subject    string    `json:"subject,omitempty"`
}

type AgentSessionIssuer interface {
	ExchangeAgentSession(context.Context, string, string, string) (AgentSessionCredential, error)
	RenewAgentSession(context.Context, string) (AgentSessionCredential, error)
}

const MaxAgentSessionResponseBytes = 64 * 1024

type HTTPAgentSessionIssuerConfig struct {
	BaseURL          string
	Client           *http.Client
	MaxResponseBytes int64
	Now              func() time.Time
}

type HTTPAgentSessionIssuer struct {
	baseURL          string
	client           *http.Client
	maxResponseBytes int64
	now              func() time.Time
}

func NewHTTPAgentSessionIssuer(config HTTPAgentSessionIssuerConfig) (*HTTPAgentSessionIssuer, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("agent session issuer URL must be an absolute HTTP(S) URL without credentials")
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: ServiceCredentialRPCTimeout + time.Second}
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = MaxAgentSessionResponseBytes
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &HTTPAgentSessionIssuer{
		baseURL: strings.TrimRight(parsed.String(), "/"), client: config.Client,
		maxResponseBytes: config.MaxResponseBytes, now: config.Now,
	}, nil
}

func (i *HTTPAgentSessionIssuer) ExchangeAgentSession(ctx context.Context, enrollmentSecret, tenantID, targetID string) (AgentSessionCredential, error) {
	if enrollmentSecret == "" || !boundedClaim(tenantID, 128) || tenantID == "*" || !boundedClaim(targetID, 256) {
		return AgentSessionCredential{}, ErrServiceCredentialRejected
	}
	body, err := json.Marshal(map[string]string{
		"enrollmentSecret": enrollmentSecret, "tenantId": tenantID, "targetId": targetID,
	})
	if err != nil {
		return AgentSessionCredential{}, ErrServiceCredentialUnavailable
	}
	return i.post(ctx, "/api/v1/agent-enrollments:exchange", body, "")
}

func (i *HTTPAgentSessionIssuer) RenewAgentSession(ctx context.Context, currentToken string) (AgentSessionCredential, error) {
	if currentToken == "" || len(currentToken) > MaxAgentTunnelTokenSize {
		return AgentSessionCredential{}, ErrServiceCredentialRejected
	}
	return i.post(ctx, "/api/v1/agent-sessions:renew", nil, currentToken)
}

func (i *HTTPAgentSessionIssuer) post(ctx context.Context, path string, body []byte, bearer string) (AgentSessionCredential, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, ServiceCredentialRPCTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(rpcCtx, http.MethodPost, i.baseURL+path, reader)
	if err != nil {
		return AgentSessionCredential{}, fmt.Errorf("%w: create request", ErrServiceCredentialUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return AgentSessionCredential{}, fmt.Errorf("%w: request failed", ErrServiceCredentialUnavailable)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, i.maxResponseBytes+1))
	if err != nil || int64(len(data)) > i.maxResponseBytes {
		return AgentSessionCredential{}, fmt.Errorf("%w: invalid response size", ErrServiceCredentialUnavailable)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return AgentSessionCredential{}, ErrServiceCredentialRejected
		case http.StatusConflict:
			return AgentSessionCredential{}, ErrServiceIdentityRevoked
		default:
			return AgentSessionCredential{}, fmt.Errorf("%w: endpoint returned HTTP %d", ErrServiceCredentialUnavailable, resp.StatusCode)
		}
	}
	var payload struct {
		Credential string `json:"credential"`
		ExpiresAt  string `json:"expiresAt"`
		SessionID  string `json:"sessionId"`
		Generation int64  `json:"generation"`
		TenantID   string `json:"tenantId"`
		TargetID   string `json:"targetId"`
		Subject    string `json:"subject"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return AgentSessionCredential{}, fmt.Errorf("%w: invalid response payload", ErrServiceCredentialUnavailable)
	}
	expiry, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil || !expiry.After(i.now().UTC()) {
		return AgentSessionCredential{}, fmt.Errorf("%w: session is already expired", ErrServiceCredentialUnavailable)
	}
	if len(payload.Credential) == 0 || len(payload.Credential) > MaxAgentTunnelTokenSize ||
		payload.Generation < 1 || payload.Subject == "" {
		return AgentSessionCredential{}, fmt.Errorf("%w: incomplete session payload", ErrServiceCredentialUnavailable)
	}
	if _, err := uuid.Parse(payload.SessionID); err != nil {
		return AgentSessionCredential{}, ErrServiceCredentialRejected
	}
	return AgentSessionCredential{
		Credential: payload.Credential, ExpiresAt: expiry, SessionID: payload.SessionID,
		Generation: payload.Generation, TenantID: payload.TenantID, TargetID: payload.TargetID, Subject: payload.Subject,
	}, nil
}

// AgentSessionTokenSource is the Agent-side renewable tunnel credential. It
// exchanges the single-use enrollment secret once, then rotates the session at
// half its lifetime. A revoked/rejected session is a permanent stop; a
// transient issuer failure keeps the old token only until its original exp.
type AgentSessionTokenSource struct {
	ctx              context.Context
	cancel           context.CancelFunc
	verifier         *AgentTunnelTokenVerifier
	issuer           AgentSessionIssuer
	tenantID         string
	targetID         string
	enrollmentSecret string
	clock            renewalClock
	once             sync.Once
	mu               sync.Mutex
	changed          chan struct{}
	value            string
	expiry           time.Time
	lastErr          error
	store            AgentSessionPersistenceStore
	sessionPath      string
}

func NewAgentSessionTokenSource(parent context.Context, verifier *AgentTunnelTokenVerifier, issuer AgentSessionIssuer, tenantID, targetID, enrollmentSecret string) (*AgentSessionTokenSource, error) {
	if verifier == nil || issuer == nil {
		return nil, errors.New("agent session verifier and issuer are required")
	}
	if !boundedClaim(tenantID, 128) || tenantID == "*" || !boundedClaim(targetID, 256) || targetID == "*" {
		return nil, errors.New("explicit tenant and target are required")
	}
	if enrollmentSecret == "" || len(enrollmentSecret) > 512 {
		return nil, errors.New("enrollment secret is required")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &AgentSessionTokenSource{
		ctx: ctx, cancel: cancel, verifier: verifier, issuer: issuer,
		tenantID: tenantID, targetID: targetID, enrollmentSecret: enrollmentSecret,
		clock: serviceRenewalClock{now: time.Now}, changed: make(chan struct{}, 1),
	}, nil
}

// ConfigurePersistence persists the newest session credential to a private
// file so an Agent restart can resume without reusing the single-use
// enrollment secret. A load failure falls back to the normal exchange path.
func (s *AgentSessionTokenSource) ConfigurePersistence(store AgentSessionPersistenceStore, path string) {
	s.mu.Lock()
	s.store, s.sessionPath = store, path
	s.mu.Unlock()
}

func (s *AgentSessionTokenSource) Token(ctx context.Context) (string, error) {
	s.once.Do(func() { go s.run() })
	for {
		s.mu.Lock()
		if s.value != "" && s.expiry.After(s.clock.Now()) {
			value := s.value
			s.mu.Unlock()
			return value, nil
		}
		if s.lastErr != nil {
			err := s.lastErr
			s.mu.Unlock()
			return "", err
		}
		s.mu.Unlock()
		select {
		case <-s.changed:
		case <-ctx.Done():
			return "", ctx.Err()
		case <-s.ctx.Done():
			return "", ErrServiceCredentialUnavailable
		}
	}
}

func (s *AgentSessionTokenSource) Close() error {
	s.cancel()
	s.notify()
	return nil
}

func (s *AgentSessionTokenSource) run() {
	if !s.loadPersisted() && !s.exchange() {
		return
	}
	backoff := time.Second
	for {
		s.mu.Lock()
		remaining := s.expiry.Sub(s.clock.Now())
		current := s.value
		s.mu.Unlock()
		if remaining <= 0 {
			s.fail(ErrServiceCredentialUnavailable)
			return
		}
		if !s.wait(remaining / 2) {
			return
		}
		credential, err := s.issuer.RenewAgentSession(s.ctx, current)
		if err != nil {
			if isPermanentAgentSessionError(err) {
				s.fail(err)
				return
			}
			s.mu.Lock()
			remaining = s.expiry.Sub(s.clock.Now())
			s.mu.Unlock()
			if remaining <= 0 || !s.wait(backoff) {
				s.fail(ErrServiceCredentialUnavailable)
				return
			}
			if backoff < 4*time.Second {
				backoff *= 2
			}
			continue
		}
		if err := s.verify(credential); err != nil {
			s.fail(err)
			return
		}
		s.set(credential)
		backoff = time.Second
	}
}

// loadPersisted resumes from a still-valid session file. This is the restart
// path: the single-use enrollment secret is already consumed, so no exchange
// is attempted when the persisted session verifies.
func (s *AgentSessionTokenSource) loadPersisted() bool {
	s.mu.Lock()
	store, path := s.store, s.sessionPath
	s.mu.Unlock()
	if store == nil || path == "" {
		return false
	}
	credential, ok, err := store.Load(path)
	if err != nil || !ok || !credential.ExpiresAt.After(s.clock.Now()) {
		return false
	}
	if err := s.verify(credential); err != nil {
		return false
	}
	s.set(credential)
	return true
}

func (s *AgentSessionTokenSource) exchange() bool {
	backoff := time.Second
	for {
		credential, err := s.issuer.ExchangeAgentSession(s.ctx, s.enrollmentSecret, s.tenantID, s.targetID)
		if err == nil {
			if err := s.verify(credential); err != nil {
				s.fail(err)
				return false
			}
			s.set(credential)
			return true
		}
		if isPermanentAgentSessionError(err) {
			s.fail(err)
			return false
		}
		if !s.wait(backoff) {
			s.fail(ErrServiceCredentialUnavailable)
			return false
		}
		if backoff < 4*time.Second {
			backoff *= 2
		}
	}
}

func (s *AgentSessionTokenSource) verify(credential AgentSessionCredential) error {
	claims, err := s.verifier.VerifyClaims(s.ctx, credential.Credential, s.tenantID, s.targetID)
	if err != nil || claims.SessionID == "" || claims.SessionID != credential.SessionID || claims.Generation != credential.Generation {
		return ErrServiceCredentialRejected
	}
	return nil
}

func (s *AgentSessionTokenSource) set(credential AgentSessionCredential) {
	s.mu.Lock()
	s.value, s.expiry, s.lastErr = credential.Credential, credential.ExpiresAt, nil
	store, path := s.store, s.sessionPath
	s.mu.Unlock()
	if store != nil && path != "" {
		_ = store.Save(path, credential)
	}
	s.notify()
}

func (s *AgentSessionTokenSource) fail(err error) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
	s.notify()
}

func (s *AgentSessionTokenSource) wait(delay time.Duration) bool {
	if delay <= 0 {
		delay = time.Millisecond
	}
	select {
	case <-s.clock.After(delay):
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *AgentSessionTokenSource) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func isPermanentAgentSessionError(err error) bool {
	return errors.Is(err, ErrServiceIdentityRevoked) || errors.Is(err, ErrServiceCredentialRejected)
}
