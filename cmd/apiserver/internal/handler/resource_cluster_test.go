package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

func resourceTrustedContext() iam.TrustedContext {
	return iam.TrustedContext{SubjectID: "subject-a", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a", PolicyVersion: "default:1", CorrelationID: "018f6c2a-4a64-7b58-9cc3-9f70462f36c1", ScopedPermissions: []iam.ScopedPermission{
		{TenantID: "tenant-a", ResourceKind: "cluster", Action: iam.ActionCreate},
	}}
}

type resourceDelegationKeys struct{ key *ecdsa.PrivateKey }

func (k resourceDelegationKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return "delegation-key", k.key, nil
}

func newDelegatingResourceHandler(t *testing.T, platformURL string) *ResourceClusterHandler {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := iam.NewDelegationSigner(iam.DelegationConfig{
		Issuer: "https://issuer.example", Audience: "hnb-platform-api", ServiceSubject: "hnb-apiserver", TTL: 30 * time.Second,
	}, resourceDelegationKeys{key: key})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewResourceClusterHandler(nil, platformURL)
	handler.ConfigureDelegation(signer)
	return handler
}

func withTrusted(req *http.Request) *http.Request {
	return req.WithContext(iam.WithTrustedContext(req.Context(), resourceTrustedContext()))
}

func validImportIntentBody(idempotencyKey string) string {
	return `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"` + idempotencyKey + `","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","credentialSecretRef":{"provider":"config","scope":"tenant-a","name":"kubeconfig-a"}}}`
}

func TestResourceClusterHandlerForwardsIntentToPlatformAPI(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		got := r.Header.Get("Authorization")
		if !strings.HasPrefix(got, "Bearer ey") || got == "Bearer token" {
			t.Errorf("delegated auth header = %q", got)
		}
		parts := strings.Split(strings.TrimPrefix(got, "Bearer "), ".")
		if len(parts) != 3 {
			t.Fatalf("delegation is not a compact JWS")
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatal(err)
		}
		if claims["sub"] != "hnb-apiserver" || claims["actorSubject"] != "subject-a" || claims["tenantId"] != "tenant-a" ||
			claims["membershipId"] != "membership-a" || claims["correlationId"] != "018f6c2a-4a64-7b58-9cc3-9f70462f36c1" ||
			claims["intentKind"] != "ImportRuntimeTarget" || claims["action"] != "create" || claims["semanticDigest"] != r.Header.Get("X-Semantic-Digest") {
			t.Fatalf("unexpected delegation claims: %#v", claims)
		}
		if got := r.Header.Get("X-Tenant-ID"); got != "" {
			t.Errorf("untrusted tenant header forwarded = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"intentId":"cluster-1","operationId":"op-1","planId":"plan-1","kind":"ImportRuntimeTarget","status":"queued","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1","createdAt":"2026-08-01T00:00:00Z","semanticDigest":"sha256:platform","replayed":false}`))
	}))
	defer platform.Close()

	handler := newDelegatingResourceHandler(t, platform.URL)
	body := validImportIntentBody("cluster-1")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(body)))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req.Header.Set("X-Correlation-ID", "018f6c2a-4a64-7b58-9cc3-9f70462f36c1")
	rec := httptest.NewRecorder()

	handler.SubmitRuntimeIntent(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	for _, want := range []string{`"intentId":"cluster-1"`, `"operationId":"op-1"`, `"executionPlanId":"plan-1"`, `"status":"operationCommitted"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("response missing %s: %s", want, got)
		}
	}
}

func TestResourceClusterHandlerRejectsInvalidIntent(t *testing.T) {
	handler := NewResourceClusterHandler(nil, "")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents",
		strings.NewReader(`{"apiVersion":"hnb.io/v1","kind":"Bogus","metadata":{"idempotencyKey":"k1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetRef":"t","scopeRef":"s"}}`)))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResourceClusterHandlerRejectsNonClusterIntentBeforeForwarding(t *testing.T) {
	forwarded := false
	platform := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded = true }))
	defer platform.Close()
	handler := NewResourceClusterHandler(nil, platform.URL)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents",
		strings.NewReader(`{"apiVersion":"hnb.io/v1","kind":"InstallRelease","metadata":{"idempotencyKey":"k1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetRef":"t","scopeRef":"s"}}`)))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusBadRequest || forwarded {
		t.Fatalf("status = %d, forwarded = %v", rec.Code, forwarded)
	}
}

func TestResourceClusterHandlerMapsPlatformUnavailable(t *testing.T) {
	handler := newDelegatingResourceHandler(t, "http://127.0.0.1:1")
	handler.client = &http.Client{Timeout: 50 * time.Millisecond}
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents",
		strings.NewReader(validImportIntentBody("k1"))))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" || !strings.Contains(rec.Body.String(), `"code":"UPSTREAM_UNAVAILABLE"`) || !strings.Contains(rec.Body.String(), `"retryable":true`) {
		t.Fatalf("unexpected problem: %s", rec.Body.String())
	}
}

func TestResourceClusterHandlerMapsBoundedDomainProblem(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"https://internal/problems/conflict","title":"Internal detail","status":409,"detail":"password=secret token=abc","code":"IDEMPOTENCY_CONFLICT","correlationId":"attacker","traceId":"attacker","stack":"sensitive stack","violations":[{"field":"spec.targetId","code":"INVALID_VALUE","message":"invalid"}]}`))
	}))
	defer platform.Close()
	handler := newDelegatingResourceHandler(t, platform.URL)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(validImportIntentBody("problem-map"))))
	req.Header.Set("X-Correlation-ID", "018f6c2a-4a64-7b58-9cc3-9f70462f36c1")
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusConflict || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"password", "secret", "token=", "sensitive stack", "attacker"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("problem leaked %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{`"code":"IDEMPOTENCY_CONFLICT"`, `"status":409`, `"correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"`, `"violations"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("problem missing %s: %s", required, body)
		}
	}
}

func TestResourceClusterHandlerRejectsMalformedUpstreamProblem(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<html>password=secret internal-host:5432</html>`))
	}))
	defer platform.Close()
	handler := newDelegatingResourceHandler(t, platform.URL)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(validImportIntentBody("malformed-problem"))))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"UPSTREAM_UNAVAILABLE"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "5432") {
		t.Fatalf("upstream body leaked: %s", rec.Body.String())
	}
}

func TestResourceClusterHandlerRejectsIdempotencyHeaderMismatch(t *testing.T) {
	handler := NewResourceClusterHandler(nil, "")
	body := validImportIntentBody("key-body")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(body)))
	req.Header.Set("Idempotency-Key", "key-header")
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestResourceClusterHandlerRejectsCorrelationHeaderMismatch(t *testing.T) {
	handler := NewResourceClusterHandler(nil, "")
	body := validImportIntentBody("k1")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(body)))
	req.Header.Set("X-Correlation-ID", "different-id")
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestResourceClusterHandlerRejectsCallerSelectedProviderAndSteps(t *testing.T) {
	handler := NewResourceClusterHandler(nil, "")
	for _, injected := range []string{
		`{"providerId":"runtime-target.lifecycle.kubernetes"}`,
		`{"steps":[{"type":"shell"}]}`,
	} {
		body := `{"apiVersion":"hnb.io/v1","kind":"ImportRuntimeTarget","metadata":{"idempotencyKey":"k1","correlationId":"018f6c2a-4a64-7b58-9cc3-9f70462f36c1"},"spec":{"targetKind":"KubernetesTarget","displayName":"cluster-a","parameters":` + injected + `}}`
		req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(body)))
		rec := httptest.NewRecorder()
		handler.SubmitRuntimeIntent(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("injected=%s status=%d body=%s", injected, rec.Code, rec.Body.String())
		}
	}
}

func TestResourceClusterHandlerForwardsSemanticDigest(t *testing.T) {
	var receivedDigest string
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedDigest = r.Header.Get("X-Semantic-Digest")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"intentId":"i1","operationId":"op-1","planId":"p1","status":"queued","createdAt":"2026-08-01T00:00:00Z","semanticDigest":"sha256:platform","replayed":false}`))
	}))
	defer platform.Close()

	handler := newDelegatingResourceHandler(t, platform.URL)
	body := validImportIntentBody("k1")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	if receivedDigest == "" {
		t.Fatal("X-Semantic-Digest header not forwarded")
	}
	if !strings.HasPrefix(receivedDigest, "sha256:") {
		t.Fatalf("unexpected digest format: %s", receivedDigest)
	}
}

func TestBFFIntentSemanticDigestIgnoresTransportMetadataAndMapOrder(t *testing.T) {
	var first, second bffIntentEnvelope
	if err := json.Unmarshal([]byte(validImportIntentBody("key-a")), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(validImportIntentBody("key-b")), &second); err != nil {
		t.Fatal(err)
	}
	first.Spec.Parameters = map[string]any{"b": 2, "a": 1}
	second.Spec.Parameters = map[string]any{"a": 1, "b": 2}
	second.Metadata.CorrelationID = "018f6c2a-4a64-7b58-9cc3-9f70462f36c2"
	if bffIntentSemanticDigest(first) != bffIntentSemanticDigest(second) {
		t.Fatal("transport metadata or map order changed semantic digest")
	}
}

func TestResourceClusterHandlerFailsClosedWithoutPlatformAPI(t *testing.T) {
	handler := NewResourceClusterHandler(nil, "")
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(validImportIntentBody("no-platform"))))
	rec := httptest.NewRecorder()
	handler.SubmitRuntimeIntent(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResourceClusterHandlerForwardsDescriptionUpdate(t *testing.T) {
	const targetID = "3b9b7e0e-5a1c-4f2d-9b4a-7c6d5e4f3a2b"
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/clusters/"+targetID+"/description" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Tenant-ID"); got != "" {
			t.Errorf("untrusted tenant header forwarded = %q", got)
		}
		if got := r.Header.Get("X-Correlation-ID"); got != "018f6c2a-4a64-7b58-9cc3-9f70462f36c1" {
			t.Errorf("correlation header = %q", got)
		}
		got := r.Header.Get("Authorization")
		parts := strings.Split(strings.TrimPrefix(got, "Bearer "), ".")
		if len(parts) != 3 {
			t.Fatalf("delegation is not a compact JWS")
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatal(err)
		}
		scope, ok := claims["scope"].(map[string]any)
		if !ok || scope["resourceKind"] != "cluster-metadata" || scope["resourceId"] != targetID {
			t.Fatalf("unexpected delegation scope: %#v", claims["scope"])
		}
		if claims["action"] != "update" || claims["intentKind"] != "" || claims["semanticDigest"] != "" {
			t.Fatalf("unexpected delegation claims: %#v", claims)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"updated"}`))
	}))
	defer platform.Close()

	handler := newDelegatingResourceHandler(t, platform.URL)
	body := `{"description":"edge cluster note"}`
	req := withTrusted(httptest.NewRequest(http.MethodPatch, "/api/v1/resources/clusters/"+targetID+"/description", strings.NewReader(body)))
	req.SetPathValue("id", targetID)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req.Header.Set("X-Correlation-ID", "018f6c2a-4a64-7b58-9cc3-9f70462f36c1")
	rec := httptest.NewRecorder()

	handler.UpdateClusterDescription(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"updated"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestResourceClusterHandlerForwardsKubeconfigDownload(t *testing.T) {
	const targetID = "3b9b7e0e-5a1c-4f2d-9b4a-7c6d5e4f3a2b"
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/clusters/"+targetID+"/kubeconfig:issue" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		got := r.Header.Get("Authorization")
		parts := strings.Split(strings.TrimPrefix(got, "Bearer "), ".")
		if len(parts) != 3 {
			t.Fatalf("delegation is not a compact JWS")
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatal(err)
		}
		scope, ok := claims["scope"].(map[string]any)
		if !ok || scope["resourceKind"] != "cluster-metadata" || scope["resourceId"] != targetID {
			t.Fatalf("unexpected delegation scope: %#v", claims["scope"])
		}
		if claims["action"] != "execute" || claims["intentKind"] != "" || claims["semanticDigest"] != "" {
			t.Fatalf("unexpected delegation claims: %#v", claims)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kubeconfig":"apiVersion: v1\nclusters: []","filename":"` + targetID + `.kubeconfig"}`))
	}))
	defer platform.Close()

	handler := newDelegatingResourceHandler(t, platform.URL)
	req := withTrusted(httptest.NewRequest(http.MethodPost, "/api/v1/resources/clusters/"+targetID+"/kubeconfig:download", nil))
	req.SetPathValue("id", targetID)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req.Header.Set("X-Correlation-ID", "018f6c2a-4a64-7b58-9cc3-9f70462f36c1")
	rec := httptest.NewRecorder()

	handler.DownloadKubeConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"filename":"`+targetID+`.kubeconfig"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
