package iam

import (
	"context"
	"errors"
	"sync"
)

// TenantTokenSource supplies a credential for one execution tenant. Providers
// that serve multiple tenants must never reuse a tenant-A credential for
// tenant-B work; the backing source is bound to an exact scope per tenant.
type TenantTokenSource interface {
	TokenForTenant(context.Context, string) (string, error)
}

type TenantCredentialFactory func(tenantID string) (*TokenVerifier, ServiceCredentialIssuer, ServiceCredentialScope, error)

// TenantRenewingTokenSource lazily creates one fixed-scope RenewingTokenSource
// per tenant. It is the multi-tenant worker adapter: it never widens a cached
// credential across tenants and never falls back to a shared static token.
type TenantRenewingTokenSource struct {
	parent  context.Context
	factory TenantCredentialFactory
	mu      sync.Mutex
	sources map[string]*RenewingTokenSource
	closed  bool
}

func NewTenantRenewingTokenSource(parent context.Context, factory TenantCredentialFactory) (*TenantRenewingTokenSource, error) {
	if parent == nil || factory == nil {
		return nil, errors.New("tenant token source requires a parent context and factory")
	}
	return &TenantRenewingTokenSource{parent: parent, factory: factory, sources: make(map[string]*RenewingTokenSource)}, nil
}

func (s *TenantRenewingTokenSource) TokenForTenant(ctx context.Context, tenantID string) (string, error) {
	if !boundedClaim(tenantID, 128) || tenantID == "*" {
		return "", ErrServiceCredentialRejected
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", ErrServiceCredentialUnavailable
	}
	source := s.sources[tenantID]
	if source == nil {
		verifier, issuer, scope, err := s.factory(tenantID)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		if verifier == nil || issuer == nil {
			s.mu.Unlock()
			return "", ErrServiceCredentialUnavailable
		}
		if scope.SubjectID == "" || scope.Audience == "" || scope.Permission.TenantID != tenantID {
			s.mu.Unlock()
			return "", ErrServiceCredentialRejected
		}
		source, err = NewRenewingTokenSource(s.parent, verifier, issuer, scope)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		s.sources[tenantID] = source
	}
	s.mu.Unlock()
	return source.Token(ctx)
}

func (s *TenantRenewingTokenSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	sources := make([]*RenewingTokenSource, 0, len(s.sources))
	for _, source := range s.sources {
		sources = append(sources, source)
	}
	s.sources = nil
	s.mu.Unlock()
	var result error
	for _, source := range sources {
		result = errors.Join(result, source.Close())
	}
	return result
}

// Token refuses to pick a tenant. Callers must use TokenForTenant so a
// provider call can never accidentally borrow another tenant's credential.
func (s *TenantRenewingTokenSource) Token(context.Context) (string, error) {
	return "", ErrServiceCredentialRejected
}

var _ TenantTokenSource = (*TenantRenewingTokenSource)(nil)
var _ TokenSource = (*TenantRenewingTokenSource)(nil)
