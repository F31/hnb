package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newAgentTunnelTestKeys(t *testing.T) *staticObserverKeys {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &staticObserverKeys{key: key, kid: "test-kid"}
}

var agentTunnelTestConfig = AgentTunnelTokenConfig{
	Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel", TTL: time.Hour, Now: time.Now,
}

func TestAgentTunnelTokenSignVerifyRoundTrip(t *testing.T) {
	keys := newAgentTunnelTestKeys(t)
	signer, err := NewAgentTunnelTokenSigner(agentTunnelTestConfig, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewAgentTunnelTokenVerifier(agentTunnelTestConfig, keys)
	if err != nil {
		t.Fatal(err)
	}
	token, expiry, err := signer.Sign(context.Background(), "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c")
	if err != nil {
		t.Fatal(err)
	}
	if !expiry.After(time.Now()) {
		t.Fatalf("expiry %v not in future", expiry)
	}
	got, err := verifier.Verify(context.Background(), token, "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !got.Equal(expiry) {
		t.Fatalf("expiry = %v, want %v", got, expiry)
	}
}

func TestAgentTunnelTokenRejectsWrongScopeTamperAndTTL(t *testing.T) {
	keys := newAgentTunnelTestKeys(t)
	signer, _ := NewAgentTunnelTokenSigner(agentTunnelTestConfig, keys)
	verifier, _ := NewAgentTunnelTokenVerifier(agentTunnelTestConfig, keys)
	const clusterID = "515eba09-0a41-5b92-b972-69af1f0f655c"
	token, _, err := signer.Sign(context.Background(), "tenant-a", clusterID)
	if err != nil {
		t.Fatal(err)
	}

	// Wrong tenant/cluster binding must be rejected.
	if _, err := verifier.Verify(context.Background(), token, "tenant-b", clusterID); err == nil {
		t.Fatal("expected wrong-tenant rejection")
	}
	if _, err := verifier.Verify(context.Background(), token, "tenant-a", "other-cluster-id"); err == nil {
		t.Fatal("expected wrong-cluster rejection")
	}

	// Tampered signature must be rejected.
	if _, err := verifier.Verify(context.Background(), token[:len(token)-2]+"AA", "tenant-a", clusterID); err == nil {
		t.Fatal("expected tampered token rejection")
	}

	// Empty / malformed tokens must be rejected.
	if _, err := verifier.Verify(context.Background(), "", "tenant-a", clusterID); err == nil {
		t.Fatal("expected empty token rejection")
	}
	if _, err := verifier.Verify(context.Background(), strings.Repeat("x", 4), "tenant-a", clusterID); err == nil {
		t.Fatal("expected malformed token rejection")
	}

	// Accessor with wildcard tenant must be rejected at signing.
	if _, _, err := signer.Sign(context.Background(), "*", clusterID); err == nil {
		t.Fatal("expected wildcard tenant rejection")
	}
	if _, _, err := signer.Sign(context.Background(), "tenant-a", ""); err == nil {
		t.Fatal("expected empty cluster rejection")
	}

	// TTL beyond the ceiling must be rejected at construction.
	bad := agentTunnelTestConfig
	bad.TTL = MaxAgentTunnelTokenTTL + time.Second
	if _, err := NewAgentTunnelTokenSigner(bad, keys); err == nil {
		t.Fatal("expected TTL ceiling rejection")
	}
}

func TestAgentTunnelSignWithTTLIssuesShortSession(t *testing.T) {
	keys := newAgentTunnelTestKeys(t)
	now := time.Unix(2_000_000_000, 0).UTC()
	config := AgentTunnelTokenConfig{
		Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel",
		TTL: MaxAgentTunnelTokenTTL, Now: func() time.Time { return now },
	}
	signer, err := NewAgentTunnelTokenSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewAgentTunnelTokenVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	const clusterID = "515eba09-0a41-5b92-b972-69af1f0f655c"
	token, expiry, err := signer.SignWithTTL(context.Background(), "tenant-a", clusterID, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if expiry.Sub(now) != 5*time.Minute {
		t.Fatalf("session expiry = %v, want 5m", expiry.Sub(now))
	}
	if _, err := verifier.Verify(context.Background(), token, "tenant-a", clusterID); err != nil {
		t.Fatalf("short session failed verification: %v", err)
	}
	for _, ttl := range []time.Duration{0, time.Millisecond, MaxAgentTunnelTokenTTL + time.Second} {
		if _, _, err := signer.SignWithTTL(context.Background(), "tenant-a", clusterID, ttl); err == nil {
			t.Fatalf("TTL %v was accepted", ttl)
		}
	}
}

func TestAgentTunnelSessionClaimsAndLookup(t *testing.T) {
	keys := newAgentTunnelTestKeys(t)
	now := time.Unix(2_000_000_000, 0).UTC()
	config := AgentTunnelTokenConfig{
		Issuer: "https://identity.hnb.cloud", Audience: "hnb-apiserver-tunnel",
		TTL: MaxAgentTunnelTokenTTL, Now: func() time.Time { return now },
	}
	signer, _ := NewAgentTunnelTokenSigner(config, keys)
	verifier, _ := NewAgentTunnelTokenVerifier(config, keys)
	const clusterID = "515eba09-0a41-5b92-b972-69af1f0f655c"
	sessionID := uuid.NewString()
	token, _, err := signer.SignSession(context.Background(), "tenant-a", clusterID, sessionID, 3, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tenantID, gotCluster, gotSession, err := ParseAgentSessionLookup(token)
	if err != nil || tenantID != "tenant-a" || gotCluster != clusterID || gotSession != sessionID {
		t.Fatalf("lookup = %q/%q/%q err=%v", tenantID, gotCluster, gotSession, err)
	}
	claims, err := verifier.VerifyClaims(context.Background(), token, "tenant-a", clusterID)
	if err != nil || claims.SessionID != sessionID || claims.Generation != 3 {
		t.Fatalf("claims = %+v err=%v", claims, err)
	}
	if _, _, err := signer.SignSession(context.Background(), "tenant-a", clusterID, "not-a-uuid", 1, time.Minute); err == nil {
		t.Fatal("non-UUID session was accepted")
	}
	if _, _, err := signer.SignSession(context.Background(), "tenant-a", clusterID, sessionID, 0, time.Minute); err == nil {
		t.Fatal("zero generation was accepted")
	}
	legacy, _, err := signer.SignWithTTL(context.Background(), "tenant-a", clusterID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ParseAgentSessionLookup(legacy); err == nil {
		t.Fatal("legacy token was treated as a session token")
	}
}
