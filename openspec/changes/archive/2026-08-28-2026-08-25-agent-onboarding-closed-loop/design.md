## Context

已归档的 `cluster-management-full-closure` 已把「导入（ImportRuntimeTarget）→ 不可变 ExecutionPlan → Operation → 观测投影 → Read Model」跑通，并明确禁止浏览器直连 Provider/Agent/Kubernetes/NATS。但闭环的**最终环节**缺失：`runtime_targets` 仍在意图提交时创建为 `status='unknown'`，platform-api 的 `generateSteps` 对 `ImportRuntimeTarget`/`CreateKubernetesTarget` 返回 `nil`（等于只建行 + 等待观测），而没有任何机制把 `cluster-agent` 投放到目标集群以建立隧道与观测。结果：导入即「卡在未连接」。

KubeSphere 与 Rancher 的做法是接入的中心环节，可作对照：

- **KubeSphere**：接入成员集群时下发/复用成员集群的 Agent（`ks-installer` 成员组件），通过账号令牌建立连接后上报集群信息。
- **Rancher**：导入集群时给出 `kubectl apply`/`curl | kubectl apply` 的集群注册命令，命令内携带面向该集群的注册令牌，集群内注册 Agent 反向连回 Rancher。

两者的共同点是：**由控制面（服务端）在校权后为特定目标签发绑定令牌，并把令牌与 Agent 部署清单交给管理员在目标集群执行**。本 change 在 HNB 安全边界内复刻该模式：浏览器不直连目标集群，由 apiserver BFF 签发 `hnb.agent-tunnel/v1` 令牌并渲染清单。

## Goals / Non-Goals

### Goals

- 为导入纳管的 Kubernetes 目标，提供服务端签发的、绑定 `(tenant, cluster)` 的集群 Agent 开通令牌（长 TTL 覆盖部署窗口）。
- 提供 apiserver BFF 端点，在校权后渲染可 `kubectl apply` 的 cluster-agent 清单（命名空间、SA、只读 RBAC、令牌 Secret、Deployment）。
- 隧道入口同时接受 agent-tunnel profile 与既有短 TTL 服务身份令牌（先新后旧回退，保证兼容）。
- 向导与集群详情页展示可复用、默认收起的接入指引（复制安装命令/清单、过期展示、轮换提示、失败重试）。
- 仅当操作成功且为「导入 Kubernetes」时在向导内展示；详情页仅对「imported + kubernetes」展示（恢复路径）。

### Non-Goals

- 不实现观测上报（`OBSERVATION_INGEST_URL`/`OBSERVER_TOKEN_FILE`）的端到端引导——Agent 在未配置观测时仍可经隧道建立连接与心跳，观测仍由部署侧配置（与现状一致）。
- 不让浏览器直连 Kubernetes API / Provider / Agent / NATS。
- 不在清单中出现纳管凭据明文（kubeconfig）；不新增数据库或中间件。
- 不实现 Agent 云上密钥轮换服务、镜像仓库对接或 Agent 版本管理。

## Architecture

```text
                              Onboarding（闭环最终环节）
 Browser (向导内联进度 / 集群详情页)
   | 提交导入 → Operation → 轮询至 succeeded → targetId 就绪
   | POST /api/v1/resources/clusters/{id}/agent-onboarding  (ResourceCluster/ActionRead)
   v
 apiserver BFF agentOnboarding handler
   | 1) 按 tenant_id 查 runtime_targets（跨租户 404，仅 kubernetes 可下发）
   | 2) iam.AgentTunnelTokenSigner.Sign(tenant, cluster)  → hnb.agent-tunnel/v1 令牌（TTL≤24h）
   | 3) 渲染 kubectl apply 清单（Namespace/SA/只读 RBAC/Secret/Deployment）
   | 返回裸载荷 { clusterId, tenantId, tunnelUrl, token, tokenExpiry, namespace, manifest, installCommand }
   v
 管理员在【目标集群】执行 kubeectl apply（凭据为 Agent 自身在集群内的 SA）
   v
 cluster-agent -- WebSocket tunnel --> apiserver /tunnel
   | verifyToken 双路径：AgentTunnelTokenVerifier(agent-tunnel) → 回退 ServiceAuthenticator(access)
   v
 隧道连接建立，heartbeat/请求代理；观测上报（如配置）进入 JetStream 投影
```

## 令牌 profile 设计（pkg/iam/agent_tunnel_token.go）

```go
const (
    AgentTunnelProfileVersion = "hnb.agent-tunnel/v1"
    AgentTunnelTokenType      = "agent-tunnel+jwt"
    MaxAgentTunnelTokenTTL    = 24 * time.Hour
)

type AgentTunnelIdentity struct { TenantID, ClusterID string }

type AgentTunnelTokenClaims struct {
    ProfileVersion, Issuer, Audience, SubjectID, SubjectType string
    Identity      AgentTunnelIdentity
    IssuedAt, NotBefore, ExpiresAt, TokenID, KeyID           ...
    Algorithm     string
}
```

- **签发**（`Sign(ctx, tenantID, clusterID)`）：要求非通配租户与合法集群绑定；`exp = now + TTL`（截断到秒，与 claim 一致）；返回令牌与过期时间。
- **校验**（`Verify(ctx, token, tenantID, clusterID)`）：ES256 验签（key ring）、严格 profile/issuer/audience/`SubjectType=service`/key/alg/TID 校验、`exp>now && exp-iat ≤ 24h`；**强制 `Identity.TenantID==tenantID && Identity.ClusterID==clusterID`**，返回过期时间。
- 与 `observer_token.go` 同构：复用 `randomID`/`decodeStrictJSON`/`boundedClaim`/`signObserverToken` 风格实现，无新依赖。

## 隧道验签双路径（cmd/apiserver/main.go）

```go
agentTunnelVerifier, agentTunnelSigner := /* iam.NewAgentTunnelTokenVerifier/Signer(issuer, "hnb-apiserver-tunnel", TTL=24h, keySet) */
verifyToken := func(ctx, token, tenantID, clusterID string) (time.Time, error) {
    if exp, err := agentTunnelVerifier.Verify(ctx, token, tenantID, clusterID); err == nil {
        return exp, nil   // 新：接入指引签发的长 TTL 令牌
    }
    trusted, err := tunnelAuthenticator.Authenticate(ctx, token, ActionExecute, tenantID, "cluster", clusterID)
    return trusted.ExpiresAt, err   // 回退：既有手动签发/轮换流程
}
```

## BFF 端点（cmd/apiserver/internal/handler/agent_onboarding.go）

路由与鉴权：
- `coreMux.HandleFunc("POST /api/v1/resources/clusters/{id}/agent-onboarding", ...)`
- `apiserverRoutes`：`{Method: POST, Pattern: "/api/v1/resources/clusters/{id}/agent-onboarding", ResourceKind: cluster, ResourceIDParam: id, Action: read}`
- 能力门禁：`readGate`（`cluster.read`，fail-closed）。

处理流程：
1. 取受信上下文（`iam.TrustedContextFrom`），`tenantID` 确定租户。
2. `SELECT name, display_name, target_type FROM runtime_targets WHERE id=$1 AND tenant_id=$2`；无行 → 404（不泄露存在性）；非 `kubernetes` → 403。
3. `signer.Sign(tenantID, id)` → 令牌 + 过期时间。
4. `tunnelURL`：`PUBLIC_BASE_URL` 非空则升级为 `ws(s)://…/tunnel`；否则按请求 `X-Forwarded-Proto`/TLS/Host 推导。
5. 命名空间 = `hnb-agent-<sanitized-tenant>`（K8s 名称安全，退化短哈希）。
6. `renderAgentOnboardingManifest(...)`：多文档 YAML（含 `agent-token` Secret + Deployment env `TUNNEL_URL/AGENT_TOKEN_FILE/TENANT_ID/CLUSTER_ID/KUBE_TOKEN_FILE/KUBE_CA_FILE/RECONNECT_INTERVAL=10/OBSERVATION_INTERVAL=60s`）。
7. `writeJSONRaw` 返回裸载荷（与集群 BFF 一致）。

## 前端

- `api/agentOnboardingApi.ts`：`getAgentOnboarding(clusterId)` → `POST /api/v1/resources/clusters/{id}/agent-onboarding`；`AgentOnboardingResponse` 类型。
- `components/AgentOnboardingGuide.vue`：props `clusterId` / `clusterName`；默认收起（状态栏 + 主按钮）；展开后请求并渲染安装命令 + 完整清单，两个复制按钮（剪贴板成功则短暂「已复制」）、令牌过期、失败重试、「重新生成」轮换提示；父组件注入 ApiClient 后调用。
- 向导 `ClusterRegisterWizard.vue`：`onboardingReady = submission && mode==='import' && !isEdgeImport && opDetail.status==='succeeded' && targetId 非空`；成功后在进度视图下方渲染 `AgentOnboardingGuide(:cluster-id="opDetail.targetId")`。
- 详情 `ClusterDetailLayout.vue`：`getClusterDetail` 携带 `source`；`showAgentOnboarding = detail.clusterType==='kubernetes' && source!=='created' && /overview$`；仅在概览页渲染组件（恢复路径）。
- i18n：`clusterMgmt.agentOnboarding.*` 中英。

## 配置与部署

- apiserver 配置 `PUBLIC_BASE_URL`（`env PUBLIC_BASE_URL`）、`AGENT_IMAGE`（`env AGENT_IMAGE`，默认 `hnb/cluster-agent:latest`）。
- apiserver helm chart `values.agentOnboarding.{publicBaseURL,image}` → deployment env。
- docker-compose apiserver 增 `AGENT_IMAGE` / `PUBLIC_BASE_URL`（可空默认）。

## Open Questions

- Agent 观测上报（`OBSERVATION_INGEST_URL`/`OBSERVER_TOKEN_FILE`）是否需要在本 change 一并纳入引导？当前判定为否：观察者令牌为短 TTL（≤10min），无法固化进 24h 清单；观测仍由部署侧预配置，Agent 未配置时应能优雅降级（仅隧道+心跳）。此项可留作后续 change。
- `naming`：命名空间 `hnb-agent-<tenant>` 避免多租户同目标冲突；ClusterRole 名带命名空间前缀防碰撞。是否需要租户级共享 SA 而非每目标一份？当前每目标一份（Deployment 独立）。
