package iam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ServiceIdentityInput is the owning-plane registration payload. The tenant and
// permission tenant must match the authenticated actor's tenant.
type ServiceIdentityInput struct {
	TenantID   string
	SubjectID  string
	Audience   string
	Permission ScopedPermission
}

type IssuedServiceCredential struct {
	Credential        string
	CredentialID      string
	ServiceIdentityID string
	SubjectID         string
	TenantID          string
	Audience          string
	Permission        ScopedPermission
	IssuedAt          time.Time
	ExpiresAt         time.Time
}

type VerifiedServiceCredential struct {
	Claims     AccessTokenClaims
	Identity   ServiceIdentityRecord
	Credential ServiceCredentialRecord
}

type ServiceCredentialAuditEvent struct {
	Action            string
	ActorSubjectID    string
	ActorSubjectType  string
	ServiceSubjectID  string
	TenantID          string
	ServiceIdentityID string
	CredentialID      string
	Reason            string
}

type ServiceCredentialAuditFunc func(context.Context, ServiceCredentialAuditEvent)

// ServiceIdentityValidationError marks a caller input error that is safe to
// return as a bounded validation message. It never carries credential material.
type ServiceIdentityValidationError struct{ Message string }

func (e ServiceIdentityValidationError) Error() string { return e.Message }

func validationError(message string) error { return ServiceIdentityValidationError{Message: message} }

// ServiceCredentialAuthority is the apiserver-side issuance, renewal,
// verification, and revocation boundary. It signs short-lived access tokens
// with the same IAM key provider as the user token manager. Issuance is
// recorded before the credential is returned so revocation has an authority.
type ServiceCredentialAuthority struct {
	store      ServiceIdentityStore
	keys       KeyProvider
	keyRing    KeyRing
	issuer     string
	defaultTTL time.Duration
	maxTTL     time.Duration
	now        func() time.Time
	audit      ServiceCredentialAuditFunc
}

func NewServiceCredentialAuthority(
	store ServiceIdentityStore,
	keys KeyProvider,
	keyRing KeyRing,
	issuer string,
	now func() time.Time,
	audit ServiceCredentialAuditFunc,
) (*ServiceCredentialAuthority, error) {
	if store == nil || keys == nil || keyRing == nil {
		return nil, errors.New("service identity store, signing key provider, and key ring are required")
	}
	if !boundedClaim(issuer, 256) {
		return nil, errors.New("token issuer is required")
	}
	if now == nil {
		now = time.Now
	}
	if audit == nil {
		audit = func(context.Context, ServiceCredentialAuditEvent) {}
	}
	return &ServiceCredentialAuthority{
		store: store, keys: keys, keyRing: keyRing, issuer: issuer,
		defaultTTL: DefaultServiceCredentialTTL, maxTTL: MaxServiceCredentialTTL,
		now: now, audit: audit,
	}, nil
}

func (a *ServiceCredentialAuthority) CreateIdentity(ctx context.Context, actor TrustedContext, input ServiceIdentityInput) (*ServiceIdentityRecord, error) {
	if !boundedClaim(actor.TenantID, 128) || actor.TenantID == "*" || input.TenantID != actor.TenantID {
		return nil, validationError("service identity tenant must match the authenticated tenant")
	}
	if _, err := uuid.Parse(input.SubjectID); err != nil {
		return nil, validationError("service identity subject must be a UUID")
	}
	if !boundedClaim(input.Audience, 256) || input.Audience == "*" {
		return nil, validationError("service identity requires one exact audience")
	}
	if input.Permission.TenantID != input.TenantID {
		return nil, validationError("permission tenant must match the service identity tenant")
	}
	if err := ValidatePermissionSnapshot("serviceIdentity", []ScopedPermission{input.Permission}, input.TenantID); err != nil {
		return nil, validationError("invalid service identity permission")
	}
	now := a.now().UTC()
	created, err := a.store.CreateServiceIdentity(ctx, ServiceIdentityRecord{
		ID: uuid.NewString(), TenantID: input.TenantID, SubjectID: input.SubjectID,
		Audience: input.Audience, Permission: input.Permission, Status: ServiceIdentityStatusActive,
		Version: 1, CreatedBySubjectID: actor.SubjectID, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	a.audit(ctx, ServiceCredentialAuditEvent{
		Action: "serviceIdentity.create", ActorSubjectID: actor.SubjectID,
		ActorSubjectType: actor.SubjectType, ServiceSubjectID: created.SubjectID,
		TenantID: created.TenantID, ServiceIdentityID: created.ID,
	})
	return created, nil
}

func (a *ServiceCredentialAuthority) Issue(ctx context.Context, actor TrustedContext, identityID string, ttl time.Duration) (*IssuedServiceCredential, error) {
	identity, err := a.store.GetServiceIdentity(ctx, actor.TenantID, identityID)
	if err != nil {
		return nil, err
	}
	if identity.Status != ServiceIdentityStatusActive {
		return nil, ErrServiceIdentityRevoked
	}
	if actor.SubjectID != identity.SubjectID && actor.SubjectType != "user" {
		return nil, ErrServiceCredentialRejected
	}
	issued, err := a.issue(ctx, identity, a.clampTTL(ttl))
	if err != nil {
		return nil, err
	}
	a.audit(ctx, ServiceCredentialAuditEvent{
		Action: "serviceIdentity.issue", ActorSubjectID: actor.SubjectID,
		ActorSubjectType: actor.SubjectType, ServiceSubjectID: identity.SubjectID,
		TenantID: identity.TenantID, ServiceIdentityID: identity.ID,
		CredentialID: issued.CredentialID,
	})
	return issued, nil
}

// Renew accepts only a still-valid credential issued for an exact registered
// identity. The presented credential cannot broaden scope; the new credential
// is minted from the stored identity, never from the token payload.
func (a *ServiceCredentialAuthority) Renew(ctx context.Context, presented string) (*IssuedServiceCredential, error) {
	verified, err := a.Verify(ctx, presented)
	if err != nil {
		return nil, err
	}
	issued, err := a.issue(ctx, &verified.Identity, a.defaultTTL)
	if err != nil {
		return nil, err
	}
	a.audit(ctx, ServiceCredentialAuditEvent{
		Action: "serviceIdentity.renew", ActorSubjectID: verified.Identity.SubjectID,
		ActorSubjectType: "service", ServiceSubjectID: verified.Identity.SubjectID,
		TenantID: verified.Identity.TenantID, ServiceIdentityID: verified.Identity.ID,
		CredentialID: issued.CredentialID,
	})
	return issued, nil
}

// Verify checks signature, exact profile/audience/scope claims, issuance state,
// identity state, and credential revocation. A revoked identity or credential
// is rejected even while its JWT would still verify offline.
func (a *ServiceCredentialAuthority) Verify(ctx context.Context, token string) (*VerifiedServiceCredential, error) {
	parsed, err := parseUnverifiedAccessToken(token)
	if err != nil {
		return nil, ErrServiceCredentialRejected
	}
	status, err := a.store.GetServiceCredentialStatus(ctx, parsed.TenantID, parsed.TokenID)
	if err != nil {
		return nil, ErrServiceCredentialRejected
	}
	if status.Credential.SubjectID != parsed.SubjectID || status.Credential.Audience != parsed.Audience[0] {
		return nil, ErrServiceCredentialRejected
	}
	verifier, err := NewTokenVerifier(TokenManagerConfig{
		Issuer: a.issuer, Audience: status.Identity.Audience, AccessTTL: MaxAccessTokenTTL, Now: a.now,
	}, a.keyRing)
	if err != nil {
		return nil, ErrServiceCredentialRejected
	}
	claims, err := verifier.Verify(ctx, token)
	if err != nil {
		return nil, ErrServiceCredentialRejected
	}
	if status.Identity.Status != ServiceIdentityStatusActive {
		return nil, ErrServiceIdentityRevoked
	}
	if status.Credential.RevokedAt != nil || !status.Credential.ExpiresAt.After(a.now().UTC()) {
		return nil, ErrServiceCredentialRejected
	}
	membershipID := "service:" + status.Identity.ID
	policyVersion := fmt.Sprintf("serviceIdentity:%d", status.Identity.Version)
	if claims.TokenID != status.Credential.ID || claims.SubjectID != status.Identity.SubjectID ||
		claims.TenantID != status.Identity.TenantID || claims.MembershipID != membershipID ||
		claims.PolicyVersion != policyVersion || len(claims.Audiences) != 1 ||
		claims.Audiences[0] != status.Identity.Audience || len(claims.ScopedPermissions) != 1 ||
		claims.ScopedPermissions[0] != status.Identity.Permission || len(claims.AllowedActions) != 1 ||
		claims.AllowedActions[0] != status.Identity.Permission.Action {
		return nil, ErrServiceCredentialRejected
	}
	return &VerifiedServiceCredential{
		Claims: *claims, Identity: status.Identity, Credential: status.Credential,
	}, nil
}

func (a *ServiceCredentialAuthority) RevokeIdentity(ctx context.Context, actor TrustedContext, identityID string, expectedVersion int64, reason string) (*ServiceIdentityRecord, error) {
	if actor.SubjectType != "user" {
		return nil, ErrServiceCredentialRejected
	}
	if expectedVersion <= 0 {
		return nil, validationError("positive expected version is required")
	}
	identity, err := a.store.RevokeServiceIdentity(ctx, actor.TenantID, identityID, expectedVersion, actor.SubjectID, boundedReason(reason), a.now().UTC())
	if err != nil {
		return nil, err
	}
	a.audit(ctx, ServiceCredentialAuditEvent{
		Action: "serviceIdentity.revoke", ActorSubjectID: actor.SubjectID,
		ActorSubjectType: actor.SubjectType, ServiceSubjectID: identity.SubjectID,
		TenantID: identity.TenantID, ServiceIdentityID: identity.ID, Reason: identity.RevocationReason,
	})
	return identity, nil
}

func (a *ServiceCredentialAuthority) RevokeCredential(ctx context.Context, actor TrustedContext, identityID, credentialID, reason string) error {
	if actor.SubjectType != "user" {
		return ErrServiceCredentialRejected
	}
	if _, err := uuid.Parse(credentialID); err != nil {
		return validationError("credential ID must be a UUID")
	}
	status, err := a.store.GetServiceCredentialStatus(ctx, actor.TenantID, credentialID)
	if err != nil {
		return err
	}
	if status.Credential.ServiceIdentityID != identityID {
		return ErrServiceCredentialNotFound
	}
	if err := a.store.RevokeServiceCredential(ctx, actor.TenantID, identityID, credentialID, actor.SubjectID, boundedReason(reason), a.now().UTC()); err != nil {
		return err
	}
	a.audit(ctx, ServiceCredentialAuditEvent{
		Action: "serviceIdentity.credential.revoke", ActorSubjectID: actor.SubjectID,
		ActorSubjectType: actor.SubjectType, ServiceSubjectID: status.Identity.SubjectID,
		TenantID: actor.TenantID, ServiceIdentityID: identityID, CredentialID: credentialID,
		Reason: boundedReason(reason),
	})
	return nil
}

func (a *ServiceCredentialAuthority) issue(ctx context.Context, identity *ServiceIdentityRecord, ttl time.Duration) (*IssuedServiceCredential, error) {
	now := a.now().UTC()
	kid, key, err := a.keys.CurrentSigningKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("load signing key: %w", err)
	}
	membershipID := "service:" + identity.ID
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: a.issuer,
		Audiences: []string{identity.Audience}, SubjectID: identity.SubjectID,
		SubjectType: "service", TenantID: identity.TenantID, MembershipID: membershipID,
		TenantMembershipIDs: []string{membershipID},
		PolicyVersion:       fmt.Sprintf("serviceIdentity:%d", identity.Version),
		ScopedPermissions:   []ScopedPermission{identity.Permission},
		AllowedActions:      []AuthorizationAction{identity.Permission.Action},
		IssuedAt:            now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
		AuthTime: now.Unix(), TokenID: uuid.NewString(), KeyID: kid, Algorithm: AccessTokenAlgorithm,
	}
	if err := ValidatePermissionSnapshot(claims.PolicyVersion, claims.ScopedPermissions, claims.TenantID); err != nil {
		return nil, err
	}
	value, err := signAccessToken(key, kid, claims)
	if err != nil {
		return nil, err
	}
	record := ServiceCredentialRecord{
		ID: claims.TokenID, ServiceIdentityID: identity.ID, TenantID: identity.TenantID,
		SubjectID: identity.SubjectID, Audience: identity.Audience,
		IssuedAt: now, ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}
	if err := a.store.RegisterServiceCredential(ctx, record); err != nil {
		return nil, err
	}
	return &IssuedServiceCredential{
		Credential: value, CredentialID: record.ID, ServiceIdentityID: identity.ID,
		SubjectID: identity.SubjectID, TenantID: identity.TenantID, Audience: identity.Audience,
		Permission: identity.Permission, IssuedAt: now, ExpiresAt: record.ExpiresAt,
	}, nil
}

func (a *ServiceCredentialAuthority) clampTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return a.defaultTTL
	}
	if ttl > a.maxTTL {
		return a.maxTTL
	}
	if ttl < time.Second {
		return time.Second
	}
	return ttl
}

func boundedReason(reason string) string {
	if len(reason) > 512 {
		return reason[:512]
	}
	return reason
}

type unverifiedAccessToken struct {
	Audience          []string              `json:"aud"`
	SubjectID         string                `json:"sub"`
	TenantID          string                `json:"tenantId"`
	TokenID           string                `json:"jti"`
	Issuer            string                `json:"iss"`
	ScopedPermissions []ScopedPermission    `json:"scopedPermissions"`
	AllowedActions    []AuthorizationAction `json:"allowedActions"`
}

// parseUnverifiedAccessToken extracts only the lookup keys needed to find the
// authoritative issuance row. Nothing from this parse is trusted; every field
// is recomputed from the verified claims and the stored identity afterwards.
func parseUnverifiedAccessToken(token string) (*unverifiedAccessToken, error) {
	if len(token) == 0 || len(token) > MaxAccessTokenSize {
		return nil, errors.New("invalid token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[1] == "" {
		return nil, errors.New("invalid compact JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid JWT payload")
	}
	var parsed unverifiedAccessToken
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, errors.New("invalid JWT payload")
	}
	if len(parsed.Audience) != 1 || !boundedClaim(parsed.Audience[0], 256) || parsed.Audience[0] == "*" ||
		!boundedClaim(parsed.SubjectID, 128) || !boundedClaim(parsed.TenantID, 128) || parsed.TenantID == "*" ||
		!boundedClaim(parsed.TokenID, 128) || !boundedClaim(parsed.Issuer, 256) {
		return nil, errors.New("invalid JWT lookup claims")
	}
	if _, err := uuid.Parse(parsed.SubjectID); err != nil {
		return nil, errors.New("invalid JWT subject")
	}
	if _, err := uuid.Parse(parsed.TokenID); err != nil {
		return nil, errors.New("invalid JWT token ID")
	}
	return &parsed, nil
}
