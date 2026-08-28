package stale

import (
	"strings"
	"testing"
	"time"
)

func TestChallengeBindingExpiryAndTamper(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	signer, err := NewSigner([]byte(strings.Repeat("k", 32)), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	signer.now = func() time.Time { return now }
	claims := ChallengeClaims{TenantID: "tenant-a", ActorID: "actor-a", TargetID: "target-a", TargetKind: "KubernetesTarget", IntentKind: "UpgradeRuntimeTarget", IntentDigest: "sha256:digest", ProjectionVersion: 7, ObservationGeneration: 2, ObservationRevision: 9, ObservedAt: now.Add(-time.Hour).Unix()}
	token, err := signer.Issue(claims)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Verify(token, claims); err != nil {
		t.Fatalf("verify: %v", err)
	}
	mutations := map[string]func(*ChallengeClaims){
		"tenant":      func(c *ChallengeClaims) { c.TenantID = "tenant-b" },
		"actor":       func(c *ChallengeClaims) { c.ActorID = "actor-b" },
		"target":      func(c *ChallengeClaims) { c.TargetID = "target-b" },
		"action":      func(c *ChallengeClaims) { c.IntentKind = "DeleteRuntimeTarget" },
		"digest":      func(c *ChallengeClaims) { c.IntentDigest = "sha256:other" },
		"projection":  func(c *ChallengeClaims) { c.ProjectionVersion++ },
		"observation": func(c *ChallengeClaims) { c.ObservationRevision++ },
	}
	for name, mutate := range mutations {
		wrong := claims
		mutate(&wrong)
		if err := signer.Verify(token, wrong); err == nil {
			t.Fatalf("%s replay accepted", name)
		}
	}
	if err := signer.Verify(token+"x", claims); err == nil {
		t.Fatal("tampered token accepted")
	}
	signer.now = func() time.Time { return now.Add(6 * time.Minute) }
	if err := signer.Verify(token, claims); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestChallengeRejectsWeakKeyAndInvalidTTL(t *testing.T) {
	if _, err := NewSigner([]byte("short"), 5*time.Minute); err == nil {
		t.Fatal("weak key accepted")
	}
	if _, err := NewSigner([]byte(strings.Repeat("k", 32)), 30*time.Minute); err == nil {
		t.Fatal("out-of-range TTL accepted")
	}
}
