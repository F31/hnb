package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DelegationProfileVersion = "hnb.delegation/v1"
	DelegationTokenType      = "delegation+jwt"
	MaxDelegationTokenSize   = 8192
	MaxDelegationTTL         = 60 * time.Second
)

var semanticDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type DelegationScope struct {
	ResourceKind  string `json:"resourceKind"`
	ResourceID    string `json:"resourceId,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	NamespaceID   string `json:"namespaceId,omitempty"`
}

type DelegationClaims struct {
	ProfileVersion string              `json:"profileVersion"`
	Issuer         string              `json:"iss"`
	Audience       string              `json:"aud"`
	ServiceSubject string              `json:"sub"`
	SubjectType    string              `json:"type"`
	ActorSubject   string              `json:"actorSubject"`
	MembershipID   string              `json:"membershipId"`
	TenantID       string              `json:"tenantId"`
	Scope          DelegationScope     `json:"scope"`
	Action         AuthorizationAction `json:"action"`
	IntentKind     string              `json:"intentKind"`
	SemanticDigest string              `json:"semanticDigest"`
	CorrelationID  string              `json:"correlationId"`
	PolicyVersion  string              `json:"policyVersion"`
	IssuedAt       int64               `json:"iat"`
	NotBefore      int64               `json:"nbf"`
	ExpiresAt      int64               `json:"exp"`
	TokenID        string              `json:"jti"`
	KeyID          string              `json:"keyId"`
	Algorithm      string              `json:"algorithm"`
}

type DelegationEvidence struct {
	Scope          DelegationScope
	Action         AuthorizationAction
	IntentKind     string
	SemanticDigest string
	CorrelationID  string
}

type DelegationConfig struct {
	Issuer         string
	Audience       string
	ServiceSubject string
	TTL            time.Duration
	Now            func() time.Time
}

type DelegationSigner struct {
	config      DelegationConfig
	keyProvider KeyProvider
}

type DelegationVerifier struct {
	config  DelegationConfig
	keyRing KeyRing
}

type delegationClaimsContextKey struct{}

func WithDelegationClaims(ctx context.Context, claims DelegationClaims) context.Context {
	return context.WithValue(ctx, delegationClaimsContextKey{}, claims)
}

func DelegationClaimsFrom(ctx context.Context) (DelegationClaims, bool) {
	claims, ok := ctx.Value(delegationClaimsContextKey{}).(DelegationClaims)
	return claims, ok
}

func NewDelegationSigner(config DelegationConfig, keyProvider KeyProvider) (*DelegationSigner, error) {
	if err := validateDelegationConfig(&config); err != nil {
		return nil, err
	}
	if keyProvider == nil {
		return nil, errors.New("delegation signing key provider is required")
	}
	return &DelegationSigner{config: config, keyProvider: keyProvider}, nil
}

func NewDelegationVerifier(config DelegationConfig, keyRing KeyRing) (*DelegationVerifier, error) {
	if err := validateDelegationConfig(&config); err != nil {
		return nil, err
	}
	if keyRing == nil {
		return nil, errors.New("delegation verification key ring is required")
	}
	return &DelegationVerifier{config: config, keyRing: keyRing}, nil
}

func validateDelegationConfig(config *DelegationConfig) error {
	if !boundedClaim(config.Issuer, 256) || !boundedClaim(config.Audience, 256) || config.Audience == "*" ||
		!boundedClaim(config.ServiceSubject, 128) || config.ServiceSubject == "*" {
		return errors.New("delegation issuer, audience, and service subject are required")
	}
	if config.TTL < time.Second || config.TTL > MaxDelegationTTL {
		return errors.New("delegation TTL must be between 1 and 60 seconds")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return nil
}

func (s *DelegationSigner) Sign(ctx context.Context, actor TrustedContext, evidence DelegationEvidence) (string, error) {
	if err := validateDelegationEvidence(actor, evidence); err != nil {
		return "", err
	}
	kid, key, err := s.keyProvider.CurrentSigningKey(ctx)
	if err != nil {
		return "", err
	}
	now := s.config.Now().UTC()
	claims := DelegationClaims{
		ProfileVersion: DelegationProfileVersion, Issuer: s.config.Issuer, Audience: s.config.Audience,
		ServiceSubject: s.config.ServiceSubject, SubjectType: "service", ActorSubject: actor.SubjectID,
		MembershipID: actor.MembershipID, TenantID: actor.TenantID, Scope: evidence.Scope,
		Action: evidence.Action, IntentKind: evidence.IntentKind, SemanticDigest: evidence.SemanticDigest,
		CorrelationID: evidence.CorrelationID, PolicyVersion: actor.PolicyVersion,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(s.config.TTL).Unix(),
		TokenID: randomID(), KeyID: kid, Algorithm: AccessTokenAlgorithm,
	}
	return signDelegationToken(key, kid, claims)
}

func (v *DelegationVerifier) Verify(ctx context.Context, token string) (*DelegationClaims, TrustedContext, error) {
	if len(token) == 0 || len(token) > MaxDelegationTokenSize {
		return nil, TrustedContext{}, errors.New("invalid delegation token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, TrustedContext{}, errors.New("invalid delegation JWT")
	}
	var header struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeStrictJSON(parts[0], &header); err != nil || header.Type != DelegationTokenType ||
		header.Algorithm != AccessTokenAlgorithm || !boundedClaim(header.KeyID, 128) {
		return nil, TrustedContext{}, errors.New("unsupported delegation JWT header")
	}
	key, err := v.keyRing.VerificationKey(ctx, header.KeyID)
	if err != nil || key == nil || key.Curve.Params().Name != "P-256" {
		return nil, TrustedContext{}, errors.New("unknown delegation signing key")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, TrustedContext{}, errors.New("invalid delegation signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return nil, TrustedContext{}, errors.New("invalid delegation signature")
	}
	var claims DelegationClaims
	if err := decodeStrictJSON(parts[1], &claims); err != nil {
		return nil, TrustedContext{}, errors.New("invalid delegation claims")
	}
	now := v.config.Now().UTC().Unix()
	if claims.ProfileVersion != DelegationProfileVersion || claims.Issuer != v.config.Issuer ||
		claims.Audience != v.config.Audience || claims.ServiceSubject != v.config.ServiceSubject || claims.SubjectType != "service" ||
		claims.KeyID != header.KeyID || claims.Algorithm != header.Algorithm || !boundedClaim(claims.TokenID, 128) ||
		claims.IssuedAt <= 0 || claims.NotBefore <= 0 || claims.ExpiresAt <= 0 || claims.IssuedAt > now || claims.NotBefore > now ||
		claims.ExpiresAt <= now || claims.ExpiresAt <= claims.IssuedAt || time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > v.config.TTL {
		return nil, TrustedContext{}, errors.New("delegation claims failed validation")
	}
	actor := TrustedContext{
		ProfileVersion: DelegationProfileVersion, SubjectID: claims.ActorSubject, SubjectType: "user",
		TenantID: claims.TenantID, MembershipID: claims.MembershipID, PolicyVersion: claims.PolicyVersion,
		ProjectID: claims.Scope.ProjectID, EnvironmentID: claims.Scope.EnvironmentID, NamespaceID: claims.Scope.NamespaceID,
		CorrelationID: claims.CorrelationID, TokenID: claims.TokenID, ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(),
	}
	if err := validateDelegationEvidence(actor, DelegationEvidence{
		Scope: claims.Scope, Action: claims.Action, IntentKind: claims.IntentKind,
		SemanticDigest: claims.SemanticDigest, CorrelationID: claims.CorrelationID,
	}); err != nil {
		return nil, TrustedContext{}, errors.New("delegation evidence failed validation")
	}
	return &claims, actor, nil
}

func validateDelegationEvidence(actor TrustedContext, evidence DelegationEvidence) error {
	if !boundedClaim(actor.SubjectID, 128) || !boundedClaim(actor.MembershipID, 128) ||
		!boundedClaim(actor.TenantID, 128) || actor.TenantID == "*" || !boundedClaim(actor.PolicyVersion, MaxPolicyVersionLen) ||
		!validScope(evidence.Scope.ProjectID, evidence.Scope.EnvironmentID, evidence.Scope.NamespaceID) ||
		!validAction(evidence.Action) || evidence.Action == "*" ||
		!correlationPattern.MatchString(strings.ToLower(evidence.CorrelationID)) {
		return errors.New("invalid delegation evidence")
	}
	switch evidence.Scope.ResourceKind {
	case string(ResourceCluster):
		// Cluster intent delegation: the target UUID, an intent kind, and the
		// semantic digest are required.
		if !boundedOptional(evidence.Scope.ResourceID, 256, false) ||
			!boundedClaim(evidence.IntentKind, 128) ||
			!semanticDigestPattern.MatchString(evidence.SemanticDigest) {
			return errors.New("invalid delegation evidence")
		}
	case string(ResourceClusterMetadata):
		// Cluster metadata delegation: scoped to a cluster UUID for metadata
		// writes (e.g. description). There is no intent kind or semantic digest.
		if !uuidString(evidence.Scope.ResourceID) ||
			evidence.IntentKind != "" || evidence.SemanticDigest != "" {
			return errors.New("invalid delegation evidence")
		}
	case string(ResourceStorageClassBinding), string(ResourceStorageDriverInstallation), string(ResourceRetainedVolume):
		if !boundedClaim(evidence.Scope.ResourceID, 256) ||
			!boundedClaim(evidence.IntentKind, 128) ||
			!semanticDigestPattern.MatchString(evidence.SemanticDigest) {
			return errors.New("invalid delegation evidence")
		}
	case string(ResourceOperation):
		// Operation delegation: the operation UUID and an operation action are
		// required; list carries no resource ID. There is no intent kind or
		// semantic digest for operation forwarding.
		if (evidence.Action != ActionList && !uuidString(evidence.Scope.ResourceID)) ||
			(evidence.Action == ActionList && evidence.Scope.ResourceID != "") ||
			evidence.IntentKind != "" || evidence.SemanticDigest != "" {
			return errors.New("invalid delegation evidence")
		}
	case string(ResourceSecret):
		// Secret registration delegation: scoped to a tenant secret reference.
		// A bounded resource ID is allowed (empty for tenant-scoped writes) and
		// there is no intent kind or semantic digest.
		if !boundedOptional(evidence.Scope.ResourceID, 256, false) ||
			evidence.IntentKind != "" || evidence.SemanticDigest != "" {
			return errors.New("invalid delegation evidence")
		}
	default:
		return errors.New("invalid delegation evidence")
	}
	return nil
}

// uuidString reports whether s is a canonical UUID.
func uuidString(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && s == uuid.MustParse(s).String()
}

func signDelegationToken(key *ecdsa.PrivateKey, kid string, claims DelegationClaims) (string, error) {
	if !boundedClaim(kid, 128) || key == nil || key.Curve == nil || key.Curve.Params().Name != "P-256" {
		return "", errors.New("ES256 signing requires a named P-256 key")
	}
	header, err := json.Marshal(struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}{DelegationTokenType, AccessTokenAlgorithm, kid})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	r, sigS, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	sigS.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
