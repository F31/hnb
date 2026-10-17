package iam

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ServiceIdentityBinding is the deployment-side declaration for one tenant:
// the registered service identity, its canonical subject, and the bootstrap
// credential file. The bootstrap file is read once; later renewals use the
// returned credential and never fall back to the file.
type ServiceIdentityBinding struct {
	IdentityID string
	SubjectID  string
	TokenFile  string
}

// NewHTTPTenantTokenSource builds the multi-tenant worker source for one
// Provider audience. Every tenant must have an explicit binding; an unbound
// tenant is rejected rather than inheriting another tenant's credential.
func NewHTTPTenantTokenSource(
	parent context.Context,
	verifier *TokenVerifier,
	baseURL, providerID, audience string,
	bindings map[string]ServiceIdentityBinding,
	now func() time.Time,
) (*TenantRenewingTokenSource, error) {
	if verifier == nil {
		return nil, errors.New("service identity verifier is required")
	}
	if !boundedClaim(providerID, 128) || providerID == "*" {
		return nil, errors.New("exact runtime provider ID is required")
	}
	if !boundedClaim(audience, 256) || audience == "*" {
		return nil, errors.New("exact provider audience is required")
	}
	if len(bindings) == 0 {
		return nil, errors.New("at least one tenant service identity binding is required")
	}
	for tenantID, binding := range bindings {
		if !boundedClaim(tenantID, 128) || tenantID == "*" {
			return nil, errors.New("service identity binding has an invalid tenant")
		}
		if _, err := uuid.Parse(binding.IdentityID); err != nil {
			return nil, errors.New("service identity binding requires a UUID identity ID")
		}
		if _, err := uuid.Parse(binding.SubjectID); err != nil {
			return nil, errors.New("service identity binding requires a UUID subject ID")
		}
		if strings.TrimSpace(binding.TokenFile) == "" {
			return nil, errors.New("service identity binding requires a bootstrap token file")
		}
	}
	return NewTenantRenewingTokenSource(parent, func(tenantID string) (*TokenVerifier, ServiceCredentialIssuer, ServiceCredentialScope, error) {
		binding, ok := bindings[tenantID]
		if !ok {
			return nil, nil, ServiceCredentialScope{}, ErrServiceCredentialRejected
		}
		issuer, err := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{
			BaseURL: baseURL, Bootstrap: FileTokenSource{Path: binding.TokenFile}, Now: now,
		})
		if err != nil {
			return nil, nil, ServiceCredentialScope{}, err
		}
		scope := ServiceCredentialScope{
			SubjectID: binding.SubjectID, Audience: audience,
			Permission: ScopedPermission{
				ResourceKind: "runtimeProvider", ResourceID: providerID,
				Action: ActionExecute, TenantID: tenantID,
			},
		}
		return verifier, issuer, scope, nil
	})
}
