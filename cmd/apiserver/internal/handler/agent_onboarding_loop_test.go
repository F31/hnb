package handler

// 端到端验收（闭环最终环节，对齐 tasks.md §6）：
//
//	BFF 签发 agent-tunnel 令牌 → 目标集群 apply → cluster-agent（集群内）经
//	隧道回连：连接注册 → 心跳 → 控制面经隧道代理请求并拿到 Agent 的响应。
//
// 本测试在同一进程内把真实链路串起来：用与 cmd/apiserver/main.go 相同的
// 双路径验签逻辑（agent-tunnel 优先，回退 access 服务身份）构造 TunnelServer，
// 再用带 agent-tunnel 令牌的 AgentClient 连接，验证注册/心跳/代理往返，
// 证明「导入 → apply 引导 → Agent 回连 → 隧道可用」闭环的服务端链路成立。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/iam"
	"github.com/F31/hnb/pkg/tunnel"
)

// tunnelVerifyToken mirrors cmd/apiserver/main.go: 先试 agent-tunnel（长 TTL，
// 覆盖 kubectl apply + 首次启动窗口），失败回退短 TTL access 服务身份。
func tunnelVerifyToken(agent *iam.AgentTunnelTokenVerifier, access *iam.ServiceAuthenticator) tunnel.AuthTokenVerifier {
	return func(ctx context.Context, token, tenantID, clusterID string) (time.Time, error) {
		if expiry, err := agent.Verify(ctx, token, tenantID, clusterID); err == nil {
			return expiry, nil
		}
		trusted, err := access.Authenticate(ctx, token, iam.ActionExecute, tenantID, "cluster", clusterID)
		return trusted.ExpiresAt, err
	}
}

func TestAgentOnboardingTunnelClosedLoop(t *testing.T) {
	const (
		tenantID  = "tenant-loop"
		clusterID = "515eba09-0a41-5b92-b972-69af1f0f655c"
	)
	signer, agentVerifier, keys := newAgentTunnelSignerForTest(t)

	// access 回退路径的验签器与 main.go 同构（同一 audience）。
	accessVerifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "hnb-apiserver-tunnel", AccessTTL: iam.MaxAccessTokenTTL,
	}, keys)
	if err != nil {
		t.Fatal(err)
	}
	accessAuth, err := iam.NewServiceAuthenticator(accessVerifier)
	if err != nil {
		t.Fatal(err)
	}

	ts := tunnel.NewTunnelServer(tunnelVerifyToken(agentVerifier, accessAuth))
	server := httptest.NewServer(ts)
	defer server.Close()
	tunnelURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/tunnel"

	// BFF 签发绑定 (tenant, cluster) 的 onboarding 令牌，写入 0600 令牌文件
	// （Agent 用 iam.FileTokenSource 读取，与 cluster-agent 进程一致）。
	token, expiry, err := signer.Sign(context.Background(), tenantID, clusterID)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tokenFile := t.TempDir() + "/agent-token"
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}

	// Agent 进程等价物：FileTokenSource + AgentClient。
	client := tunnel.NewAgentClient(tunnelURL, iam.FileTokenSource{Path: tokenFile}, tenantID, clusterID)
	if err := client.Connect(); err != nil {
		t.Fatalf("agent connect: %v", err)
	}
	defer client.Close()

	// 注册成功 → 控制面能看到该 Agent。
	if count := ts.Registry().Count(); count != 1 {
		t.Fatalf("registered agents = %d, want 1", count)
	}
	if _, ok := ts.Registry().Get(clusterID); !ok {
		t.Fatalf("agent %s not present in registry", clusterID)
	}

	// 心跳往返：Agent 发送，控制面包ack。
	if err := client.SendHeartbeat(tunnel.HeartbeatPayload{ClusterID: clusterID, NodeCount: 3}); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read heartbeat ack: %v", err)
	}
	if msg.Type != tunnel.MsgHeartbeat {
		t.Fatalf("heartbeat reply type = %s, want heartbeat", msg.Type)
	}

	// 控制面经隧道向 Agent 发起代理请求并收到响应（观测/命令往返的传输底座）。
	requestHandled := make(chan error, 1)
	go func() {
		reqMsg, err := client.ReadMessage()
		if err != nil {
			requestHandled <- err
			return
		}
		var payload tunnel.RequestPayload
		if err := json.Unmarshal(reqMsg.Payload, &payload); err != nil {
			requestHandled <- err
			return
		}
		body := []byte(`{"items":[]}`)
		requestHandled <- client.SendResponse(payload.RequestID, http.StatusOK,
			map[string]string{"Content-Type": "application/json"}, body)
	}()

	resp, err := ts.ProxyRequest(clusterID, &tunnel.RequestPayload{
		RequestID: "list-nodes", Method: http.MethodGet, Path: "api/v1/nodes",
	})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if err := <-requestHandled; err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(resp.Body) != `{"items":[]}` {
		t.Fatalf("proxied response = status %d body %q", resp.StatusCode, resp.Body)
	}

	// 令牌过期时间落在签发 TTL 窗口内。
	if !expiry.After(time.Now()) || expiry.Sub(time.Now()) > time.Hour+time.Second {
		t.Fatalf("unexpected token expiry: %v", expiry)
	}
}

// TestAgentOnboardingTunnelClosedLoopCrossTenant 验证隧道拒绝错绑定的令牌
// （对端必须绑定同一 (tenant, cluster)，再审慎丢弃连接，不泄露存在性）。
func TestAgentOnboardingTunnelClosedLoopCrossTenant(t *testing.T) {
	signer, agentVerifier, keys := newAgentTunnelSignerForTest(t)
	accessVerifier, err := iam.NewTokenVerifier(iam.TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: "hnb-apiserver-tunnel", AccessTTL: iam.MaxAccessTokenTTL,
	}, keys)
	if err != nil {
		t.Fatal(err)
	}
	accessAuth, err := iam.NewServiceAuthenticator(accessVerifier)
	if err != nil {
		t.Fatal(err)
	}

	ts := tunnel.NewTunnelServer(tunnelVerifyToken(agentVerifier, accessAuth))
	server := httptest.NewServer(ts)
	defer server.Close()

	// 一个租户的令牌，尝试以另一个租户的身份回连 → 必须被拒。
	token, _, err := signer.Sign(context.Background(), "tenant-a", "515eba09-0a41-5b92-b972-69af1f0f655c")
	if err != nil {
		t.Fatal(err)
	}
	client := tunnel.NewAgentClient("ws"+strings.TrimPrefix(server.URL, "http")+"/tunnel",
		iam.FileTokenSource{Path: writeTokenFile(t, token)}, "tenant-b", "515eba09-0a41-5b92-b972-69af1f0f655c")
	if err := client.Connect(); err == nil {
		t.Fatal("cross-tenant agent must not connect")
	}
	if count := ts.Registry().Count(); count != 0 {
		t.Fatalf("cross-tenant sockets must not register, got %d", count)
	}
}

func writeTokenFile(t *testing.T, token string) string {
	t.Helper()
	path := t.TempDir() + "/agent-token"
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
