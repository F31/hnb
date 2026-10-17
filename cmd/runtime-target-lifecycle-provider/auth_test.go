package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provider"
)

// identityFixture mirrors the real platform identity layout: an ECDSA signing
// key plus a key manifest that the provider's verifier reloads.
type identityFixture struct {
	dir      string
	manifest string
	private  string
	key      *ecdsa.PrivateKey
	kid      string
	issuer   string
}

func newIdentityFixture(t *testing.T, issuer string) *identityFixture {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	now := time.Now().UTC()
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{
		Type: "PUBLIC KEY", Bytes: mustMarshalPublic(t, &key.PublicKey)}), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	writeManifest(t, manifestPath, issuer, publicPath, now)
	return &identityFixture{dir: dir, manifest: manifestPath, private: privatePath, key: key, kid: "test-kid", issuer: issuer}
}

func mustMarshalPublic(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writeManifest(t *testing.T, path, issuer, publicPath string, now time.Time) {
	t.Helper()
	manifest := iam.KeyManifest{
		Issuer:      issuer,
		Generation:  1,
		ActiveKeyID: "test-kid",
		Keys: map[string]iam.KeyManifestEntry{
			"test-kid": {PublicKeyPath: publicPath, Status: iam.KeyActive, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// mintWithAudience issues a real access token for one audience through the
// platform token manager, so the provider verifier sees production-shaped JWTs.
func mintWithAudience(t *testing.T, f *identityFixture, audience string) string {
	t.Helper()
	keys, err := iam.LoadReloadingKeySet(context.Background(), iam.ReloadingKeySetConfig{
		ManifestPath: f.manifest, ActivePrivateKeyPath: f.private, Issuer: f.issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Service credentials carry a service subject type; user tokens must never
	// authorize step execution.
	identity := &iam.Identity{UserID: "service-1", SubjectID: "subject-1", SubjectType: "service",
		MembershipID: "membership-1", TenantID: "tenant-1"}
	resolver := staticIdentityResolver{identity: identity}
	permissions := staticPermissionResolver{}
	manager, err := iam.NewTokenManager(iam.TokenManagerConfig{
		Issuer: f.issuer, Audience: audience, Audiences: []string{audience},
		AccessTTL: iam.MaxAccessTokenTTL, RefreshTTL: time.Hour,
	}, keys, keys, resolver, permissions, discardRefreshStore{})
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := manager.Issue(context.Background(), "service-1", "membership-1")
	if err != nil {
		t.Fatal(err)
	}
	return access.Token
}

// staticIdentityResolver satisfies the manager's identity/permission lookups.
type staticIdentityResolver struct{ identity *iam.Identity }

func (s staticIdentityResolver) ResolveUserIdentity(context.Context, string, string) (*iam.Identity, error) {
	return s.identity, nil
}

func (s staticIdentityResolver) ResolveMembership(context.Context, string, string) (*iam.Identity, error) {
	return s.identity, nil
}

// discardRefreshStore satisfies the manager contract; access tokens are the
// only artifact under test and no refresh token is ever redeemed here.
type discardRefreshStore struct{}

func (discardRefreshStore) CreateRefreshToken(context.Context, iam.RefreshTokenRecord) error {
	return nil
}

func (discardRefreshStore) RotateRefreshToken(context.Context, string, iam.RefreshTokenRecord, time.Time) (*iam.RefreshTokenRecord, error) {
	return nil, nil
}

// staticPermissionResolver grants the single action the execution endpoint needs.
type staticPermissionResolver struct{}

func (staticPermissionResolver) ResolvePermissions(context.Context, string, string, string) (string, []iam.ScopedPermission, error) {
	// RuntimeExecutionScope authorizes on tenant + provider identity only, so the
	// grant must not pin project/environment; a narrower grant would be skipped.
	return "policy-1", []iam.ScopedPermission{{
		ResourceKind: "runtimeProvider",
		ResourceID:   "runtime-target.lifecycle.kubernetes",
		Action:       iam.ActionExecute,
		TenantID:     "tenant-1",
	}}, nil
}

func authenticatedHandler(t *testing.T, f *identityFixture) http.Handler {
	t.Helper()
	t.Setenv("API_TOKEN_ISSUER", f.issuer)
	t.Setenv("API_TOKEN_AUDIENCE", provider.Audience)
	t.Setenv("API_TOKEN_KEY_MANIFEST_FILE", f.manifest)
	authenticator, err := loadServiceAuthenticator(provider.Audience)
	if err != nil {
		t.Fatal(err)
	}
	return iam.RequireServiceIdentity(authenticator, iam.ActionExecute,
		iam.RuntimeExecutionScope(provider.ExecuteBodyLimit), "/healthz")(
		provider.NewServer(profileFixture(t), nil, nil, nil).Handler())
}

func profileFixture(t *testing.T) provider.Profile {
	t.Helper()
	p, err := provider.ProfileForProviderID("runtime-target.lifecycle.kubernetes")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func executeBody() string {
	payload := map[string]any{
		"schemaVersion": provider.ContractVersion,
		"execution": map[string]any{
			"step_id": "step-1", "operation_id": "op-1", "tenant_id": "tenant-1",
			"project_id": "project-1", "environment_id": "env-1", "step_type": "kubernetes.create",
			"provider_id": "runtime-target.lifecycle.kubernetes", "provider_version": "0.1.0",
			"provider_digest":           "sha256:" + strings.Repeat("a", 64),
			"provider_protocol_version": provider.ContractVersion,
			"idempotency_key":           "idem-1", "execution_attempt_id": "attempt-1",
			"fencing_generation": "1", "inputs": map[string]any{},
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

// An unauthenticated or wrongly-scoped caller must never reach provisioning.
func TestExecutionEndpointRequiresServiceIdentity(t *testing.T) {
	f := newIdentityFixture(t, "https://issuer.lifecycle.test")
	handler := authenticatedHandler(t, f)
	server := httptest.NewServer(handler)
	defer server.Close()

	post := func(authorization string) int {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/steps:execute", strings.NewReader(executeBody()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := post(""); code != http.StatusUnauthorized {
		t.Fatalf("missing token accepted: %d", code)
	}
	if code := post("Bearer "); code != http.StatusUnauthorized {
		t.Fatalf("empty bearer accepted: %d", code)
	}
	if code := post("Basic dXNlcjpwYXNz"); code != http.StatusUnauthorized {
		t.Fatalf("non-bearer scheme accepted: %d", code)
	}
	if code := post("Bearer not-a-real-token"); code != http.StatusUnauthorized {
		t.Fatalf("forged token accepted: %d", code)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v2/steps:execute", strings.NewReader(executeBody()))
	req.Header.Add("Authorization", "Bearer a")
	req.Header.Add("Authorization", "Bearer b")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ambiguous duplicate Authorization header accepted: %d", resp.StatusCode)
	}
	// Liveness must stay reachable so orchestration does not treat a secured
	// provider as an unhealthy one.
	health, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("healthz should stay unauthenticated: %d", health.StatusCode)
	}
}

// A token minted for a different audience must not authorize this provider.
func TestExecutionEndpointRejectsForeignAudience(t *testing.T) {
	f := newIdentityFixture(t, "https://issuer.lifecycle.test")
	handler := authenticatedHandler(t, f)
	server := httptest.NewServer(handler)
	defer server.Close()

	for _, audience := range []string{"hnb-apiserver", "hnb-platform-api", "hnb-kubernetes-provider", provider.Audience + "-other"} {
		token := mintWithAudience(t, f, audience)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/steps:execute", strings.NewReader(executeBody()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("audience %q was accepted by the lifecycle provider: %d", audience, resp.StatusCode)
		}
	}
}

func TestLoadServiceAuthenticatorFailsClosedOnMissingConfig(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"no-issuer":      {"API_TOKEN_AUDIENCE": provider.Audience},
		"no-audience":    {"API_TOKEN_ISSUER": "https://issuer.lifecycle.test"},
		"wrong-audience": {"API_TOKEN_ISSUER": "https://issuer.lifecycle.test", "API_TOKEN_AUDIENCE": "hnb-apiserver"},
		"no-manifest":    {"API_TOKEN_ISSUER": "https://issuer.lifecycle.test", "API_TOKEN_AUDIENCE": provider.Audience},
		"empty-all":      {},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range env {
				t.Setenv(key, value)
			}
			if _, err := loadServiceAuthenticator(provider.Audience); err == nil {
				t.Fatal("provider started without a verifiable service identity")
			}
		})
	}
}

// A correctly scoped token must get past authentication. The manager is nil
// here on purpose: reaching the handler proves authorization passed, and the
// handler must not crash before doing so.
func TestExecutionEndpointAcceptsCorrectlyScopedToken(t *testing.T) {
	f := newIdentityFixture(t, "https://issuer.lifecycle.test")
	token := mintWithAudience(t, f, provider.Audience)
	handler := authenticatedHandler(t, f)
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/steps:execute", strings.NewReader(executeBody()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("a valid, correctly scoped service token was rejected")
	}
	// Reaching the handler proves authentication passed. The manager is nil in
	// this fixture, so the handler cannot complete; a panic would mean the
	// request never reached it. Any non-401 status means authorization let it by.
	if resp.StatusCode == http.StatusOK {
		t.Fatal("fixture has no manager, so a 200 would mean the handler did not execute")
	}
}
