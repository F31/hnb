package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
)

const (
	AccessTokenProfileVersion = "hnb.identity/v1"
	AccessTokenType           = "at+jwt"
	AccessTokenAlgorithm      = "ES256"
	MaxAccessTokenSize        = 8192
	MaxAccessTokenTTL         = 60 * time.Second
)

var (
	ErrNoAuthorizedTenant = errors.New("no authorized tenant membership")
	ErrMembershipMismatch = errors.New("membership does not belong to subject")
	ErrRefreshReplay      = errors.New("refresh token is invalid or already used")
)

// AccessTokenClaims is the single versioned access-token profile used by both
// issuance and verification. Registered JWT names are retained on the wire.
type AccessTokenClaims struct {
	ProfileVersion      string                `json:"profileVersion"`
	Issuer              string                `json:"iss"`
	Audiences           []string              `json:"aud"`
	SubjectID           string                `json:"sub"`
	SubjectType         string                `json:"type"`
	TenantID            string                `json:"tenantId"`
	MembershipID        string                `json:"membershipId"`
	TenantMembershipIDs []string              `json:"tenantMembershipIds"`
	PolicyVersion       string                `json:"policyVersion"`
	ScopedPermissions   []ScopedPermission    `json:"scopedPermissions"`
	AllowedActions      []AuthorizationAction `json:"allowedActions"`
	IssuedAt            int64                 `json:"iat"`
	NotBefore           int64                 `json:"nbf"`
	ExpiresAt           int64                 `json:"exp"`
	AuthTime            int64                 `json:"auth_time"`
	TokenID             string                `json:"jti"`
	KeyID               string                `json:"keyId"`
	Algorithm           string                `json:"algorithm"`
}

type TrustedContext struct {
	ProfileVersion    string
	SubjectID         string
	SubjectType       string
	TenantID          string
	MembershipID      string
	PolicyVersion     string
	ScopedPermissions []ScopedPermission
	ProjectID         string
	EnvironmentID     string
	NamespaceID       string
	CorrelationID     string
	Traceparent       string
	TokenID           string
	AuthTime          time.Time
	ExpiresAt         time.Time
}

type trustedContextKey struct{}
type rawAccessTokenKey struct{}

func WithTrustedContext(ctx context.Context, trusted TrustedContext) context.Context {
	return context.WithValue(ctx, trustedContextKey{}, trusted)
}

func TrustedContextFrom(ctx context.Context) (TrustedContext, bool) {
	trusted, ok := ctx.Value(trustedContextKey{}).(TrustedContext)
	return trusted, ok
}

func WithRawAccessToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, rawAccessTokenKey{}, token)
}

func RawAccessTokenFrom(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(rawAccessTokenKey{}).(string)
	return token, ok && token != ""
}

type KeyProvider interface {
	CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error)
}

type KeyRing interface {
	VerificationKey(context.Context, string) (*ecdsa.PublicKey, error)
}

type Identity struct {
	UserID       string
	SubjectID    string
	SubjectType  string
	MembershipID string
	TenantID     string
}

type IdentityResolver interface {
	ResolveUserIdentity(context.Context, string, string) (*Identity, error)
	ResolveMembership(context.Context, string, string) (*Identity, error)
}

type PermissionResolver interface {
	ResolvePermissions(context.Context, string, string, string) (string, []ScopedPermission, error)
}

type RefreshTokenRecord struct {
	TokenHash    string
	SubjectID    string
	MembershipID string
	ExpiresAt    time.Time
}

type RefreshTokenStore interface {
	CreateRefreshToken(context.Context, RefreshTokenRecord) error
	RotateRefreshToken(context.Context, string, RefreshTokenRecord, time.Time) (*RefreshTokenRecord, error)
}

type TokenManagerConfig struct {
	Issuer     string
	Audience   string
	Audiences  []string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time
}

type TokenManager struct {
	config       TokenManagerConfig
	verifier     *TokenVerifier
	keyProvider  KeyProvider
	keyRing      KeyRing
	identities   IdentityResolver
	permissions  PermissionResolver
	refreshStore RefreshTokenStore
}

// TokenVerifier verifies access tokens and derives trusted context without a
// signing key, refresh store, identity database, or other IAM state.
type TokenVerifier struct {
	config  TokenManagerConfig
	keyRing KeyRing
}

func NewTokenVerifier(config TokenManagerConfig, keyRing KeyRing) (*TokenVerifier, error) {
	if config.Issuer == "" || config.Audience == "" || config.Audience == "*" {
		return nil, errors.New("issuer and a non-wildcard audience are required")
	}
	if config.AccessTTL < time.Second || config.AccessTTL > MaxAccessTokenTTL {
		return nil, errors.New("access token TTL must be between 1 and 60 seconds")
	}
	if keyRing == nil {
		return nil, errors.New("key ring is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &TokenVerifier{config: config, keyRing: keyRing}, nil
}

func NewTokenManager(config TokenManagerConfig, keyProvider KeyProvider, keyRing KeyRing, identities IdentityResolver, permissions PermissionResolver, refreshStore RefreshTokenStore) (*TokenManager, error) {
	verifier, err := NewTokenVerifier(config, keyRing)
	if err != nil {
		return nil, err
	}
	if config.RefreshTTL < time.Second {
		return nil, errors.New("positive refresh token TTL is required")
	}
	if keyProvider == nil || keyRing == nil || identities == nil || permissions == nil || refreshStore == nil {
		return nil, errors.New("key provider, key ring, identity, permission, and refresh stores are required")
	}
	if len(config.Audiences) == 0 || !contains(config.Audiences, config.Audience) || !validAudiences(config.Audiences) {
		return nil, errors.New("explicit non-wildcard token audiences including the verifier audience are required")
	}
	config.Now = verifier.config.Now
	return &TokenManager{config: config, verifier: verifier, keyProvider: keyProvider, keyRing: keyRing, identities: identities, permissions: permissions, refreshStore: refreshStore}, nil
}

func (tm *TokenManager) Issue(ctx context.Context, userID, membershipSelector string) (*Token, *Token, error) {
	identity, err := tm.identities.ResolveUserIdentity(ctx, userID, membershipSelector)
	if err != nil {
		return nil, nil, err
	}
	return tm.issue(ctx, identity, tm.config.Now().UTC())
}

func (tm *TokenManager) issue(ctx context.Context, identity *Identity, authTime time.Time) (*Token, *Token, error) {
	now := tm.config.Now().UTC()
	kid, privateKey, err := tm.keyProvider.CurrentSigningKey(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load signing key: %w", err)
	}
	if kid == "" || privateKey == nil || privateKey.Curve.Params().Name != "P-256" {
		return nil, nil, errors.New("active signing key must be a named P-256 key")
	}
	policyVersion, permissions, err := tm.permissions.ResolvePermissions(ctx, identity.SubjectID, identity.MembershipID, identity.TenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve permission snapshot: %w", err)
	}
	if err := ValidatePermissionSnapshot(policyVersion, permissions, identity.TenantID); err != nil {
		return nil, nil, err
	}
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: tm.config.Issuer,
		Audiences: append([]string(nil), tm.config.Audiences...), SubjectID: identity.SubjectID,
		SubjectType: identity.SubjectType, TenantID: identity.TenantID, MembershipID: identity.MembershipID,
		TenantMembershipIDs: []string{identity.MembershipID},
		PolicyVersion:       policyVersion, ScopedPermissions: append([]ScopedPermission{}, permissions...),
		AllowedActions: permissionActions(permissions),
		IssuedAt:       now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(tm.config.AccessTTL).Unix(),
		AuthTime: authTime.Unix(), TokenID: randomID(), KeyID: kid, Algorithm: AccessTokenAlgorithm,
	}
	encoded, err := signAccessToken(privateKey, kid, claims)
	if err != nil {
		return nil, nil, err
	}
	if len(encoded) > MaxAccessTokenSize {
		return nil, nil, errors.New("permission snapshot exceeds access token size")
	}
	refreshValue, err := randomOpaqueToken()
	if err != nil {
		return nil, nil, err
	}
	refresh := RefreshTokenRecord{
		TokenHash: hashRefreshToken(refreshValue), SubjectID: identity.SubjectID,
		MembershipID: identity.MembershipID, ExpiresAt: now.Add(tm.config.RefreshTTL),
	}
	if err := tm.refreshStore.CreateRefreshToken(ctx, refresh); err != nil {
		return nil, nil, fmt.Errorf("store refresh token: %w", err)
	}
	return &Token{ID: claims.TokenID, UserID: identity.SubjectID, Token: encoded, ExpiresAt: time.Unix(claims.ExpiresAt, 0), CreatedAt: now},
		&Token{Token: refreshValue, ExpiresAt: refresh.ExpiresAt, CreatedAt: now}, nil
}

func (tm *TokenManager) Verify(ctx context.Context, token string) (*AccessTokenClaims, error) {
	return tm.verifier.Verify(ctx, token)
}

func (v *TokenVerifier) Verify(ctx context.Context, token string) (*AccessTokenClaims, error) {
	if len(token) == 0 || len(token) > MaxAccessTokenSize {
		return nil, errors.New("invalid token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("invalid compact JWT")
	}
	var header struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeStrictJSON(parts[0], &header); err != nil {
		return nil, fmt.Errorf("invalid JWT header: %w", err)
	}
	if header.Type != AccessTokenType || header.Algorithm != AccessTokenAlgorithm || header.KeyID == "" {
		return nil, errors.New("unsupported JWT header")
	}
	publicKey, err := v.keyRing.VerificationKey(ctx, header.KeyID)
	if err != nil || publicKey == nil {
		return nil, errors.New("unknown signing key")
	}
	if publicKey.Curve.Params().Name != "P-256" {
		return nil, errors.New("verification key is not P-256")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, errors.New("invalid ES256 signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(publicKey, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return nil, errors.New("invalid signature")
	}
	var claims AccessTokenClaims
	if err := decodeStrictJSON(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("invalid JWT claims: %w", err)
	}
	now := v.config.Now().UTC().Unix()
	if claims.ProfileVersion != AccessTokenProfileVersion || claims.Issuer != v.config.Issuer ||
		!contains(claims.Audiences, v.config.Audience) || !validAudiences(claims.Audiences) || claims.KeyID != header.KeyID ||
		claims.Algorithm != header.Algorithm || claims.SubjectID == "" || !validSubjectType(claims.SubjectType) ||
		!boundedClaim(claims.TenantID, 128) || !boundedClaim(claims.MembershipID, 128) ||
		!validMemberships(claims.TenantMembershipIDs, claims.MembershipID) || claims.TokenID == "" ||
		claims.IssuedAt <= 0 || claims.NotBefore <= 0 || claims.ExpiresAt <= 0 || claims.AuthTime <= 0 ||
		claims.IssuedAt > now || claims.AuthTime > claims.IssuedAt || claims.NotBefore > now || claims.ExpiresAt <= now ||
		claims.ExpiresAt <= claims.IssuedAt || time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > v.config.AccessTTL {
		return nil, errors.New("access token claims failed validation")
	}
	if err := ValidatePermissionSnapshot(claims.PolicyVersion, claims.ScopedPermissions, claims.TenantID); err != nil {
		return nil, errors.New("access token permission snapshot failed validation")
	}
	if !validAllowedActions(claims.AllowedActions, claims.ScopedPermissions) {
		return nil, errors.New("access token allowed actions failed validation")
	}
	return &claims, nil
}

func (tm *TokenManager) Authenticate(ctx context.Context, token, correlationID, traceparent string) (TrustedContext, error) {
	trusted, err := tm.verifier.Authenticate(ctx, token, correlationID, traceparent)
	if err != nil {
		return TrustedContext{}, err
	}
	identity, err := tm.identities.ResolveMembership(ctx, trusted.SubjectID, trusted.MembershipID)
	if err != nil {
		return TrustedContext{}, err
	}
	if identity.SubjectID != trusted.SubjectID || identity.SubjectType != trusted.SubjectType ||
		identity.MembershipID != trusted.MembershipID || identity.TenantID != trusted.TenantID {
		return TrustedContext{}, ErrMembershipMismatch
	}
	return trusted, nil
}

func (v *TokenVerifier) Authenticate(ctx context.Context, token, correlationID, traceparent string) (TrustedContext, error) {
	claims, err := v.Verify(ctx, token)
	if err != nil {
		return TrustedContext{}, err
	}
	return TrustedContext{
		ProfileVersion: claims.ProfileVersion,
		SubjectID:      claims.SubjectID, SubjectType: claims.SubjectType,
		TenantID: claims.TenantID, MembershipID: claims.MembershipID,
		PolicyVersion: claims.PolicyVersion, ScopedPermissions: append([]ScopedPermission{}, claims.ScopedPermissions...),
		CorrelationID: correlationID, Traceparent: traceparent,
		TokenID: claims.TokenID, AuthTime: time.Unix(claims.AuthTime, 0).UTC(),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}, nil
}

func (tm *TokenManager) Refresh(ctx context.Context, refreshValue string) (*Token, *Token, error) {
	if len(refreshValue) < 43 || len(refreshValue) > 512 {
		return nil, nil, ErrRefreshReplay
	}
	newValue, err := randomOpaqueToken()
	if err != nil {
		return nil, nil, err
	}
	now := tm.config.Now().UTC()
	newRecord := RefreshTokenRecord{TokenHash: hashRefreshToken(newValue), ExpiresAt: now.Add(tm.config.RefreshTTL)}
	oldRecord, err := tm.refreshStore.RotateRefreshToken(ctx, hashRefreshToken(refreshValue), newRecord, now)
	if err != nil {
		return nil, nil, ErrRefreshReplay
	}
	identity, err := tm.identities.ResolveMembership(ctx, oldRecord.SubjectID, oldRecord.MembershipID)
	if err != nil {
		return nil, nil, err
	}
	newRecord.SubjectID, newRecord.MembershipID = identity.SubjectID, identity.MembershipID
	access, _, err := tm.issueAccess(ctx, identity, now)
	if err != nil {
		return nil, nil, err
	}
	return access, &Token{Token: newValue, ExpiresAt: newRecord.ExpiresAt, CreatedAt: now}, nil
}

func (tm *TokenManager) issueAccess(ctx context.Context, identity *Identity, authTime time.Time) (*Token, *AccessTokenClaims, error) {
	now := tm.config.Now().UTC()
	kid, key, err := tm.keyProvider.CurrentSigningKey(ctx)
	if err != nil {
		return nil, nil, err
	}
	policyVersion, permissions, err := tm.permissions.ResolvePermissions(ctx, identity.SubjectID, identity.MembershipID, identity.TenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve permission snapshot: %w", err)
	}
	if err := ValidatePermissionSnapshot(policyVersion, permissions, identity.TenantID); err != nil {
		return nil, nil, err
	}
	claims := AccessTokenClaims{ProfileVersion: AccessTokenProfileVersion, Issuer: tm.config.Issuer, Audiences: append([]string(nil), tm.config.Audiences...), SubjectID: identity.SubjectID, SubjectType: identity.SubjectType, TenantID: identity.TenantID, MembershipID: identity.MembershipID, TenantMembershipIDs: []string{identity.MembershipID}, PolicyVersion: policyVersion, ScopedPermissions: append([]ScopedPermission{}, permissions...), AllowedActions: permissionActions(permissions), IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(tm.config.AccessTTL).Unix(), AuthTime: authTime.Unix(), TokenID: randomID(), KeyID: kid, Algorithm: AccessTokenAlgorithm}
	value, err := signAccessToken(key, kid, claims)
	if err != nil {
		return nil, nil, err
	}
	if len(value) > MaxAccessTokenSize {
		return nil, nil, errors.New("permission snapshot exceeds access token size")
	}
	return &Token{ID: claims.TokenID, UserID: identity.SubjectID, Token: value, ExpiresAt: time.Unix(claims.ExpiresAt, 0), CreatedAt: now}, &claims, nil
}

func signAccessToken(key *ecdsa.PrivateKey, kid string, claims AccessTokenClaims) (string, error) {
	if !boundedClaim(kid, 128) || key == nil || key.Curve == nil || key.Curve.Params().Name != "P-256" {
		return "", errors.New("ES256 signing requires a named P-256 key")
	}
	header, err := json.Marshal(struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}{AccessTokenType, AccessTokenAlgorithm, kid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeStrictJSON(value string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func validAudiences(values []string) bool {
	if len(values) == 0 || len(values) > 16 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !boundedClaim(value, 256) || value == "*" {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validMemberships(values []string, selected string) bool {
	if len(values) == 0 || len(values) > 128 || !contains(values, selected) {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !boundedClaim(value, 128) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validSubjectType(value string) bool {
	return value == "user" || value == "workload" || value == "service"
}

func permissionActions(permissions []ScopedPermission) []AuthorizationAction {
	actions := make([]AuthorizationAction, 0, len(permissions))
	seen := make(map[AuthorizationAction]struct{}, len(permissions))
	for _, permission := range permissions {
		if _, ok := seen[permission.Action]; ok {
			continue
		}
		seen[permission.Action] = struct{}{}
		actions = append(actions, permission.Action)
	}
	return actions
}

func validAllowedActions(actions []AuthorizationAction, permissions []ScopedPermission) bool {
	if len(actions) > MaxScopedPermissions {
		return false
	}
	permissionSet := make(map[AuthorizationAction]struct{}, len(permissions))
	for _, permission := range permissions {
		permissionSet[permission.Action] = struct{}{}
	}
	seen := make(map[AuthorizationAction]struct{}, len(actions))
	for _, action := range actions {
		if !validAction(action) || action == "*" {
			return false
		}
		if _, ok := permissionSet[action]; !ok {
			return false
		}
		if _, duplicate := seen[action]; duplicate {
			return false
		}
		seen[action] = struct{}{}
	}
	return len(actions) == len(permissionSet)
}

func boundedClaim(value string, maximum int) bool {
	return value != "" && len(value) <= maximum
}

func randomOpaqueToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashRefreshToken(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(value)
}

func generateID() string { return randomID() }
