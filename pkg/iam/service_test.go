package iam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceAuthenticatorRejectsUserAndWrongScope(t *testing.T) {
	manager, _, _, keys, now := newTestManager(t)
	userToken, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewTokenVerifier(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "hnb-kubernetes-provider", AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now }}, keys)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, _ := NewServiceAuthenticator(verifier)
	if _, err := authenticator.Authenticate(context.Background(), userToken.Token, ActionExecute, "tenant-1", "runtimeProvider", "k8s"); err == nil {
		t.Fatal("user token was accepted as a service credential")
	}

	valid := serviceTokenClaims(t, manager, now)
	tests := map[string]AccessTokenClaims{
		"wrong audience": func() AccessTokenClaims { c := valid; c.Audiences = []string{"other-provider"}; return c }(),
		"multiple audiences": func() AccessTokenClaims {
			c := valid
			c.Audiences = []string{"hnb-kubernetes-provider", "other-provider"}
			return c
		}(),
		"wrong action": func() AccessTokenClaims {
			c := valid
			c.AllowedActions = []AuthorizationAction{ActionRead}
			c.ScopedPermissions = []ScopedPermission{{ResourceKind: "runtimeProvider", ResourceID: "k8s", Action: ActionRead, TenantID: "tenant-1"}}
			return c
		}(),
		"wrong tenant": func() AccessTokenClaims {
			c := valid
			c.TenantID = "tenant-2"
			c.ScopedPermissions = []ScopedPermission{{ResourceKind: "runtimeProvider", ResourceID: "k8s", Action: ActionExecute, TenantID: "tenant-2"}}
			return c
		}(),
		"expired":         func() AccessTokenClaims { c := valid; c.ExpiresAt = now.Unix(); return c }(),
		"wildcard action": func() AccessTokenClaims { c := valid; c.AllowedActions = []AuthorizationAction{"*"}; return c }(),
		"wildcard tenant": func() AccessTokenClaims {
			c := valid
			c.TenantID = "*"
			c.ScopedPermissions = []ScopedPermission{{ResourceKind: "runtimeProvider", ResourceID: "k8s", Action: ActionExecute, TenantID: "*"}}
			return c
		}(),
	}
	for name, claims := range tests {
		t.Run(name, func(t *testing.T) {
			token := signTestToken(t, keys.key, map[string]string{"typ": AccessTokenType, "alg": AccessTokenAlgorithm, "kid": keys.kid}, claims)
			if _, err := authenticator.Authenticate(context.Background(), token, ActionExecute, "tenant-1", "runtimeProvider", "k8s"); err == nil {
				t.Fatal("invalid service token was accepted")
			}
		})
	}
}

func TestRequireServiceIdentityStopsBeforeHandler(t *testing.T) {
	_, _, _, keys, now := newTestManager(t)
	verifier, _ := NewTokenVerifier(TokenManagerConfig{Issuer: "https://issuer.example", Audience: "hnb-kubernetes-provider", AccessTTL: MaxAccessTokenTTL, Now: func() time.Time { return now }}, keys)
	authenticator, _ := NewServiceAuthenticator(verifier)
	called := false
	handler := RequireServiceIdentity(authenticator, ActionExecute, RuntimeExecutionScope(1024), "/healthz")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v2/steps:execute", strings.NewReader(`{"execution":{"tenant_id":"tenant-1","provider_id":"k8s"}}`))
	req.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("status = %d, handler called = %v", response.Code, called)
	}
}

func TestFileTokenSourceMissingPermissionsAndRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	source := FileTokenSource{Path: path}
	if _, err := source.Token(context.Background()); err == nil {
		t.Fatal("missing token file was accepted")
	}
	if err := os.WriteFile(path, []byte("first.token.value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Token(context.Background()); err == nil {
		t.Fatal("over-permissive token file was accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := source.Token(context.Background()); err != nil || got != "first.token.value" {
		t.Fatalf("first token = %q, err = %v", got, err)
	}
	if err := os.WriteFile(path, []byte("rotated.token.value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := source.Token(context.Background()); err != nil || got != "rotated.token.value" {
		t.Fatalf("rotated token = %q, err = %v", got, err)
	}
}

func serviceTokenClaims(t *testing.T, manager *TokenManager, now time.Time) AccessTokenClaims {
	t.Helper()
	access, _, err := manager.Issue(context.Background(), "user-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Verify(context.Background(), access.Token)
	if err != nil {
		t.Fatal(err)
	}
	claims.SubjectType = "workload"
	claims.Audiences = []string{"hnb-kubernetes-provider"}
	claims.ScopedPermissions = []ScopedPermission{{ResourceKind: "runtimeProvider", ResourceID: "k8s", Action: ActionExecute, TenantID: "tenant-1"}}
	claims.AllowedActions = []AuthorizationAction{ActionExecute}
	claims.IssuedAt = now.Unix()
	claims.NotBefore = now.Unix()
	claims.ExpiresAt = now.Add(MaxAccessTokenTTL).Unix()
	return *claims
}
