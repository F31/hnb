package stale

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidChallenge = errors.New("stale confirmation is invalid or expired")

type ChallengeClaims struct {
	TenantID              string `json:"tenantId"`
	ActorID               string `json:"actorId"`
	TargetID              string `json:"targetId"`
	TargetKind            string `json:"targetKind"`
	IntentKind            string `json:"intentKind"`
	IntentDigest          string `json:"intentDigest"`
	ProjectionVersion     int64  `json:"projectionVersion"`
	ObservationGeneration int64  `json:"observationGeneration"`
	ObservationRevision   int64  `json:"observationRevision"`
	ObservedAt            int64  `json:"observedAt"`
	IssuedAt              int64  `json:"issuedAt"`
	ExpiresAt             int64  `json:"expiresAt"`
}

type Signer struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

func NewSigner(key []byte, ttl time.Duration) (*Signer, error) {
	if len(key) < 32 {
		return nil, errors.New("stale challenge key must contain at least 32 bytes")
	}
	if ttl < time.Minute || ttl > 15*time.Minute {
		return nil, errors.New("stale challenge TTL must be between 1 and 15 minutes")
	}
	return &Signer{key: append([]byte(nil), key...), ttl: ttl, now: time.Now}, nil
}

func (s *Signer) Issue(claims ChallengeClaims) (string, error) {
	now := s.now().UTC()
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = now.Add(s.ttl).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Signer) Verify(token string, expected ChallengeClaims) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return ErrInvalidChallenge
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ErrInvalidChallenge
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return ErrInvalidChallenge
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrInvalidChallenge
	}
	var actual ChallengeClaims
	if json.Unmarshal(payload, &actual) != nil || s.now().Unix() > actual.ExpiresAt || actual.ExpiresAt <= actual.IssuedAt {
		return ErrInvalidChallenge
	}
	actual.IssuedAt, actual.ExpiresAt = 0, 0
	expected.IssuedAt, expected.ExpiresAt = 0, 0
	if actual != expected {
		return ErrInvalidChallenge
	}
	return nil
}
