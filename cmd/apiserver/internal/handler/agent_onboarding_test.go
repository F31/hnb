package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/F31/hnb/pkg/iam"
	"gopkg.in/yaml.v3"
)

type agentTunnelTestKeys struct{ key *ecdsa.PrivateKey }

func (k agentTunnelTestKeys) CurrentSigningKey(context.Context) (string, *ecdsa.PrivateKey, error) {
	return "agent-kid", k.key, nil
}

func (k agentTunnelTestKeys) VerificationKey(context.Context, string) (*ecdsa.PublicKey, error) {
	return &k.key.PublicKey, nil
}

func newAgentTunnelSignerForTest(t *testing.T) (*iam.AgentTunnelTokenSigner, *iam.AgentTunnelTokenVerifier, agentTunnelTestKeys) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := agentTunnelTestKeys{key: key}
	cfg := iam.AgentTunnelTokenConfig{
		Issuer: "https://issuer.example", Audience: "hnb-apiserver-tunnel", TTL: time.Hour,
	}
	signer, err := iam.NewAgentTunnelTokenSigner(cfg, keys)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := iam.NewAgentTunnelTokenVerifier(cfg, keys)
	if err != nil {
		t.Fatal(err)
	}
	return signer, verifier, keys
}

func agentOnboardingRequest(t *testing.T, h *AgentOnboardingHandler, tenantID, clusterID string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/resources/clusters/"+clusterID+"/agent-onboarding", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	r.SetPathValue("id", clusterID)
	r = r.WithContext(iam.WithTrustedContext(r.Context(), iam.TrustedContext{
		SubjectID: "subject-a", SubjectType: "user", MembershipID: "membership-a",
		TenantID: tenantID, PolicyVersion: "default:1", CorrelationID: "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
		ScopedPermissions: []iam.ScopedPermission{{TenantID: tenantID, ResourceKind: "cluster", Action: iam.ActionRead}},
	}))
	w := httptest.NewRecorder()
	h.AgentOnboarding(w, r)
	return w
}

func TestAgentOnboardingHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT rt\.name[\s\S]*WHERE rt\.id = \$1 AND rt\.tenant_id = \$2`).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c", "tenant-a").
		WillReturnRows(sqlmock.NewRows([]string{"name", "display_name", "target_type"}).
			AddRow("prod-cluster", "Prod Cluster", "kubernetes"))

	signer, verifier, _ := newAgentTunnelSignerForTest(t)
	h := NewAgentOnboardingHandler(db, signer, "https://hnb.example.com", "hnb/cluster-agent:1.0.0")

	w := agentOnboardingRequest(t, h, "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var payload struct {
		ClusterID      string `json:"clusterId"`
		TenantID       string `json:"tenantId"`
		TunnelURL      string `json:"tunnelUrl"`
		Token          string `json:"token"`
		TokenExpiry    string `json:"tokenExpiry"`
		Namespace      string `json:"namespace"`
		Manifest       string `json:"manifest"`
		InstallCommand string `json:"installCommand"`
	}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v (body=%s)", err, w.Body.String())
	}
	if payload.ClusterID != "515eba09-0a41-5b92-b972-69af1f0f655c" || payload.TenantID != "tenant-a" {
		t.Fatalf("binding = %+v", payload)
	}
	if payload.TunnelURL != "wss://hnb.example.com/tunnel" {
		t.Fatalf("tunnelUrl = %q", payload.TunnelURL)
	}
	if payload.Namespace != "hnb-agent-tenant-a" {
		t.Fatalf("namespace = %q", payload.Namespace)
	}
	if payload.Manifest == "" || !strings.Contains(payload.Manifest, "kind: Deployment") {
		t.Fatalf("manifest missing deployment: %q", payload.Manifest)
	}
	for _, want := range []string{
		"- name: TUNNEL_URL",
		`value: "wss://hnb.example.com/tunnel"`,
		`value: "tenant-a"`,
		`value: "515eba09-0a41-5b92-b972-69af1f0f655c"`,
		`"hnb/cluster-agent:1.0.0"`,
		`agent-token: "`,
		"KUBE_TOKEN_FILE",
		"serviceAccountName: hnb-cluster-agent",
	} {
		if !strings.Contains(payload.Manifest, want) {
			t.Fatalf("manifest missing %q\n%s", want, payload.Manifest)
		}
	}
	if !strings.HasPrefix(payload.InstallCommand, "kubectl apply -f - <<'HNB_AGENT_EOF'") {
		t.Fatalf("installCommand = %q", payload.InstallCommand)
	}

	// 清单必须是可被 `kubectl apply` 解析的多文档 YAML：逐个文档解析并断言
	// 期望的 kind 集合都出现（捕获拼接/引号导致的 YAML 语法回归）。
	docKinds := []string{}
	for _, doc := range strings.Split(payload.Manifest, "---") {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		// 去掉文档开头的纯注释行（清单头部说明），再去丢已经空掉的文档；
		// 只对剩余的实际 YAML 内容做解析。
		lines := strings.Split(doc, "\n")
		for len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "#") {
			lines = lines[1:]
		}
		body := strings.TrimSpace(strings.Join(lines, "\n"))
		if body == "" {
			continue
		}
		var meta struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal([]byte(body), &meta); err != nil {
			t.Fatalf("manifest document is not parseable YAML: %v\n%s", err, body)
		}
		if meta.Kind == "" {
			t.Fatalf("manifest document missing kind:\n%s", doc)
		}
		docKinds = append(docKinds, meta.Kind)
	}
	wantKinds := map[string]bool{
		"Namespace": true, "ServiceAccount": true, "ClusterRole": true,
		"ClusterRoleBinding": true, "Secret": true, "Deployment": true,
	}
	for _, kind := range docKinds {
		delete(wantKinds, kind)
	}
	if len(wantKinds) != 0 {
		t.Fatalf("manifest missing kinds %v (got %v)", wantKinds, docKinds)
	}

	// 令牌可被隧道侧校验端接受，且绑定一致
	expiry, err := verifier.Verify(context.Background(), payload.Token, "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !expiry.After(time.Now()) {
		t.Fatalf("expiry %v not in future", expiry)
	}
	if _, err := verifier.Verify(context.Background(), payload.Token, "tenant-b", "515eba09-0a41-5b92-b972-69af1f0f655c"); err == nil {
		t.Fatal("token must not verify for another tenant")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentOnboardingCrossTenantNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT rt\.name`).
		WithArgs("515eba09-0a41-5b92-b972-69af1f0f655c", "tenant-b").
		WillReturnError(sql.ErrNoRows)

	signer, _, _ := newAgentTunnelSignerForTest(t)
	h := NewAgentOnboardingHandler(db, signer, "", "hnb/cluster-agent:1.0.0")

	w := agentOnboardingRequest(t, h, "tenant-b", "515eba09-0a41-5b92-b972-69af1f0f655c", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (跨租户不泄露存在性)", w.Code)
	}
}

func TestAgentOnboardingEdgeTargetForbidden(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT rt\.name`).
		WillReturnRows(sqlmock.NewRows([]string{"name", "display_name", "target_type"}).
			AddRow("edge-a", "Edge A", "edge_runtime"))

	signer, _, _ := newAgentTunnelSignerForTest(t)
	h := NewAgentOnboardingHandler(db, signer, "", "hnb/cluster-agent:1.0.0")

	w := agentOnboardingRequest(t, h, "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for non-kubernetes target", w.Code)
	}
}

func TestAgentOnboardingNotConfigured(t *testing.T) {
	h := NewAgentOnboardingHandler(nil, nil, "", "")
	w := agentOnboardingRequest(t, h, "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when signer not configured", w.Code)
	}
}

func TestAgentOnboardingTunnelURLDerivation(t *testing.T) {
	signer, _, _ := newAgentTunnelSignerForTest(t)

	// 无 PUBLIC_BASE_URL：按 X-Forwarded-Proto + Host 推导
	h := NewAgentOnboardingHandler(nil, signer, "", "hnb/cluster-agent:1.0.0")
	httpsReq := httptest.NewRequest(http.MethodGet, "https://hnb.example.com/", nil)
	httpsReq.Header.Set("X-Forwarded-Proto", "https")
	if got := h.tunnelURL(httpsReq); got != "wss://hnb.example.com/tunnel" {
		t.Fatalf("derived tunnelUrl = %q", got)
	}
	// http 明文推导为 ws
	if got := h.tunnelURL(httptest.NewRequest(http.MethodGet, "http://10.0.0.5:8080/", nil)); got != "ws://10.0.0.5:8080/tunnel" {
		t.Fatalf("derived tunnelUrl = %q", got)
	}
	// PUBLIC_BASE_URL 显式覆盖（http → ws）
	h2 := NewAgentOnboardingHandler(nil, signer, "http://internal.hnb.svc:8080/", "hnb/cluster-agent:1.0.0")
	if got := h2.tunnelURL(httptest.NewRequest(http.MethodGet, "http://hnb.example.com/", nil)); got != "ws://internal.hnb.svc:8080/tunnel" {
		t.Fatalf("override tunnelUrl = %q", got)
	}
}

func TestAgentOnboardingNamespaceSanitize(t *testing.T) {
	if got := agentOnboardingNamespace("tenant-a"); got != "hnb-agent-tenant-a" {
		t.Fatalf("namespace = %q", got)
	}
	if got := agentOnboardingNamespace("Tenant/Dept_1"); got != "hnb-agent-tenant-dept-1" {
		t.Fatalf("namespace = %q", got)
	}
	// 纯非法字符退化为哈希，保证非空且稳定
	first := agentOnboardingNamespace("///")
	second := agentOnboardingNamespace("///")
	if first != second || !strings.HasPrefix(first, "hnb-agent-") {
		t.Fatalf("fallback namespace = %q / %q", first, second)
	}
}
