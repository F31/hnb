package main

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/cluster-agent/internal/config"
	"github.com/F31/hnb/pkg/tunnel"
)

func TestNewKubeHTTPClientTrustsConfiguredCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	caFile := filepath.Join(t.TempDir(), "kube-ca.crt")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := newKubeHTTPClient(caFile, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestProxyToKubeAPIForwardsQueryAndPlainTextAccept(t *testing.T) {
	var gotURI, gotAccept, gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		gotAccept = r.Header.Get("Accept")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "line one\nline two\n")
	}))
	defer server.Close()

	status, headers, body := proxyToKubeAPI(tunnel.RequestPayload{
		Method:   http.MethodGet,
		Path:     "api/v1/namespaces/default/pods/p1/log",
		RawQuery: "container=api&tailLines=200&timestamps=true",
		Headers:  map[string]string{"Accept": "text/plain"},
	}, &config.Config{KubeAPI: server.URL, KubeToken: "service-account-token"})

	if status != http.StatusOK || string(body) != "line one\nline two\n" {
		t.Fatalf("status/body = %d %q", status, body)
	}
	if gotURI != "/api/v1/namespaces/default/pods/p1/log?container=api&tailLines=200&timestamps=true" {
		t.Fatalf("request URI = %q", gotURI)
	}
	if gotAccept != "text/plain" || gotAuthorization != "Bearer service-account-token" {
		t.Fatalf("headers accept=%q authorization=%q response=%q", gotAccept, gotAuthorization, headers)
	}
}

func TestKubeProxyResourceMethodAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		allowed bool
	}{
		{name: "storage class discovery", method: http.MethodGet, path: "/apis/storage.k8s.io/v1/storageclasses", allowed: true},
		{name: "namespaced CSI capacity discovery", method: http.MethodGet, path: "/apis/storage.k8s.io/v1/namespaces/default/csistoragecapacities", allowed: true},
		{name: "optional snapshot discovery", method: http.MethodGet, path: "/apis/snapshot.storage.k8s.io/v1/namespaces/default/volumesnapshots", allowed: true},
		{name: "console pod logs", method: http.MethodGet, path: "/api/v1/namespaces/default/pods/api-0/log", allowed: true},
		{name: "console service create", method: http.MethodPost, path: "/api/v1/namespaces/default/services", allowed: true},
		{name: "console workload restart", method: http.MethodPatch, path: "/apis/apps/v1/namespaces/default/deployments/api", allowed: true},
		{name: "PV create", method: http.MethodPost, path: "/api/v1/persistentvolumes", allowed: false},
		{name: "PVC patch", method: http.MethodPatch, path: "/api/v1/namespaces/default/persistentvolumeclaims/data", allowed: false},
		{name: "storage class delete", method: http.MethodDelete, path: "/apis/storage.k8s.io/v1/storageclasses/fast", allowed: false},
		{name: "volume attachment mutation", method: http.MethodPut, path: "/apis/storage.k8s.io/v1/volumeattachments/attachment-a", allowed: false},
		{name: "arbitrary custom resource", method: http.MethodDelete, path: "/apis/example.io/v1/namespaces/default/widgets/a", allowed: false},
		{name: "unneeded workload create", method: http.MethodPost, path: "/apis/apps/v1/namespaces/default/deployments", allowed: false},
		{name: "unknown subresource", method: http.MethodGet, path: "/api/v1/namespaces/default/pods/api-0/exec", allowed: false},
		{name: "encoded traversal", method: http.MethodGet, path: "/api/v1/namespaces/default/pods/%2e%2e/secrets", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubeProxyAllowed(tt.method, tt.path); got != tt.allowed {
				t.Fatalf("kubeProxyAllowed(%q, %q) = %v, want %v", tt.method, tt.path, got, tt.allowed)
			}
		})
	}
}

func TestProxyToKubeAPIRejectsStorageMutationBeforeForwarding(t *testing.T) {
	forwarded := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		forwarded = true
	}))
	defer server.Close()

	status, _, body := proxyToKubeAPI(tunnel.RequestPayload{
		Method: http.MethodPatch,
		Path:   "/api/v1/persistentvolumes/pv-a",
		Body:   []byte(`{"spec":{"claimRef":null}}`),
	}, &config.Config{KubeAPI: server.URL})

	if status != http.StatusForbidden || forwarded {
		t.Fatalf("status=%d forwarded=%v body=%q", status, forwarded, body)
	}
}

func TestClusterAgentRBACSeparatesObserverAndExecutor(t *testing.T) {
	chartDir := filepath.Join("..", "..", "deploy", "charts", "hnb", "charts", "cluster-agent", "templates")
	rbac, err := os.ReadFile(filepath.Join(chartDir, "rbac.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(rbac)
	for _, expected := range []string{"kind: ServiceAccount", "-observer", "-executor", "resources: [\"storageclasses\", \"csidrivers\", \"csinodes\", \"csistoragecapacities\", \"volumeattachments\"]"} {
		if !strings.Contains(manifest, expected) {
			t.Errorf("RBAC template does not contain %q", expected)
		}
	}
	clusterRoleDocument := "apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:"
	if strings.Count(manifest, clusterRoleDocument) != 2 {
		t.Fatalf("RBAC template must define separate observer and executor ClusterRoles")
	}
	executorStart := strings.Index(manifest, "name: {{ .Release.Name }}-{{ .Chart.Name }}-executor\nrules:")
	if executorStart < 0 {
		t.Fatal("executor ClusterRole section not found")
	}
	executorEnd := strings.Index(manifest[executorStart:], "\n---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding")
	if executorEnd < 0 {
		t.Fatal("executor ClusterRole section not found")
	}
	executorRules := manifest[executorStart : executorStart+executorEnd]
	for _, storageResource := range []string{"persistentvolumes", "persistentvolumeclaims", "storageclasses", "volumeattachments", "volumesnapshots"} {
		if strings.Contains(executorRules, storageResource) {
			t.Errorf("executor mutation role must not grant storage resource %q", storageResource)
		}
	}

	deployment, err := os.ReadFile(filepath.Join(chartDir, "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(deployment), ".Values.serviceAccount.observerName") {
		t.Fatal("cluster-agent deployment must use the observer ServiceAccount")
	}
}
