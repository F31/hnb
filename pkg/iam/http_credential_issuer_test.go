package iam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

type failingTokenSource struct{}

func (failingTokenSource) Token(context.Context) (string, error) {
	return "", errors.New("bootstrap unavailable")
}

func craftedAccessToken(t *testing.T, tenantID, subjectID, audience string, permission ScopedPermission) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"aud": []string{audience}, "sub": subjectID, "tenantId": tenantID,
		"jti": uuid.NewString(), "iss": "https://issuer.example",
		"scopedPermissions": []ScopedPermission{permission},
		"allowedActions":    []AuthorizationAction{permission.Action},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".c2ln"
}

func testServiceCredentialScope(t *testing.T) ServiceCredentialScope {
	t.Helper()
	return ServiceCredentialScope{
		SubjectID: uuid.NewString(), Audience: "provider-audience",
		Permission: ScopedPermission{ResourceKind: "runtimeProvider", ResourceID: "provider-a", Action: ActionExecute, TenantID: "tenant-a"},
	}
}

func TestHTTPCredentialIssuerRenewsAndRotatesBootstrap(t *testing.T) {
	scope := testServiceCredentialScope(t)
	now := time.Unix(2_000_000_000, 0).UTC()
	first := craftedAccessToken(t, scope.Permission.TenantID, scope.SubjectID, scope.Audience, scope.Permission)
	second := craftedAccessToken(t, scope.Permission.TenantID, scope.SubjectID, scope.Audience, scope.Permission)

	var mu sync.Mutex
	presented := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/service-credentials:renew" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		presented = append(presented, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		count := len(presented)
		mu.Unlock()
		credential := first
		if count > 1 {
			credential = second
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"credential": credential,
			"expiresAt":  now.Add(60 * time.Second).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	issuer, err := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{
		BaseURL: server.URL, Bootstrap: staticTokenSource("bootstrap-token"),
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := issuer.IssueServiceCredential(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatal("issuer returned an unexpected credential")
	}
	if _, err := issuer.IssueServiceCredential(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(presented) != 2 || presented[0] != "bootstrap-token" || presented[1] != first {
		t.Fatalf("unexpected bearer rotation: %v", presented)
	}
}

func TestHTTPCredentialIssuerRejectsMismatchWithoutLeakingSecrets(t *testing.T) {
	scope := testServiceCredentialScope(t)
	now := time.Unix(2_000_000_000, 0).UTC()

	t.Run("scope mismatch", func(t *testing.T) {
		other := craftedAccessToken(t, scope.Permission.TenantID, uuid.NewString(), scope.Audience, scope.Permission)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"credential": other, "expiresAt": now.Add(time.Minute).Format(time.RFC3339)})
		}))
		defer server.Close()
		issuer, _ := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{BaseURL: server.URL, Bootstrap: staticTokenSource("bootstrap-token"), Now: func() time.Time { return now }})
		_, err := issuer.IssueServiceCredential(context.Background(), scope)
		if !errors.Is(err, ErrServiceCredentialRejected) {
			t.Fatalf("scope mismatch was accepted: %v", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("sensitive-server-body"))
		}))
		defer server.Close()
		issuer, _ := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{BaseURL: server.URL, Bootstrap: staticTokenSource("bootstrap-token"), Now: func() time.Time { return now }})
		_, err := issuer.IssueServiceCredential(context.Background(), scope)
		if !errors.Is(err, ErrServiceCredentialRejected) {
			t.Fatalf("unauthorized renewal was not a permanent rejection: %v", err)
		}
		if strings.Contains(err.Error(), "sensitive-server-body") || strings.Contains(err.Error(), "bootstrap-token") {
			t.Fatalf("error leaked a credential or server body: %v", err)
		}
	})

	t.Run("server failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		issuer, _ := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{BaseURL: server.URL, Bootstrap: staticTokenSource("bootstrap-token"), Now: func() time.Time { return now }})
		_, err := issuer.IssueServiceCredential(context.Background(), scope)
		if !errors.Is(err, ErrServiceCredentialUnavailable) {
			t.Fatalf("server failure was not temporary: %v", err)
		}
	})

	t.Run("bootstrap failure", func(t *testing.T) {
		issuer, _ := NewHTTPCredentialIssuer(HTTPCredentialIssuerConfig{BaseURL: "https://issuer.example", Bootstrap: failingTokenSource{}, Now: func() time.Time { return now }})
		_, err := issuer.IssueServiceCredential(context.Background(), scope)
		if !errors.Is(err, ErrServiceCredentialUnavailable) {
			t.Fatalf("missing bootstrap was not temporary: %v", err)
		}
	})
}
