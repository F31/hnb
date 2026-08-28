package iam

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestDelegationSignerVerifierBindsServiceActorAndScope(t *testing.T) {
	_, _, _, keys, now := newTestManager(t)
	config := DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second, Now: func() time.Time { return now }}
	signer, err := NewDelegationSigner(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewDelegationVerifier(config, keys)
	if err != nil {
		t.Fatal(err)
	}
	actor := TrustedContext{SubjectID: "actor-a", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2"}
	evidence := DelegationEvidence{
		Scope:  DelegationScope{ResourceKind: "cluster", ResourceID: "target-a", ProjectID: "project-a"},
		Action: ActionUpdate, IntentKind: "UpgradeRuntimeTarget",
		SemanticDigest: "sha256:" + strings.Repeat("a", 64), CorrelationID: "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
	}
	token, err := signer.Sign(context.Background(), actor, evidence)
	if err != nil {
		t.Fatal(err)
	}
	claims, trusted, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ServiceSubject != "hnb-apiserver" || claims.ActorSubject != actor.SubjectID || trusted.SubjectID != actor.SubjectID || trusted.ProjectID != "project-a" {
		t.Fatalf("unexpected delegation: claims=%+v trusted=%+v", claims, trusted)
	}
}

func TestDelegationVerifierRejectsTamperingAndWrongBoundary(t *testing.T) {
	_, _, _, keys, now := newTestManager(t)
	config := DelegationConfig{Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second, Now: func() time.Time { return now }}
	signer, _ := NewDelegationSigner(config, keys)
	actor := TrustedContext{SubjectID: "actor-a", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:2"}
	evidence := DelegationEvidence{Scope: DelegationScope{ResourceKind: "cluster"}, Action: ActionCreate, IntentKind: "ImportRuntimeTarget", SemanticDigest: "sha256:" + strings.Repeat("b", 64), CorrelationID: "018f6c2a-4a64-7b58-9cc3-9f70462f36c1"}
	token, err := signer.Sign(context.Background(), actor, evidence)
	if err != nil {
		t.Fatal(err)
	}

	wrongConfigs := []DelegationConfig{
		{Issuer: config.Issuer, Audience: "other-api", ServiceSubject: config.ServiceSubject, TTL: config.TTL, Now: config.Now},
		{Issuer: config.Issuer, Audience: config.Audience, ServiceSubject: "other-service", TTL: config.TTL, Now: config.Now},
	}
	for _, wrong := range wrongConfigs {
		verifier, _ := NewDelegationVerifier(wrong, keys)
		if _, _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Fatalf("wrong boundary accepted: %+v", wrong)
		}
	}

	parts := strings.Split(token, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	payload[0] ^= 1
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	verifier, _ := NewDelegationVerifier(config, keys)
	if _, _, err := verifier.Verify(context.Background(), strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered delegation was accepted")
	}

	expiredConfig := config
	expiredConfig.Now = func() time.Time { return now.Add(time.Minute) }
	expired, _ := NewDelegationVerifier(expiredConfig, keys)
	if _, _, err := expired.Verify(context.Background(), token); err == nil {
		t.Fatal("expired delegation was accepted")
	}
}
