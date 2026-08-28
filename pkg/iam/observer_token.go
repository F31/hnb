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
	"strings"
	"time"
)

// Observer identity tokens bind a workload (Agent or CloudCore observer) to the
// exact tenant, target, target kind, observer kind, observer generation lease,
// and observer ID it is permitted to report. The platform projector treats this
// verified identity as authoritative over any payload field.
const (
	ObserverProfileVersion = "hnb.observer/v1"
	ObserverTokenType      = "observer+jwt"
	MaxObserverTokenSize   = 8192
	MaxObserverTokenTTL    = 10 * time.Minute
)

type ObserverIdentity struct {
	TenantID     string `json:"tenantId"`
	TargetID     string `json:"targetId"`
	TargetKind   string `json:"targetKind"`
	ObserverID   string `json:"observerId"`
	ObserverKind string `json:"observerKind"`
}

type ObserverTokenClaims struct {
	ProfileVersion     string           `json:"profileVersion"`
	Issuer             string           `json:"iss"`
	Audience           string           `json:"aud"`
	SubjectID          string           `json:"sub"`
	SubjectType        string           `json:"type"`
	Identity           ObserverIdentity `json:"observer"`
	ObserverLeaseID    string           `json:"observerLeaseId"`
	ObserverGeneration int64            `json:"observerGeneration"`
	IssuedAt           int64            `json:"iat"`
	NotBefore          int64            `json:"nbf"`
	ExpiresAt          int64            `json:"exp"`
	TokenID            string           `json:"jti"`
	KeyID              string           `json:"keyId"`
	Algorithm          string           `json:"algorithm"`
}

type ObserverTokenConfig struct {
	Issuer   string
	Audience string
	TTL      time.Duration
	Now      func() time.Time
}

type ObserverTokenSigner struct {
	config      ObserverTokenConfig
	keyProvider KeyProvider
}

type ObserverTokenVerifier struct {
	config  ObserverTokenConfig
	keyRing KeyRing
}

func NewObserverTokenSigner(config ObserverTokenConfig, keyProvider KeyProvider) (*ObserverTokenSigner, error) {
	if err := validateObserverTokenConfig(&config); err != nil {
		return nil, err
	}
	if keyProvider == nil {
		return nil, errors.New("observer signing key provider is required")
	}
	return &ObserverTokenSigner{config: config, keyProvider: keyProvider}, nil
}

func NewObserverTokenVerifier(config ObserverTokenConfig, keyRing KeyRing) (*ObserverTokenVerifier, error) {
	if err := validateObserverTokenConfig(&config); err != nil {
		return nil, err
	}
	if keyRing == nil {
		return nil, errors.New("observer verification key ring is required")
	}
	return &ObserverTokenVerifier{config: config, keyRing: keyRing}, nil
}

func validateObserverTokenConfig(config *ObserverTokenConfig) error {
	if !boundedClaim(config.Issuer, 256) || !boundedClaim(config.Audience, 256) || config.Audience == "*" {
		return errors.New("observer issuer and audience are required")
	}
	if config.TTL < time.Second || config.TTL > MaxObserverTokenTTL {
		return errors.New("observer token TTL must be between 1 second and 10 minutes")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return nil
}

// Sign issues an observer identity token for the given workload lease. The
// subject is the workload identity; the observer identity carries the exact
// target binding the lease authorizes.
func (s *ObserverTokenSigner) Sign(ctx context.Context, workloadSubject string, identity ObserverIdentity, observerLeaseID string, observerGeneration int64) (string, error) {
	if !boundedClaim(workloadSubject, 128) || !validObserverIdentity(identity) || !boundedClaim(observerLeaseID, 128) || observerGeneration < 1 {
		return "", errors.New("invalid observer lease")
	}
	kid, key, err := s.keyProvider.CurrentSigningKey(ctx)
	if err != nil {
		return "", err
	}
	now := s.config.Now().UTC()
	claims := ObserverTokenClaims{
		ProfileVersion: ObserverProfileVersion, Issuer: s.config.Issuer, Audience: s.config.Audience,
		SubjectID: workloadSubject, SubjectType: "workload", Identity: identity,
		ObserverLeaseID: observerLeaseID, ObserverGeneration: observerGeneration,
		IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(s.config.TTL).Unix(),
		TokenID: randomID(), KeyID: kid, Algorithm: AccessTokenAlgorithm,
	}
	return signObserverToken(key, kid, claims)
}

// Verify validates an observer identity token and returns the bound identity.
func (v *ObserverTokenVerifier) Verify(ctx context.Context, token string) (*ObserverTokenClaims, error) {
	if len(token) == 0 || len(token) > MaxObserverTokenSize {
		return nil, errors.New("invalid observer token size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("invalid observer JWT")
	}
	var header struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeStrictJSON(parts[0], &header); err != nil || header.Type != ObserverTokenType ||
		header.Algorithm != AccessTokenAlgorithm || !boundedClaim(header.KeyID, 128) {
		return nil, errors.New("unsupported observer JWT header")
	}
	key, err := v.keyRing.VerificationKey(ctx, header.KeyID)
	if err != nil || key == nil || key.Curve.Params().Name != "P-256" {
		return nil, errors.New("unknown observer signing key")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, errors.New("invalid observer signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return nil, errors.New("invalid observer signature")
	}
	var claims ObserverTokenClaims
	if err := decodeStrictJSON(parts[1], &claims); err != nil {
		return nil, errors.New("invalid observer claims")
	}
	now := v.config.Now().UTC().Unix()
	if claims.ProfileVersion != ObserverProfileVersion || claims.Issuer != v.config.Issuer ||
		claims.Audience != v.config.Audience || claims.SubjectType != "workload" ||
		claims.KeyID != header.KeyID || claims.Algorithm != header.Algorithm || !boundedClaim(claims.TokenID, 128) ||
		claims.IssuedAt <= 0 || claims.NotBefore <= 0 || claims.ExpiresAt <= 0 || claims.IssuedAt > now || claims.NotBefore > now ||
		claims.ExpiresAt <= now || claims.ExpiresAt <= claims.IssuedAt || time.Duration(claims.ExpiresAt-claims.IssuedAt)*time.Second > v.config.TTL {
		return nil, errors.New("observer claims failed validation")
	}
	if !validObserverIdentity(claims.Identity) || !boundedClaim(claims.ObserverLeaseID, 128) || claims.ObserverGeneration < 1 {
		return nil, errors.New("observer identity failed validation")
	}
	return &claims, nil
}

func validObserverIdentity(identity ObserverIdentity) bool {
	return boundedClaim(identity.TenantID, 128) && identity.TenantID != "*" &&
		boundedClaim(identity.TargetID, 64) && identity.TargetID != "*" &&
		(identity.TargetKind == "KubernetesTarget" || identity.TargetKind == "EdgeRuntimeTarget") &&
		boundedClaim(identity.ObserverID, 256) &&
		(identity.ObserverKind == "Agent" || identity.ObserverKind == "CloudCore") &&
		(identity.TargetKind == "KubernetesTarget") == (identity.ObserverKind == "Agent")
}

func signObserverToken(key *ecdsa.PrivateKey, kid string, claims ObserverTokenClaims) (string, error) {
	if !boundedClaim(kid, 128) || key == nil || key.Curve == nil || key.Curve.Params().Name != "P-256" {
		return "", errors.New("ES256 signing requires a named P-256 key")
	}
	header := struct {
		Type      string `json:"typ"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}{Type: ObserverTokenType, Algorithm: AccessTokenAlgorithm, KeyID: kid}
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
