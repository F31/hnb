package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/F31/hnb/pkg/appstore/store"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

type appTestAuthenticator struct{}
type appNoPermissionAuthenticator struct{}

func (appTestAuthenticator) Authenticate(_ context.Context, token, correlationID, traceparent string) (iam.TrustedContext, error) {
	if token != "valid-token" {
		return iam.TrustedContext{}, fmt.Errorf("invalid token")
	}
	return iam.TrustedContext{
		SubjectID: "signed-subject", SubjectType: "user", TenantID: "signed-tenant", MembershipID: "membership-a",
		PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{ResourceKind: "capture", Action: iam.ActionCreate, TenantID: "signed-tenant"}},
		CorrelationID: correlationID, Traceparent: traceparent,
	}, nil
}

func (appNoPermissionAuthenticator) Authenticate(_ context.Context, token, correlationID, traceparent string) (iam.TrustedContext, error) {
	if token != "valid-token" {
		return iam.TrustedContext{}, fmt.Errorf("invalid token")
	}
	return iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", CorrelationID: correlationID, Traceparent: traceparent}, nil
}

type appTestKeyRing struct{ key *ecdsa.PublicKey }

func (k appTestKeyRing) VerificationKey(_ context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid != "key-1" {
		return nil, fmt.Errorf("unknown key")
	}
	return k.key, nil
}

func TestAppMarketProtectsAPIAndOverridesSpoofedIdentity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /api/v1/capture", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Publisher   appstore.Publisher   `json:"publisher"`
			Release     appstore.Release     `json:"release"`
			Application appstore.Application `json:"application"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		applyPublisherIdentity(r, &input.Publisher)
		applyReleaseIdentity(r, &input.Release)
		applyApplicationIdentity(r, &input.Application)
		trusted, ok := iam.TrustedContextFrom(r.Context())
		if !ok || trusted.MembershipID != "membership-a" || r.Header.Get("X-Tenant-ID") != "" || r.Header.Get("X-User-ID") != "" {
			http.Error(w, "missing trusted context", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(input)
	})
	handler := appMarketHTTPHandlerWithRoutes(appTestAuthenticator{}, mux, []iam.RouteMetadata{
		{Method: http.MethodGet, Pattern: "/health", Public: true},
		{Method: http.MethodPost, Pattern: "/api/v1/capture", ResourceKind: "capture", Action: iam.ActionCreate},
	})

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodPost, "/api/v1/capture", strings.NewReader(`{}`)))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d", unauthenticated.Code)
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusNoContent {
		t.Fatalf("health status = %d", health.Code)
	}

	body := `{"publisher":{"tenant_id":"spoofed"},"release":{"created_by":"spoofed"},"application":{"tenant_id":"spoofed"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/capture", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("X-Tenant-ID", "header-spoof")
	req.Header.Set("X-User-ID", "header-spoof")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"tenant_id":"signed-tenant"`) || !strings.Contains(rec.Body.String(), `"created_by":"signed-subject"`) {
		t.Fatalf("spoofed identity was not overridden: %s", rec.Body)
	}
}

func TestAppMarketRejectsWrongAudience(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claims := iam.AccessTokenClaims{
		ProfileVersion: iam.AccessTokenProfileVersion, Issuer: "https://issuer.example", Audiences: []string{"other-service"},
		SubjectID: "subject", SubjectType: "user", TenantID: "tenant", MembershipID: "membership",
		TenantMembershipIDs: []string{"membership"}, IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(iam.MaxAccessTokenTTL).Unix(),
		AuthTime: now.Unix(), TokenID: "token-1", KeyID: "key-1", Algorithm: iam.AccessTokenAlgorithm,
	}
	token := signAppTestToken(t, privateKey, claims)
	verifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "hnb-app-market", AccessTTL: iam.MaxAccessTokenTTL,
	}, appTestKeyRing{key: &privateKey.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	handler := appMarketHTTPHandler(verifier, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestLoadConfigRequiresPublicVerifier(t *testing.T) {
	t.Setenv("API_TOKEN_ISSUER", "")
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("missing verifier configuration was accepted")
	}
}

func TestLoadConfigRejectsReloadIntervalOverPropagationBound(t *testing.T) {
	t.Setenv("API_TOKEN_ISSUER", "https://issuer.example")
	t.Setenv("API_TOKEN_AUDIENCE", "hnb-app-market")
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", "/keys/manifest.json")
	t.Setenv("API_TOKEN_KEY_RELOAD_INTERVAL", "61s")
	if _, err := loadConfig(); err == nil {
		t.Fatal("polling over 60 seconds was accepted")
	}
}

func TestAppMarketEveryProtectedRouteDeniesBeforeHandlerWithoutPermission(t *testing.T) {
	called := 0
	handler := appMarketHTTPHandler(appNoPermissionAuthenticator{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ }))
	for _, route := range appMarketRoutes {
		if route.Public {
			continue
		}
		path := strings.NewReplacer("{productId}", uuid.NewString(), "{id}", uuid.NewString()).Replace(route.Pattern)
		req := httptest.NewRequest(route.Method, path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer valid-token")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d", route.Method, path, recorder.Code)
		}
	}
	if called != 0 {
		t.Fatalf("protected handler calls = %d", called)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden || called != 0 {
		t.Fatalf("unknown route status = %d, calls = %d", recorder.Code, called)
	}
}

func TestWriteMarketRepoErrorMapsForeignUUIDToNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeMarketRepoError(recorder, sql.ErrNoRows)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestWriteReleaseArtifactErrorMapsDraftAndArtifactStateToConflict(t *testing.T) {
	for _, err := range []error{store.ErrUploadReleaseState, store.ErrArtifactNotAttachable} {
		recorder := httptest.NewRecorder()
		writeReleaseArtifactError(recorder, err)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("error %v status = %d", err, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	writeReleaseArtifactError(recorder, sql.ErrNoRows)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing entity status = %d", recorder.Code)
	}
}

func TestDeprecatedArtifactUploadReturnsGuidanceWithoutReadingBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload", errReader{})
	deprecatedArtifactUploadHandler(recorder, request)
	if recorder.Code != http.StatusGone {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "/api/v1/artifacts/session") {
		t.Fatalf("response does not contain session guidance: %s", recorder.Body.String())
	}
}

func TestAppMarketDoesNotExposeImmediateArtifactDeleteRoute(t *testing.T) {
	for _, route := range appMarketRoutes {
		if route.Method == http.MethodDelete && strings.Contains(route.Pattern, "/api/v1/artifacts") {
			t.Fatalf("immediate artifact delete route is exposed: %+v", route)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("body must not be read") }

func signAppTestToken(t *testing.T, key *ecdsa.PrivateKey, claims iam.AccessTokenClaims) string {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]string{"typ": iam.AccessTokenType, "alg": iam.AccessTokenAlgorithm, "kid": "key-1"})
	claimsJSON, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
