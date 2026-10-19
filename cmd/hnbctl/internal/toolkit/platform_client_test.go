package toolkit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// intentFixture emulates the server contract: idempotency replay plus a
// server-side scope check on secret references.
func intentFixture(t *testing.T) (*httptest.Server, *int, *[]string) {
	t.Helper()
	calls := 0
	var seenScopes []string
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runtime-intents" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var envelope intentEnvelope
		if json.Unmarshal(body, &envelope) != nil || envelope.APIVersion != "hnb.io/v1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls++
		for _, ref := range envelope.Spec.SecretReferences {
			seenScopes = append(seenScopes, ref.Scope)
		}
		if envelope.Spec.SSHSecretRef != nil {
			seenScopes = append(seenScopes, envelope.Spec.SSHSecretRef.Scope)
		}
		key := r.Header.Get("Idempotency-Key")
		replayed := calls > 1 && key != ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(intentResponse{APIVersion: "hnb.io/v1", IntentID: "intent-1",
			Status: "accepted", OperationID: "op-1", Replayed: replayed, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	}))
	t.Cleanup(server.Close)
	return server, &calls, &seenScopes
}

// 同义请求幂等: the same idempotency key replays the same operation.
func TestSubmitIntentIsIdempotentOnReplay(t *testing.T) {
	server, calls, _ := intentFixture(t)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	first, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem-key-1", "corr-1", IntentSpec{TargetKind: "kubernetes"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem-key-1", "corr-1", IntentSpec{TargetKind: "kubernetes"})
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID || first.IntentID != second.IntentID {
		t.Fatalf("replay produced a different operation: %+v vs %+v", first, second)
	}
	if !second.Replayed || first.Replayed {
		t.Fatalf("replayed flag wrong: first=%+v second=%+v", first, second)
	}
	if *calls != 2 {
		t.Fatalf("expected both requests to reach the idempotent server, got %d", *calls)
	}
}

// A missing idempotency key is refused before any network call.
func TestSubmitIntentRequiresIdempotencyKey(t *testing.T) {
	server, calls, _ := intentFixture(t)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	if _, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "  ", "corr-1", IntentSpec{TargetKind: "kubernetes"}); err == nil {
		t.Fatal("empty idempotency key accepted")
	}
	if *calls != 0 {
		t.Fatal("empty idempotency key still produced a request")
	}
}

// 跨租户 Secret 拒绝: a reference outside the authenticated scope is refused
// locally and never sent.
func TestCrossTenantSecretReferenceIsRefused(t *testing.T) {
	server, calls, seenScopes := intentFixture(t)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", TenantScope: "tenant-a", Client: server.Client()}
	spec := IntentSpec{TargetKind: "kubernetes", SecretReferences: []IntentSecretRef{{Provider: "vault", Scope: "tenant-b", Name: "other-tenant-secret"}}}
	if _, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", spec); err == nil || !strings.Contains(err.Error(), "outside the authenticated tenant scope") {
		t.Fatalf("cross-tenant secret reference accepted: %v", err)
	}
	if *calls != 0 || len(*seenScopes) != 0 {
		t.Fatal("cross-tenant secret reference was sent to the platform")
	}
	// Same-tenant reference passes the local check.
	ok := IntentSpec{TargetKind: "kubernetes", SecretReferences: []IntentSecretRef{{Provider: "vault", Scope: "tenant-a", Name: "my-secret"}}}
	if _, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", ok); err != nil {
		t.Fatalf("same-tenant secret reference rejected: %v", err)
	}
}

// 503 不回退 SSH: availability problems surface as ErrPlatformUnavailable and
// never as a local execution path.
func TestPlatformUnavailableDoesNotFallback(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	_, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", IntentSpec{TargetKind: "kubernetes"})
	if err == nil || !strings.Contains(err.Error(), "PLATFORM_UNAVAILABLE") {
		t.Fatalf("503 not surfaced as unavailable: %v", err)
	}
	// Unreachable host also maps to unavailable.
	dead := &PlatformClient{BaseURL: "https://127.0.0.1:1", Token: "fixture-api-token", Client: &http.Client{Timeout: time.Second}}
	if _, err := dead.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", IntentSpec{TargetKind: "kubernetes"}); err == nil || !strings.Contains(err.Error(), "PLATFORM_UNAVAILABLE") {
		t.Fatalf("unreachable platform not surfaced as unavailable: %v", err)
	}
}

// Server refusals (validation/authorization) are rejected, never retried or
// downgraded into success.
func TestServerRejectionIsNotRetried(t *testing.T) {
	var attempts int
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	_, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", IntentSpec{TargetKind: "kubernetes"})
	if err == nil || !strings.Contains(err.Error(), "REQUEST_REJECTED") {
		t.Fatalf("403 not surfaced as rejection: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("rejected request was retried %d times", attempts)
	}
}

// Missing credential is refused before any request.
func TestMissingCredentialRefusedBeforeRequest(t *testing.T) {
	server, calls, _ := intentFixture(t)
	client := &PlatformClient{BaseURL: server.URL, Client: server.Client()}
	if _, err := client.SubmitIntent(context.Background(), IntentCreateKubernetesTarget, "idem", "corr-1", IntentSpec{TargetKind: "kubernetes"}); err != ErrNotAuthenticated {
		t.Fatalf("missing credential not refused: %v", err)
	}
	if *calls != 0 {
		t.Fatal("request sent without credential")
	}
}

// Only server-declared intent kinds are accepted.
func TestUnknownIntentKindRefused(t *testing.T) {
	client := &PlatformClient{BaseURL: "https://api.invalid", Token: "t"}
	if _, err := client.SubmitIntent(context.Background(), "DeleteEverything", "idem", "corr-1", IntentSpec{}); err == nil || !strings.Contains(err.Error(), "no server contract") {
		t.Fatalf("undeclared intent kind accepted: %v", err)
	}
}

// node add 意图: AddNodesRuntimeTarget carries sshSecretRef and the nodes list;
// a cross-tenant ssh reference is refused locally before any request.
func TestNodeAddIntentSubmitsWithSSHSecretRef(t *testing.T) {
	server, calls, seenScopes := intentFixture(t)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", TenantScope: "tenant-a", Client: server.Client()}
	spec := IntentSpec{
		TargetID:     "018f6c2a-4a64-7b58-9cc3-9f70462f36c2",
		TargetKind:   "KubernetesTarget",
		SSHSecretRef: &IntentSecretRef{Provider: "vault", Scope: "tenant-a", Name: "cluster-ssh", Version: "2"},
		Parameters:   map[string]any{"nodes": []any{map[string]any{"name": "worker-3", "role": "worker"}}},
	}
	result, err := client.SubmitIntent(context.Background(), IntentAddNodesRuntimeTarget, "node-add-42", "corr-1", spec)
	if err != nil {
		t.Fatalf("node add intent rejected: %v", err)
	}
	if result.OperationID != "op-1" {
		t.Fatalf("node add intent did not produce an operation: %+v", result)
	}
	if *calls != 1 || len(*seenScopes) != 1 || (*seenScopes)[0] != "tenant-a" {
		t.Fatalf("ssh reference not carried: calls=%d scopes=%v", *calls, *seenScopes)
	}
	cross := spec
	cross.SSHSecretRef = &IntentSecretRef{Provider: "vault", Scope: "tenant-b", Name: "cluster-ssh"}
	if _, err := client.SubmitIntent(context.Background(), IntentAddNodesRuntimeTarget, "node-add-43", "corr-1", cross); err == nil || !strings.Contains(err.Error(), "outside the authenticated tenant scope") {
		t.Fatalf("cross-tenant ssh reference accepted: %v", err)
	}
}

func TestGetOperationRequiresValidIDAndMapsAvailability(t *testing.T) {
	client := &PlatformClient{BaseURL: "https://api.invalid", Token: "t"}
	if _, err := client.GetOperation(context.Background(), ""); err == nil {
		t.Fatal("empty operation id accepted")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client = &PlatformClient{BaseURL: server.URL, Token: "t", Client: server.Client()}
	if _, err := client.GetOperation(context.Background(), "op-1"); err == nil || !strings.Contains(err.Error(), "PLATFORM_UNAVAILABLE") {
		t.Fatalf("operation 503 not mapped: %v", err)
	}
}

// cluster list 读取服务端权威列表: 认证、拒绝与不可达映射与 GetOperation 一致。
func TestListRuntimeTargetsIsServerAuthoritative(t *testing.T) {
	client := &PlatformClient{BaseURL: "https://api.invalid", Token: ""}
	if _, err := client.ListRuntimeTargets(context.Background()); err != ErrNotAuthenticated {
		t.Fatalf("missing credential must be refused locally: %v", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/resources/clusters" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatal("missing bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":2,"items":[{"clusterId":"t-1","displayName":"minimal","kind":"kubernetes","status":"RUNNING","runtimeVersion":"v1.36.5+k3s1","createdAt":"2026-10-12T00:00:00Z","acceptance":{"available":true}},{"clusterId":"t-2","displayName":"dr","kind":"kubernetes","status":"REGISTERING","runtimeVersion":"","createdAt":"2026-10-13T00:00:00Z","acceptance":{"available":false}}]}`))
	}))
	defer server.Close()
	client = &PlatformClient{BaseURL: server.URL, Token: "t", Client: server.Client()}
	targets, err := client.ListRuntimeTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].ID != "t-1" || !targets[0].IsActive || targets[1].Status != "REGISTERING" {
		t.Fatalf("unexpected target list: %+v", targets)
	}

	// 总数与行数不一致的服务端响应必须按不可读处理。
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":3,"items":[]}`))
	}))
	defer server.Close()
	client = &PlatformClient{BaseURL: server.URL, Token: "t", Client: server.Client()}
	if _, err := client.ListRuntimeTargets(context.Background()); err == nil {
		t.Fatal("inconsistent target list accepted")
	}

	// 5xx 映射为不可达，绝不本地回退。
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	client = &PlatformClient{BaseURL: server.URL, Token: "t", Client: server.Client()}
	if _, err := client.ListRuntimeTargets(context.Background()); err == nil || !strings.Contains(err.Error(), "PLATFORM_UNAVAILABLE") {
		t.Fatalf("target list 502 not mapped: %v", err)
	}
}
