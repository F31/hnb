## Why

已归档的 `cluster-management-full-closure` change 鉴定了“接入集群（纳管 Kubernetes 目标）”的写路径与观测投影，但浏览器一端仍缺少闭环的**最终部署环节**：导入/纳管一个 Kubernetes 集群后，`runtime_targets` 只是落库为 `status='unknown'`，管理员并没有可执行的、平台签发的步骤去把 `cluster-agent` 部署到目标集群，导致隧道连接与观测永远不会建立，集群长期停留在 REGISTERING/未连接。而 KubeSphere（成员集群 Agent 下发）与 Rancher（集群注册 kubectl 命令）均把「向目标集群下发并运行 Agent」作为接入闭环的核心环节。

本 change 补齐该最终环节的服务端能力与前端呈现，形成「导入 → Operation 进度 → Agent 接入指引 → 目标集群部署 → 隧道回连 → 运行中」的完整闭环。

## What Changes

- Change ID: `agent-onboarding-closed-loop`。
- Tier: T1（服务端 BFF + Web L3 组件）；无数据库/中间件变更。
- 影响平面：`cmd/apiserver`（新 BFF 端点 + 隧道验签双路径 + 配置）、`pkg/iam`（新 agent-tunnel token profile）、`web/plugins/resource`（接入指引组件与两处集成）、部署清单（apiserver chart / compose）。
- 新增 **agent-tunnel token profile**（`hnb.agent-tunnel/v1`）：与 60s 上限的 access token 解耦，TTL 上限 24h，签发给 cluster-agent，仅绑定 `(tenantID, clusterID)`，`aud=hnb-apiserver-tunnel`；仅当校验器对目标作用域一致时通过。
- 新增 apiserver BFF 端点 `POST /api/v1/resources/clusters/{id}/agent-onboarding`（`ResourceCluster/ActionRead` 授权，`cluster.read` 能力门禁）：
  - 按 `tenant_id` 精确过滤目标（跨租户一律 404，不泄露存在性）；仅 `kubernetes` 目标可用（edge 走 CloudCore，create 由控制面自动装机）；
  - 服务端签发 agent-tunnel 令牌，并渲染「kubectl apply 即用」的 cluster-agent 清单（Namespace/ServiceAccount/只读 ClusterRole+Binding/令牌 Secret/Deployment）；
  - 返回裸载荷（与集群 BFF 其余端点一致，无 `code/message/data` 信封）。
- 隧道验签双路径（`cmd/apiserver/main.go verifyToken`）：先校验 agent-tunnel profile（长 TTL 覆盖 `kubectl apply` + 首次 Pod 启动窗口），失败回退到既有短 TTL access-token 服务身份路径，保留既有手动签发/轮换流程兼容性。
- 部署配置：`PUBLIC_BASE_URL`（对外地址，渲染 TUNNEL_URL；留空按请求 Host/协议推导）与 `AGENT_IMAGE`（清单引用的镜像），接入 apiserver helm chart 与 docker-compose。
- 前端：
  - 新增可复用 `AgentOnboardingGuide` L3 组件（收起/展开、安装命令与完整清单复制、令牌过期展示、重新生成轮换提示、失败重试）；
  - 向导内联进度视图：仅「导入 Kubernetes 且 Operation 成功」后展示接入指引（闭环最终环节），默认收起不立即请求；
  - 集群详情概览页：仅「imported + kubernetes」集群展示接入指引（恢复路径，便于重新下发 Agent）。
- 非目标：不在本 change 实现 Agent 观测上报的端到端引导（`OBSERVATION_INGEST_URL`/`OBSERVER_TOKEN_FILE` 仍由部署侧配置）；不实现 Agent 云上密钥轮换服务；不实现 Agent 版本管理/镜像仓库对接。

## Capabilities

### New Capabilities

- `cluster-agent-onboarding`（新增）：把 Kubernetes 目标接入闭环的最终环节标准化为「服务端签发绑定令牌 + 渲染可执行部署清单 + 前端指引引导」，依赖既有 `resource-cluster` / `runtime-target` 能力。

### Modified Capabilities

- `runtime-target`：为导入纳管的 KubernetesTarget 补上可执行的 Agent 下发路径，使 `runtime_targets` 从 `status='unknown'` 走通到隧道连接与观测。
- `platform-kernel`：新增一类与用户身份隔离的服务端签发的长 TTL 隧道令牌（agent-tunnel profile），并明确其签发即授权、校验绑定一致、断开/过期由隧道侧强制收敛的边界。
- `portal-experience`：将「接入集群」从提交即结束提升为含 Agent 下发引导的完整闭环体验。

## Impact

- 受影响代码：`pkg/iam/agent_tunnel_token.go(+test)`、`cmd/apiserver/internal/{config,handler,router,middleware}`、`cmd/apiserver/main.go`、`web/plugins/resource/src/{api,components,pages/cluster-management,locales,types}`、`deploy/charts/hnb/charts/apiserver`、`deploy/docker-compose/compose.yml`。
- 依赖 change：已归档 `2026-08-05-cluster-management-full-closure`、`2026-08-01-web-resource-cluster-management`。
- 迁移影响：无数据库迁移；无新增中间件；新增可配置环境变量（`PUBLIC_BASE_URL`、`AGENT_IMAGE`），未设置时行为退化为请求推导/默认镜像，向后兼容。
- 兼容与回滚：隧道验签先新后旧双路径，旧 access-token 隧道凭据不受影响；前端两处引入均默认收起/条件渲染，能力门禁可 fail-closed 关闭端点；回滚删除 BFF 路由与前端指引组件即可，不改动既有写路径与数据。
- 安全风险：令牌泄露、跨租户绑定、未授权下发。缓解：令牌仅绑定单一 `(tenant, cluster)`，签发即授权且必填非通配；校验端强制作用域一致并校验 `exp-iat ≤ 24h`；端点按目标租户精确过滤并走 `ResourceCluster/ActionRead` 授权；清单内不出现纳管凭据（Agent 在目标集群内用自身 ServiceAccount 观测）；令牌/清单仅经复制交管理员在目标集群执行，前端不持久化。
