package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func signBindingCredential(t *testing.T, keys testKeys, scope ServiceCredentialScope, now time.Time) string {
	t.Helper()
	membership := "service:" + uuid.NewString()
	claims := AccessTokenClaims{
		ProfileVersion: AccessTokenProfileVersion, Issuer: "https://issuer.example",
		Audiences: []string{scope.Audience}, SubjectID: scope.SubjectID, SubjectType: "service",
		TenantID: scope.Permission.TenantID, MembershipID: membership,
		TenantMembershipIDs: []string{membership}, PolicyVersion: "serviceIdentity:1",
		ScopedPermissions: []ScopedPermission{scope.Permission},
		AllowedActions:    []AuthorizationAction{scope.Permission.Action},
		IssuedAt:          now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(30 * time.Second).Unix(),
		AuthTime: now.Unix(), TokenID: uuid.NewString(), KeyID: keys.kid, Algorithm: AccessTokenAlgorithm,
	}
	token, err := signAccessToken(keys.key, keys.kid, claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestHTTPTenantTokenSourceBindsEachTenantAndRejectsUnbound(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeys{kid: "test-key-1", key: key}
	now := time.Unix(2_000_000_000, 0).UTC()
	dir := t.TempDir()
	writeBootstrap := func(name, value string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	subjectA, subjectB := uuid.NewString(), uuid.NewString()
	bindings := map[string]ServiceIdentityBinding{
		"tenant-a": {IdentityID: uuid.NewString(), SubjectID: subjectA, TokenFile: writeBootstrap("a", "bootstrap-a")},
		"tenant-b": {IdentityID: uuid.NewString(), SubjectID: subjectB, TokenFile: writeBootstrap("b", "bootstrap-b")},
	}
	bootstrapToTenant := map[string]string{"bootstrap-a": "tenant-a", "bootstrap-b": "tenant-b"}
	scopeFor := func(tenantID string) ServiceCredentialScope {
		binding := bindings[tenantID]
		return ServiceCredentialScope{
			SubjectID: binding.SubjectID, Audience: "provider-audience",
			Permission: ScopedPermission{ResourceKind: "runtimeProvider", ResourceID: "k8s", Action: ActionExecute, TenantID: tenantID},
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tenantID, ok := bootstrapToTenant[bearer]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"credential": signBindingCredential(t, keys, scopeFor(tenantID), now),
			"expiresAt":  now.Add(30 * time.Second).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	verifier, err := NewTokenVerifier(TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "provider-audience",
		AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now },
	}, keys)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewHTTPTenantTokenSource(context.Background(), verifier, server.URL, "k8s", "provider-audience", bindings, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	tokenA, err := source.TokenForTenant(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	claimsA, err := verifier.Verify(context.Background(), tokenA)
	if err != nil {
		t.Fatal(err)
	}
	if claimsA.SubjectID != subjectA || claimsA.TenantID != "tenant-a" {
		t.Fatalf("tenant-a received the wrong identity: %+v", claimsA)
	}
	tokenB, err := source.TokenForTenant(context.Background(), "tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	claimsB, err := verifier.Verify(context.Background(), tokenB)
	if err != nil {
		t.Fatal(err)
	}
	if claimsB.SubjectID != subjectB || claimsB.TenantID != "tenant-b" {
		t.Fatalf("tenant-b received the wrong identity: %+v", claimsB)
	}
	if tokenA == tokenB {
		t.Fatal("tenants shared a credential")
	}

	if _, err := source.TokenForTenant(context.Background(), "tenant-c"); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatalf("unbound tenant was accepted: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := source.TokenForTenant(context.Background(), "tenant-a"); !errors.Is(err, ErrServiceCredentialUnavailable) {
		t.Fatalf("closed source returned a token: %v", err)
	}
}

func TestHTTPTenantTokenSourceRejectsInvalidBindings(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeys{kid: "test-key-1", key: key}
	verifier, _ := NewTokenVerifier(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "provider-audience", AccessTTL: MaxAccessTokenTTL}, keys)
	valid := ServiceIdentityBinding{IdentityID: uuid.NewString(), SubjectID: uuid.NewString(), TokenFile: "/tmp/token"}
	for _, tc := range []struct {
		name     string
		provider string
		audience string
		bindings map[string]ServiceIdentityBinding
	}{
		{name: "empty bindings", provider: "k8s", audience: "provider-audience"},
		{name: "bad subject", provider: "k8s", audience: "provider-audience", bindings: map[string]ServiceIdentityBinding{
			"tenant-a": {IdentityID: valid.IdentityID, SubjectID: "not-a-uuid", TokenFile: valid.TokenFile},
		}},
		{name: "missing token file", provider: "k8s", audience: "provider-audience", bindings: map[string]ServiceIdentityBinding{
			"tenant-a": {IdentityID: valid.IdentityID, SubjectID: valid.SubjectID},
		}},
		{name: "wildcard audience", provider: "k8s", audience: "*", bindings: map[string]ServiceIdentityBinding{"tenant-a": valid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewHTTPTenantTokenSource(context.Background(), verifier, "https://issuer.example", tc.provider, tc.audience, tc.bindings, nil); err == nil {
				t.Fatal("invalid binding was accepted")
			}
		})
	}
}
