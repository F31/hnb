package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"
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
