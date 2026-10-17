package iam

import (
	"context"
	"errors"
	"math/rand/v2"
	"regexp"
	"sync"
	"time"
)

const ServiceCredentialRPCTimeout = 2 * time.Second

var (
	ErrServiceCredentialUnavailable = errors.New("service credential unavailable")
	ErrServiceCredentialRejected    = errors.New("service credential rejected")
	ErrServiceIdentityRevoked       = errors.New("service identity revoked")
)

var serviceCredentialResourceKind = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,127}$`)

// ServiceCredentialScope binds one consumer to one exact service permission.
// Its JSON contract is identity/v1/service-credential-scope.schema.json.
// This request is not workload authentication or authority to issue a token.
type ServiceCredentialScope struct {
	SubjectID  string           `json:"subjectId"`
	Audience   string           `json:"audience"`
	Permission ScopedPermission `json:"permission"`
}

// ServiceCredentialIssuer must authenticate the workload afresh and honor ctx.
// In particular, an old access token alone must not authorize renewed access.
// Permanent revocation/rejection must use the corresponding sentinel error;
// other errors are treated as temporary and are never exposed to consumers.
type ServiceCredentialIssuer interface {
	IssueServiceCredential(context.Context, ServiceCredentialScope) (string, error)
}

type renewalClock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type serviceRenewalClock struct{ now func() time.Time }

func (c serviceRenewalClock) Now() time.Time                         { return c.now() }
func (c serviceRenewalClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// RenewingTokenSource implements TokenSource with one shared background issuer.
// Call Close or cancel parent when the consumer shuts down. It never persists
// credentials and never changes a verifier's profile, TTL, or permissions.
type RenewingTokenSource struct {
	ctx      context.Context
	cancel   context.CancelFunc
	verifier *TokenVerifier
	issuer   ServiceCredentialIssuer
	scope    ServiceCredentialScope
	clock    renewalClock
	random   func() float64
	once     sync.Once
	mu       sync.Mutex
	changed  chan struct{}
	value    string
	expiry   time.Time
	lastErr  error
}

var _ TokenSource = (*RenewingTokenSource)(nil)

func NewRenewingTokenSource(parent context.Context, verifier *TokenVerifier, issuer ServiceCredentialIssuer, scope ServiceCredentialScope) (*RenewingTokenSource, error) {
	if parent == nil || verifier == nil || issuer == nil || !validServiceCredentialScope(scope) || verifier.config.Audience != scope.Audience {
		return nil, errors.New("parent, verifier, issuer and matching exact service scope are required")
	}
	ctx, cancel := context.WithCancel(parent)
	return &RenewingTokenSource{
		ctx: ctx, cancel: cancel, verifier: verifier, issuer: issuer, scope: scope,
		clock: serviceRenewalClock{now: verifier.config.Now}, random: rand.Float64,
		changed: make(chan struct{}),
	}, nil
}

func validServiceScopeValue(value string, maximum int, required bool) bool {
	if value == "" {
		return !required
	}
	if len(value) > maximum {
		return false
	}
	for i := range value {
		if value[i] <= ' ' || value[i] >= 127 || value[i] == '*' {
			return false
		}
	}
	return true
}

func validServiceCredentialScope(scope ServiceCredentialScope) bool {
	p := scope.Permission
	return validServiceScopeValue(scope.SubjectID, 128, true) && validServiceScopeValue(scope.Audience, 256, true) &&
		validServiceScopeValue(p.TenantID, 128, true) && validServiceScopeValue(p.ResourceKind, 128, true) &&
		serviceCredentialResourceKind.MatchString(p.ResourceKind) &&
		validServiceScopeValue(p.ResourceID, 256, true) && validAction(p.Action) &&
		validServiceScopeValue(p.ProjectID, 128, false) && validServiceScopeValue(p.EnvironmentID, 128, false) &&
		validServiceScopeValue(p.NamespaceID, 253, false) && validScope(p.ProjectID, p.EnvironmentID, p.NamespaceID)
}

func (s *RenewingTokenSource) String() string   { return "RenewingTokenSource{redacted}" }
func (s *RenewingTokenSource) GoString() string { return s.String() }

func (s *RenewingTokenSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	s.once.Do(func() { go s.run() })
	for {
		s.mu.Lock()
		if err := ctx.Err(); err != nil {
			s.mu.Unlock()
			return "", err
		}
		if err := s.ctx.Err(); err != nil {
			s.mu.Unlock()
			return "", err
		}
		if s.value != "" && s.clock.Now().Before(s.expiry) {
			value := s.value
			s.mu.Unlock()
			return value, nil
		}
		if s.lastErr != nil {
			err := s.lastErr
			s.mu.Unlock()
			return "", err
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-s.ctx.Done():
			return "", s.ctx.Err()
		case <-changed:
		}
	}
}

// Close cancels in-flight issuance as well as scheduled renewal. Issuers are
// required to honor context cancellation; no goroutine is spawned per attempt.
func (s *RenewingTokenSource) Close() error {
	s.cancel()
	s.mu.Lock()
	s.value = ""
	s.notifyLocked()
	s.mu.Unlock()
	return nil
}

func (s *RenewingTokenSource) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *RenewingTokenSource) wait(delay time.Duration) bool {
	select {
	case <-s.ctx.Done():
		return false
	case <-s.clock.After(delay):
		return s.ctx.Err() == nil
	}
}

func (s *RenewingTokenSource) jitter(value time.Duration, fraction float64) time.Duration {
	return time.Duration(float64(value) * (1 + fraction*(2*s.random()-1)))
}

func (s *RenewingTokenSource) run() {
	backoff := time.Second
	for s.ctx.Err() == nil {
		s.mu.Lock()
		remaining := s.expiry.Sub(s.clock.Now())
		s.mu.Unlock()
		// Do not start an attempt that cannot finish within the old lease. After
		// expiry, fresh workload proof may reacquire access without using it.
		if remaining > 0 && remaining < ServiceCredentialRPCTimeout {
			if !s.wait(remaining) {
				return
			}
			continue
		}
		ctx, cancel := context.WithTimeout(s.ctx, ServiceCredentialRPCTimeout)
		value, err := s.issuer.IssueServiceCredential(ctx, s.scope)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		var claims *AccessTokenClaims
		if err == nil {
			claims, err = s.verifyCredential(ctx, value)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		cancel()
		if s.ctx.Err() != nil {
			return
		}
		permanent := errors.Is(err, ErrServiceIdentityRevoked) || errors.Is(err, ErrServiceCredentialRejected)
		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		switch {
		case err == nil:
			s.value, s.expiry, s.lastErr = value, time.Unix(claims.ExpiresAt, 0), nil
		case permanent:
			s.value = ""
			if errors.Is(err, ErrServiceIdentityRevoked) {
				s.lastErr = ErrServiceIdentityRevoked
			} else {
				s.lastErr = ErrServiceCredentialRejected
			}
		default:
			s.lastErr = ErrServiceCredentialUnavailable
		}
		s.notifyLocked()
		expiry := s.expiry
		s.mu.Unlock()
		if permanent {
			return
		}
		var delay time.Duration
		if err == nil {
			backoff = time.Second
			issuedAt := time.Unix(claims.IssuedAt, 0)
			// +/-10% of half the lifetime is +/-5% of the full lifetime.
			delay = issuedAt.Add(s.jitter(expiry.Sub(issuedAt)/2, 0.1)).Sub(s.clock.Now())
			// A still-valid cached issuer response must not cause a busy loop.
			if delay < time.Second {
				delay = time.Second
			}
		} else {
			delay = s.jitter(backoff, 0.2)
			if delay > 4*time.Second {
				delay = 4 * time.Second
			}
			if backoff < 4*time.Second {
				backoff *= 2
			}
		}
		remaining = expiry.Sub(s.clock.Now())
		if remaining > 0 && delay+ServiceCredentialRPCTimeout > remaining {
			delay = remaining
		}
		if !s.wait(delay) {
			return
		}
	}
}

func (s *RenewingTokenSource) verifyCredential(ctx context.Context, value string) (*AccessTokenClaims, error) {
	claims, err := s.verifier.Verify(ctx, value)
	if err != nil || (claims.SubjectType != "service" && claims.SubjectType != "workload") ||
		claims.SubjectID != s.scope.SubjectID || claims.TenantID != s.scope.Permission.TenantID ||
		len(claims.Audiences) != 1 || claims.Audiences[0] != s.scope.Audience ||
		len(claims.AllowedActions) != 1 || claims.AllowedActions[0] != s.scope.Permission.Action ||
		len(claims.ScopedPermissions) != 1 || claims.ScopedPermissions[0] != s.scope.Permission {
		return nil, ErrServiceCredentialRejected
	}
	return claims, nil
}
