package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
)

type staticObserverKeys struct {
	key *ecdsa.PrivateKey
	kid string
}

func (s *staticObserverKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return s.kid, s.key, nil
}

func (s *staticObserverKeys) VerificationKey(context.Context, string) (*ecdsa.PublicKey, error) {
	return &s.key.PublicKey, nil
}

func TestObserverTokenSignVerifyRoundTrip(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := &staticObserverKeys{key: key, kid: "test-kid"}
	config := ObserverTokenConfig{Issuer: "https://identity.hnb.cloud", Audience: "hnb-platform-api", TTL: 5 * time.Minute, Now: time.Now}
	signer, err := NewObserverTokenSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewObserverTokenVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	identity := ObserverIdentity{
		TenantID: "tenant-a", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c",
		TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent",
	}
	token, err := signer.Sign(context.Background(), "workload-agent-1", identity, uuid.NewString(), 1)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Identity != identity {
		t.Fatalf("identity = %+v, want %+v", claims.Identity, identity)
	}
	if claims.ObserverGeneration != 1 {
		t.Fatalf("generation = %d", claims.ObserverGeneration)
	}
}

func TestObserverTokenRejectsTamperedAndInvalidIdentity(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keys := &staticObserverKeys{key: key, kid: "test-kid"}
	config := ObserverTokenConfig{Issuer: "https://identity.hnb.cloud", Audience: "hnb-platform-api", TTL: 5 * time.Minute, Now: time.Now}
	signer, _ := NewObserverTokenSigner(config, keys)
	verifier, _ := NewObserverTokenVerifier(config, keys)
	identity := ObserverIdentity{
		TenantID: "tenant-a", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c",
		TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent",
	}
	token, _ := signer.Sign(context.Background(), "workload-agent-1", identity, uuid.NewString(), 1)
	if _, err := verifier.Verify(context.Background(), token[:len(token)-2]+"AA"); err == nil {
		t.Fatal("expected tampered token rejection")
	}

	bad := identity
	bad.TargetKind = "EdgeRuntimeTarget"
	bad.ObserverKind = "Agent" // mismatched kind/source
	if _, err := signer.Sign(context.Background(), "workload-agent-1", bad, uuid.NewString(), 1); err == nil {
		t.Fatal("expected invalid identity rejection at signing")
	}
}

func TestObserverTokenRejectsExpired(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keys := &staticObserverKeys{key: key, kid: "test-kid"}
	now := time.Now()
	signer, _ := NewObserverTokenSigner(ObserverTokenConfig{Issuer: "https://identity.hnb.cloud", Audience: "hnb-platform-api", TTL: 5 * time.Minute, Now: func() time.Time { return now }}, keys)
	verifier, _ := NewObserverTokenVerifier(ObserverTokenConfig{Issuer: "https://identity.hnb.cloud", Audience: "hnb-platform-api", TTL: 5 * time.Minute, Now: func() time.Time { return now.Add(6 * time.Minute) }}, keys)
	identity := ObserverIdentity{
		TenantID: "tenant-a", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c",
		TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent",
	}
	token, _ := signer.Sign(context.Background(), "workload-agent-1", identity, uuid.NewString(), 1)
	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("expected expired token rejection")
	}
}
