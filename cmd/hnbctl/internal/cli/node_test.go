package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturedNodeIntent is what the platform fixture saw for one node-add request.
type capturedNodeIntent struct {
	kind            string
	targetID        string
	targetKind      string
	expectedVersion int64
	nodes           []map[string]any
	sshRefName      string
}

// nodeAddFixture is a live TLS platform that records the node-add intent
// envelope and answers with an accepted Operation.
func nodeAddFixture(t *testing.T) (*httptest.Server, *capturedNodeIntent) {
	t.Helper()
	captured := &capturedNodeIntent{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var envelope struct {
			Kind string `json:"kind"`
			Spec struct {
				TargetID        string         `json:"targetId"`
				TargetKind      string         `json:"targetKind"`
				ExpectedVersion int64          `json:"expectedVersion"`
				Parameters      map[string]any `json:"parameters"`
				SSHSecretRef    *struct {
					Name string `json:"name"`
				} `json:"sshSecretRef"`
			} `json:"spec"`
		}
		if json.Unmarshal(body, &envelope) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		captured.kind = envelope.Kind
		captured.targetID = envelope.Spec.TargetID
		captured.targetKind = envelope.Spec.TargetKind
		captured.expectedVersion = envelope.Spec.ExpectedVersion
		for _, raw := range asList(envelope.Spec.Parameters["nodes"]) {
			if node, ok := raw.(map[string]any); ok {
				captured.nodes = append(captured.nodes, node)
			}
		}
		if envelope.Spec.SSHSecretRef != nil {
			captured.sshRefName = envelope.Spec.SSHSecretRef.Name
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"hnb.io/v1","intentId":"node-add-intent","status":"accepted","operationId":"op-1","replayed":false}`))
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

func writeNodeAddFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "nodes.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validNodeAddFile = "sshSecretRef:\n  provider: vault\n  scope: tenant-a\n  name: cluster-ssh\n  version: \"2\"\nnodes:\n  - name: worker-3\n    role: worker\n"

func TestNodeAddSubmitsAddNodesIntent(t *testing.T) {
	server, captured := nodeAddFixture(t)
	platformClientOverride = server.Client()
	t.Cleanup(func() { platformClientOverride = nil })
	token := writeTokenFile(t)
	nodesPath := writeNodeAddFile(t, t.TempDir(), validNodeAddFile)

	code, out, errOut := runCLI(t, "node", "add", "--target-id", "018f6c2a-4a64-7b58-9cc3-9f70462f36c2", "-f", nodesPath,
		"--platform-url", server.URL, "--token-file", token, "--idempotency-key", "node-add-42",
		"--correlation-id", "018f6c2a-4a64-7b58-9cc3-9f70462f36c1", "--expected-version", "3")
	if code != ExitSuccess {
		t.Fatalf("node add must succeed against a reachable platform, got %d (%s %s)", code, out, errOut)
	}
	if !strings.Contains(out, "intent=node-add-intent") || !strings.Contains(out, "operation=op-1") {
		t.Fatalf("unexpected node add output: %s", out)
	}
	if captured.kind != "AddNodesRuntimeTarget" {
		t.Fatalf("intent kind = %q", captured.kind)
	}
	if captured.expectedVersion != 3 {
		t.Fatalf("expectedVersion = %d, want 3", captured.expectedVersion)
	}
	if captured.targetID != "018f6c2a-4a64-7b58-9cc3-9f70462f36c2" || captured.targetKind != "KubernetesTarget" {
		t.Fatalf("intent target = %s/%s", captured.targetKind, captured.targetID)
	}
	if len(captured.nodes) != 1 || captured.nodes[0]["name"] != "worker-3" || captured.nodes[0]["role"] != "worker" {
		t.Fatalf("intent nodes = %+v", captured.nodes)
	}
	if captured.sshRefName != "cluster-ssh" {
		t.Fatalf("intent sshSecretRef = %q", captured.sshRefName)
	}
}

func TestNodeAddRejectsUnsupportedRoleLocally(t *testing.T) {
	dir := t.TempDir()
	nodesPath := writeNodeAddFile(t, dir, "sshSecretRef:\n  provider: vault\n  scope: tenant-a\n  name: cluster-ssh\nnodes:\n  - name: cp-1\n    role: control-plane\n")
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", nodesPath)
	if code != ExitUsage || !strings.Contains(errOut, "only worker nodes") {
		t.Fatalf("control-plane role must be refused locally: %d %s", code, errOut)
	}
}

func TestNodeAddRejectsDuplicateNodes(t *testing.T) {
	dir := t.TempDir()
	nodesPath := writeNodeAddFile(t, dir, "sshSecretRef:\n  provider: vault\n  scope: tenant-a\n  name: cluster-ssh\nnodes:\n  - name: worker-3\n    role: worker\n  - name: worker-3\n    role: worker\n")
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", nodesPath)
	if code != ExitUsage || !strings.Contains(errOut, "duplicate node") {
		t.Fatalf("duplicate node must be refused locally: %d %s", code, errOut)
	}
}

func TestNodeAddRejectsEmptyNodesList(t *testing.T) {
	dir := t.TempDir()
	nodesPath := writeNodeAddFile(t, dir, "sshSecretRef:\n  provider: vault\n  scope: tenant-a\n  name: cluster-ssh\nnodes: []\n")
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", nodesPath)
	if code != ExitUsage || !strings.Contains(errOut, "at least one node") {
		t.Fatalf("empty nodes list must be refused locally: %d %s", code, errOut)
	}
}

func TestNodeAddRejectsMissingNodesFile(t *testing.T) {
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", filepath.Join(t.TempDir(), "absent.yaml"))
	if code != ExitUsage || !strings.Contains(errOut, "read node add declaration") {
		t.Fatalf("absent nodes file must be refused locally: %d %s", code, errOut)
	}
}

func TestNodeAddRejectsInvalidDeclaration(t *testing.T) {
	for name, content := range map[string]string{
		"unknown root field":   validNodeAddFile + "providerId: arbitrary-provider\n",
		"unknown node field":   strings.Replace(validNodeAddFile, "    role: worker", "    role: worker\n    command: ignored", 1),
		"unknown secret field": strings.Replace(validNodeAddFile, "  provider: vault", "  provider: vault\n  password: must-not-be-echoed", 1),
		"extra document":       validNodeAddFile + "---\nnodes: []\n",
		"invalid name":         strings.Replace(validNodeAddFile, "worker-3", "Worker_3", 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := writeNodeAddFile(t, t.TempDir(), content)
			code, out, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", path)
			if code != ExitUsage || out != "" || strings.Contains(errOut, "must-not-be-echoed") {
				t.Fatalf("invalid declaration accepted or leaked: %d %s %s", code, out, errOut)
			}
		})
	}
}

func TestNodeAddRequiresExpectedVersion(t *testing.T) {
	path := writeNodeAddFile(t, t.TempDir(), validNodeAddFile)
	code, _, errOut := runCLI(t, "node", "add", "--target-id", "target-abc", "-f", path)
	if code != ExitUsage || !strings.Contains(errOut, "--expected-version") {
		t.Fatalf("missing target version accepted: %d %s", code, errOut)
	}
}
