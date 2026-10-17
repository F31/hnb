package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Agent tunnel tokens bind a cluster-agent workload to the exact tenant and
// cluster it is allowed to connect to through the platform tunnel
// (pkg/tunnel). Unlike short-lived access tokens (capped at
// MaxAccessTokenTTL), onboarding tokens need to outlive `kubectl apply` plus
// first pod startup, so this is a dedicated profile with its own TTL ceiling.
//
// The token is issued by the agent-onboarding endpoint after the caller has
// been authorized for the target (authorization happens at issuance); the
// tunnel server treats the verified binding as authoritative.
const (
	AgentTunnelProfileVersion = "hnb.agent-tunnel/v1"
	AgentTunnelTokenType      = "agent-tunnel+jwt"
	MaxAgentTunnelTokenSize   = 8192
	MaxAgentTunnelTokenTTL    = 24 * time.Hour
)

type AgentTunnelIdentity struct {
	TenantID  string `json:"tenantId"`
	ClusterID string `json:"clusterId"`
}

type AgentTunnelTokenClaims struct {
	ProfileVersion string              `json:"profileVersion"`
	Issuer         string              `json:"iss"`
	Audience       string              `json:"aud"`
	SubjectID      string              `json:"sub"`
	SubjectType    string              `json:"type"`
	Identity       AgentTunnelIdentity `json:"agent"`
	SessionID      string              `json:"sessionId,omitempty"`
	Generation     int64               `json:"generation,omitempty"`
	IssuedAt       int64               `json:"iat"`
	NotBefore      int64               `json:"nbf"`
	ExpiresAt      int64               `json:"exp"`
	TokenID        string              `json:"jti"`
	KeyID          string              `json:"keyId"`
	Algorithm      string              `json:"algorithm"`
}

type AgentTunnelTokenConfig struct {
	Issuer   string
	Audience string
	TTL      time.Duration
	Now      func() time.Time
}

type AgentTunnelTokenSigner struct {
	config      AgentTunnelTokenConfig
	keyProvider KeyProvider
}

type AgentTunnelTokenVerifier struct {
	config  AgentTunnelTokenConfig
	keyRing KeyRing
}

func NewAgentTunnelTokenSigner(config AgentTunnelTokenConfig, keyProvider KeyProvider) (*AgentTunnelTokenSigner, error) {
	if err := validateAgentTunnelTokenConfig(&config); err != nil {
		return nil, err
	}
	if keyProvider == nil {
		return nil, errors.New("agent tunnel signing key provider is required")
	}
	return &AgentTunnelTokenSigner{config: config, keyProvider: keyProvider}, nil
}

func NewAgentTunnelTokenVerifier(config AgentTunnelTokenConfig, keyRing KeyRing) (*AgentTunnelTokenVerifier, error) {
	if err := validateAgentTunnelTokenConfig(&config); err != nil {
		return nil, err
	}
	if keyRing == nil {
		return nil, errors.New("agent tunnel verification key ring is required")
	}
	return &AgentTunnelTokenVerifier{config: config, keyRing: keyRing}, nil
}

func validateAgentTunnelTokenConfig(config *AgentTunnelTokenConfig) error {
	if !boundedClaim(config.Issuer, 256) || !boundedClaim(config.Audience, 256) || config.Audience == "*" {
		return errors.New("agent tunnel issuer and audience are required")
	}
	if config.TTL < time.Second || config.TTL > MaxAgentTunnelTokenTTL {
		return fmt.Errorf("agent tunnel token TTL must be between 1 second and %s", MaxAgentTunnelTokenTTL)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return nil
}

func validAgentTunnelIdentity(identity AgentTunnelIdentity) bool {
	return boundedClaim(identity.TenantID, 128) && identity.TenantID != "*" &&
		boundedClaim(identity.ClusterID, 64) && identity.ClusterID != "*"
}

// Sign issues an agent tunnel token bound to (tenantID, clusterID). The
// returned time.Time is the token expiry (UTC).
func (s *AgentTunnelTokenSigner) Sign(ctx context.Context, tenantID, clusterID string) (string, time.Time, error) {
	return s.SignWithTTL(ctx, tenantID, clusterID, s.config.TTL)
}

// SignWithTTL issues a shorter-lived legacy token without a session binding.
// It never exceeds the configured or profile TTL.
func (s *AgentTunnelTokenSigner) SignWithTTL(ctx context.Context, tenantID, clusterID string, ttl time.Duration) (string, time.Time, error) {
	return s.sign(ctx, tenantID, clusterID, "", 0, ttl)
}

// SignSession issues a renewable session token bound to one agent_sessions row.
// Rotation keeps the same session ID and generation; re-enrollment creates a
// new generation. The tunnel verifier checks the persisted session state.
func (s *AgentTunnelTokenSigner) SignSession(ctx context.Context, tenantID, clusterID, sessionID string, generation int64, ttl time.Duration) (string, time.Time, error) {
	if _, err := uuid.Parse(sessionID); err != nil {
		return "", time.Time{}, errors.New("agent session ID must be a UUID")
	}
	if generation < 1 {
		return "", time.Time{}, errors.New("agent session generation must be positive")
	}
	return s.sign(ctx, tenantID, clusterID, sessionID, generation, ttl)
}

func (s *AgentTunnelTokenSigner) sign(ctx context.Context, tenantID, clusterID, sessionID string, generation int64, ttl time.Duration) (string, time.Time, error) {
	if !validAgentTunnelIdentity(AgentTunnelIdentity{TenantID: tenantID, ClusterID: clusterID}) {
		return "", time.Time{}, errors.New("explicit tenant and cluster binding are required")
	}
	if ttl < time.Second || ttl > s.config.TTL || ttl > MaxAgentTunnelTokenTTL {
		return "", time.Time{}, errors.New("agent tunnel session TTL is outside the allowed window")
	}
	kid, key, err := s.keyProvider.CurrentSigningKey(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.config.Now().UTC()
	expiry := now.Add(ttl).Truncate(time.Second)
	claims := AgentTunnelTokenClaims{
		ProfileVersion: AgentTunnelProfileVersion, Issuer: s.config.Issuer, Audience: s.config.Audience,
		SubjectID: "cluster-agent", SubjectType: "service",
		Identity:  AgentTunnelIdentity{TenantID: tenantID, ClusterID: clusterID},
		SessionID: sessionID, Generation: generation,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: expiry.Unix(),
		TokenID: randomID(), KeyID: kid, Algorithm: AccessTokenAlgorithm,
	}
	token, err := signAgentTunnelToken(key, kid, claims)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiry, nil
}

// Verify validates a token signature, profile, and exact (tenant, cluster)
// binding; it returns the token expiry (UTC).
func (v *AgentTunnelTokenVerifier) Verify(ctx context.Context, token, tenantID, clusterID string) (time.Time, error) {
	claims, err := v.VerifyClaims(ctx, token, tenantID, clusterID)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(claims.ExpiresAt, 0).UTC(), nil
}

// VerifyClaims returns the verified claims so callers can enforce persisted
// session revocation/generation state after the signature check.
func (v *AgentTunnelTokenVerifier) VerifyClaims(ctx context.Context, token, tenantID, clusterID string) (*AgentTunnelTokenClaims, error) {
	if len(token) == 0 || len(token) > MaxAgentTunnelTokenSize {
		return nil, errors.New("invalid agent tunnel token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("invalid agent tunnel JWT")
	}
	var header struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeStrictJSON(parts[0], &header); err != nil || header.Type != AgentTunnelTokenType ||
		header.Algorithm != AccessTokenAlgorithm || !boundedClaim(header.KeyID, 128) {
		return nil, errors.New("unsupported agent tunnel JWT header")
	}
	key, err := v.keyRing.VerificationKey(ctx, header.KeyID)
	if err != nil || key == nil || key.Curve.Params().Name != "P-256" {
		return nil, errors.New("unknown agent tunnel signing key")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, errors.New("invalid agent tunnel signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return nil, errors.New("invalid agent tunnel signature")
	}
	var claims AgentTunnelTokenClaims
	if err := decodeStrictJSON(parts[1], &claims); err != nil {
		return nil, errors.New("invalid agent tunnel claims")
	}
	now := v.config.Now().UTC().Unix()
	if claims.ProfileVersion != AgentTunnelProfileVersion || claims.Issuer != v.config.Issuer ||
		claims.Audience != v.config.Audience || claims.SubjectType != "service" ||
		claims.KeyID != header.KeyID || claims.Algorithm != header.Algorithm || !boundedClaim(claims.TokenID, 128) ||
		claims.IssuedAt <= 0 || claims.NotBefore <= 0 || claims.ExpiresAt <= 0 || claims.IssuedAt > now || claims.NotBefore > now ||
		claims.ExpiresAt <= now || claims.ExpiresAt <= claims.IssuedAt ||
		time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > MaxAgentTunnelTokenTTL {
		return nil, errors.New("agent tunnel claims failed validation")
	}
	if !validAgentTunnelIdentity(claims.Identity) || claims.Identity.TenantID != tenantID || claims.Identity.ClusterID != clusterID {
		return nil, errors.New("agent tunnel token is not bound to the requested scope")
	}
	if (claims.SessionID == "") != (claims.Generation == 0) {
		return nil, errors.New("agent tunnel session claims are incomplete")
	}
	if claims.SessionID != "" {
		if _, err := uuid.Parse(claims.SessionID); err != nil || claims.Generation < 1 {
			return nil, errors.New("agent tunnel session claims are invalid")
		}
	}
	return &claims, nil
}

func signAgentTunnelToken(key *ecdsa.PrivateKey, kid string, claims AgentTunnelTokenClaims) (string, error) {
	if !boundedClaim(kid, 128) || key == nil || key.Curve == nil || key.Curve.Params().Name != "P-256" {
		return "", errors.New("ES256 signing requires a named P-256 key")
	}
	header := struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}{Type: AgentTunnelTokenType, Algorithm: AccessTokenAlgorithm, KeyID: kid}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
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

// ParseAgentSessionLookup extracts only the unverified lookup keys needed to
// find the authoritative agent_sessions row. The caller must still verify the
// token with AgentTunnelTokenVerifier.VerifyClaims before rotating or using it.
func ParseAgentSessionLookup(token string) (string, string, string, error) {
	if len(token) == 0 || len(token) > MaxAgentTunnelTokenSize {
		return "", "", "", errors.New("invalid agent session token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[1] == "" {
		return "", "", "", errors.New("invalid agent session JWT")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", errors.New("invalid agent session payload")
	}
	var claims AgentTunnelTokenClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return "", "", "", errors.New("invalid agent session payload")
	}
	if !validAgentTunnelIdentity(claims.Identity) || claims.SessionID == "" {
		return "", "", "", errors.New("not an agent session token")
	}
	if _, err := uuid.Parse(claims.SessionID); err != nil {
		return "", "", "", errors.New("invalid agent session ID")
	}
	return claims.Identity.TenantID, claims.Identity.ClusterID, claims.SessionID, nil
}
