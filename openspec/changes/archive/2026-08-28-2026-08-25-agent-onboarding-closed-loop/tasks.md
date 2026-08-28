# agent-onboarding-closed-loop — Tasks

## 1. 服务端令牌 profile

- [x] `pkg/iam/agent_tunnel_token.go`：`AgentTunnelIdentity`/`AgentTunnelTokenClaims`/`Config`、`Sign`、`Verify`、`validateAgentTunnelTokenConfig`、`validAgentTunnelIdentity`。
- [x] TTL 上限 `MaxAgentTunnelTokenTTL = 24h`；签发 `exp` 截断到秒与 claim 一致；校验 `exp-iat ≤ 24h` 且强制 `(tenant, cluster)` 绑定一致。
- [x] `pkg/iam/agent_tunnel_token_test.go`：往返、错作用域/篡改/畸形/通配/超 TTL 上限。

## 2. apiserver 接入与配置

- [x] `cmd/apiserver/internal/config/config.go`：新增 `PublicBaseURL`、`AgentImage`（`PUBLIC_BASE_URL`/`AGENT_IMAGE`）。
- [x] `cmd/apiserver/main.go`：构造 `AgentTunnelTokenSigner/Verifier`；`verifyToken` 先 agent-tunnel 后回退 access 服务身份。
- [x] `cmd/apiserver/internal/router/router.go`：`NewWithCapabilities` 增加 `agentTunnelSigner/publicBaseURL/agentImage` 参数；注册路由 `POST /api/v1/resources/clusters/{id}/agent-onboarding`（`readGate`）。
- [x] `cmd/apiserver/internal/middleware/authorization.go`：`apiserverRoutes` 新增 `cluster/ActionRead` 条目。

## 3. BFF 端点

- [x] `cmd/apiserver/internal/handler/agent_onboarding.go`：租户隔离目标校验（跨租户 404、非 kubernetes 403）、签发、`tunnelURL`（publicBaseURL 优先，否则按请求推导）、`agentOnboardingNamespace`、`renderAgentOnboardingManifest`（Namespace/SA/只读 ClusterRole+Binding/令牌 Secret/Deployment）、`writeJSONRaw` 裸载荷。
- [x] `agent_onboarding_test.go`：happy path（返回字段 + 令牌可校验绑定一致 + 清单含 TUNNEL_URL/TENANT/CLUSTER/SA）、跨租户 404、edge 403、未配置 503、tunnelURL 推导、命名空间清理。

## 4. 前端引导

- [x] `api/agentOnboardingApi.ts` + `index.ts` 注入 `setAgentOnboardingApiClient`。
- [x] `components/AgentOnboardingGuide.vue`：默认收起、展开请求、安装命令/清单复制、令牌过期、失败重试、重新生成轮换提示。
- [x] 向导 `ClusterRegisterWizard.vue`：import 且 kubernetes 且 Operation succeeded → `AgentOnboardingGuide(:cluster-id="opDetail.targetId")`。
- [x] 详情 `ClusterDetailLayout.vue`：`source` 透传；`showAgentOnboarding`（imported+kubernetes+/overview）概览页渲染（恢复路径）。
- [x] i18n `clusterMgmt.agentOnboarding.*` 中英。

## 5. 部署

- [x] apiserver chart：`values.agentOnboarding.{publicBaseURL,image}` + deployment env `AGENT_IMAGE`/`PUBLIC_BASE_URL`。
- [x] docker-compose apiserver：`AGENT_IMAGE` / `PUBLIC_BASE_URL`（可空默认）。

## 6. 验收

- [x] 后端：`go build`/`go vet`/`go test`（iam、apiserver handler/router/middleware）全绿。
- [x] 前端：vitest 29 tests（agentOnboardingApi / AgentOnboardingGuide / ClusterRegisterWizard / kubeconfig / clusterDetailApi）全绿；`vue-tsc --noEmit` 通过。
- [x] 端到端闭环（服务端链路自动化验收，替代 live-stack 手工项）：
  - `agent_onboarding_loop_test.go` 走通「签发 agent-tunnel 令牌 → 双路径验签（agent-tunnel 优先、access 回退）→ AgentClient 回连 → 注册 → 心跳 → 控制面代理请求往返」，并验证跨租户令牌被隧道拒绝（不注册）。
  - `agent_onboarding_test.go` 强化：manifest 以多文档 YAML 逐段解析，断言 Namespace/SA/ClusterRole/Binding/Secret/Deployment 齐全且可被 `kubectl apply` 解析。
  - `cmd/cluster-agent` 与 `cmd/tunnel-server` Dockerfile 修复（GOWORK=off + 完整依赖拷贝 + 代理屏蔽），`docker build` 实测产出 12MB scratch 镜像；补充各自 Makefile（build/test/lint/vet/docker，docker 目标以 `--network=host` + 仓库根上下文构建）。
  - 授权语义钉死：`middleware/authorization.go` 将 `kubeconfig:download` 与 `agent-onboarding` 固定为 `cluster/ActionRead`（对齐 design.md readGate 语义），测试矩阵 deny-execute 场景随之通过。
  - 全量回归：platform-api 全部内网包、apiserver 全部包（含 DSN 集成）、pkg/iam、pkg/tunnel 全绿；web 321 tests / 64 files + `vue-tsc --noEmit` 全绿。
  - 巡检副产物：`TestEveryProtectedRouteDeniesBeforeStoreWithoutPermission` 暴露 `handleCreateRuntimeIntentBatch` 缺失 `s.authorize`（越权批量删除风险）已补；前端移除 `ClusterStatus` 中已废弃的 `SUSPENDED` 比较并注入 clusterMonitoringApi 测试的 apiClient mock。
  - live-stack 手工项（导入 k8s → apply → agent 经隧道回连 → 列表收敛）仍可作为部署侧选做，服务端与镜像侧已闭环。
