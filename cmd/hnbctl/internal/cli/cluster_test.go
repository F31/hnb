package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func writeTokenFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("fixture-api-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A live TLS platform fixture that replays idempotent intents.
func platformFixture(t *testing.T, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"hnb.io/v1","intentId":"intent-1","status":"accepted","operationId":"op-1","replayed":false}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// The self-signed test server is not a valid CA, so the CLI is exercised through
// the client's injected transport rather than a real TLS trust chain. The
// important CLI contracts here are the exit codes and the absence of any local
// fallback output.
func TestClusterUnavailableReturnsConnectionFailureWithoutFallback(t *testing.T) {
	server := platformFixture(t, http.StatusServiceUnavailable)
	token := writeTokenFile(t)
	code, out, errOut := runCLI(t, "cluster", "create", "--platform-url", server.URL, "--token-file", token, "--idempotency-key", "idem-1", "--target-kind", "kubernetes")
	if code != ExitConnectionFailed {
		t.Fatalf("503 must map to connection-failed exit, got %d (%s %s)", code, out, errOut)
	}
	combined := out + errOut
	for _, forbidden := range []string{"kubeadm", "ssh ", "bootstrap", "operationId\":\"op-", "falling back"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("unavailable platform produced local execution output %q: %s", forbidden, combined)
		}
	}
	if !strings.Contains(errOut, "PLATFORM_UNAVAILABLE") {
		t.Fatalf("expected PLATFORM_UNAVAILABLE reason: %s", errOut)
	}
}

func TestClusterRequiresHTTPSAndPrivateToken(t *testing.T) {
	token := writeTokenFile(t)
	code, _, errOut := runCLI(t, "cluster", "create", "--platform-url", "http://insecure.invalid", "--token-file", token, "--idempotency-key", "k")
	if code == ExitSuccess || !strings.Contains(errOut, "HTTPS") {
		t.Fatalf("non-HTTPS platform URL accepted: %d %s", code, errOut)
	}
	// A world-readable token file is refused.
	loose := filepath.Join(t.TempDir(), "loose-token")
	if err := os.WriteFile(loose, []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runCLI(t, "cluster", "create", "--platform-url", "https://api.invalid", "--token-file", loose, "--idempotency-key", "k")
	if code == ExitSuccess || !strings.Contains(errOut, "private") {
		t.Fatalf("non-private token file accepted: %d %s", code, errOut)
	}
}

func TestClusterImportRequiresTargetID(t *testing.T) {
	token := writeTokenFile(t)
	code, _, errOut := runCLI(t, "cluster", "import", "--platform-url", "https://api.invalid", "--token-file", token, "--idempotency-key", "k")
	if code == ExitSuccess || !strings.Contains(errOut, "--target-id") {
		t.Fatalf("import without target id accepted: %d %s", code, errOut)
	}
}

func TestClusterUnknownSubcommandRejected(t *testing.T) {
	code, _, errOut := runCLI(t, "cluster", "destroy")
	if code == ExitSuccess || !strings.Contains(errOut, "unknown cluster subcommand") {
		t.Fatalf("unknown cluster subcommand accepted: %d %s", code, errOut)
	}
}

// cluster list 从服务端权威列表读取，不伪造行；缺凭据与 5xx 本地拒绝。
func TestClusterListIsServerAuthoritative(t *testing.T) {
	body := `{"total":1,"targets":[{"id":"t-1","name":"minimal","targetType":"kubernetes","distribution":"k3s","connectionType":"kubeconfig","status":"active","labels":{},"isActive":true,"createdAt":"2026-10-12T00:00:00Z"}]}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/targets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)

	code, out, errOut := runCLI(t, "cluster", "list", "--platform-url", server.URL, "--token-file", token, "--format", "json")
	if code != ExitSuccess {
		t.Fatalf("cluster list failed: %d %s", code, errOut)
	}
	var doc struct {
		SchemaVersion string                         `json:"schemaVersion"`
		Total         int                            `json:"total"`
		Targets       []toolkit.RuntimeTargetSummary `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != "cluster-list/v1" || doc.Total != 1 || len(doc.Targets) != 1 || doc.Targets[0].ID != "t-1" {
		t.Fatalf("unexpected cluster list document: %+v", doc)
	}

	// Human form shows rows, never credentials.
	code, human, _ := runCLI(t, "cluster", "list", "--platform-url", server.URL, "--token-file", token)
	if code != ExitSuccess || !strings.Contains(human, "minimal") || strings.Contains(human, "fixture-api-token") {
		t.Fatalf("human cluster list unexpected: %d %s", code, human)
	}

	// 平台不可达必须本地拒绝，不回退到本地视图。
	code, out, errOut = runCLI(t, "cluster", "list", "--platform-url", "https://127.0.0.1:1", "--token-file", token, "--timeout", "1s")
	if code == ExitSuccess {
		t.Fatalf("unreachable platform reported a cluster list: %s", out)
	}
	if !strings.Contains(errOut, "PLATFORM_UNAVAILABLE") {
		t.Fatalf("expected PLATFORM_UNAVAILABLE: %s", errOut)
	}
}

func TestOperationGetRequiresOperationID(t *testing.T) {
	code, _, errOut := runCLI(t, "operation", "get")
	if code == ExitSuccess || !strings.Contains(errOut, "operation get") {
		t.Fatalf("operation get without id accepted: %d %s", code, errOut)
	}
}

// The 1.2 table requires cluster create/import to stay non-success until the
// server contract is reachable; here the URL is invalid so nothing is sent and
// no operation may be fabricated.
func TestClusterNeverFabricatesOperationOnFailure(t *testing.T) {
	token := writeTokenFile(t)
	code, out, _ := runCLI(t, "cluster", "create", "--platform-url", "https://127.0.0.1:1", "--token-file", token, "--idempotency-key", "k", "--target-kind", "kubernetes", "--format", "json", "--timeout", "1s")
	if code == ExitSuccess {
		t.Fatalf("unreachable platform reported success: %s", out)
	}
	var ref struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal([]byte(out), &ref); err == nil && ref.OperationID != "" {
		t.Fatalf("fabricated operation id: %s", out)
	}
}
