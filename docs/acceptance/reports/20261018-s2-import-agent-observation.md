# S2 独立目标 import 与 Agent 观测实机结果

- run_id: `s2-import-agent-20261018`
- commit: `533d8ae41b386d9ae738528f73641720d40f997f` + 本批工作树增量
- scope: 三主机 lab 的独立业务集群；正式 API import、幂等重放、Agent enrollment/tunnel、
  node/capability 观测。**3.3 功能链路已接通，但因一次 lab 游标修复仍保留 partial，不宣称
  3.3–3.5 全部完成。**

## 目标与结果

| 项 | 值 |
|---|---|
| tenant | `default` |
| target | `ef14ba93-f2df-523f-9231-cd2fad333cc3` |
| displayName | `hnb-target-live3` |
| import intent | `914b4bc5-0594-4bac-940f-8f8408a3045e` |
| operation | `600fabde-2c0e-4ca3-8cb0-91208cf85168` |
| import 结果 | `operationCommitted` → step `succeeded` |
| 幂等重放 | 同 intent/operation，`replayed=true` |
| Agent session generation | 2 |
| observer generation | 2 |
| observation revision | 6 |
| nodeCount | 2 |
| runtimeVersion | `v1.36.5+k3s1` |
| cpuTotal / memoryTotal | `4C` / `8 GiB` |
| freshness / available | `FRESH` / `true` |

最终 read model：

```text
status=RUNNING lifecycle=ACTIVE health=HEALTHY connectivity=CONNECTED
freshness=FRESH observedAt=2026-10-18T03:06:16Z nodeCount=2
acceptance=available:true observationState:observed observationGeneration:2
```

## 执行的正式链路

1. `POST /api/v1/secrets:register` 注册 target kubeconfig，返回 `local-aes` SecretReference。
2. `POST /api/v1/runtime-intents` 提交 `ImportRuntimeTarget`，operation-worker 调用
   lifecycle provider，provider 连接独立业务集群并注册 target。
3. 幂等重放同一 idempotency key/correlation，返回相同 intent/operation，`replayed=true`。
4. `POST /api/v1/resources/clusters/{id}/agent-enrollments` 创建一次性 enrollment。
5. 业务集群部署 cluster-agent；Agent 通过 `agent-enrollments:exchange` 换取 session，
   通过 tunnel-server 持久连接回连，并把 observer token 写到私有文件。
6. Agent 每 30s 上报 Full/Delta；投影得到 2 节点、Kubernetes 版本、CPU/内存与 FRESH。
7. 平台重启后撤销旧 session、创建新 enrollment，session/observer generation 提升到 2；
   旧 generation 的观测被 fencing。

## 本轮修复的真实缺陷

1. **租户中间件阻断续期/入网**：`service-credentials:renew`、`agent-enrollments:exchange`、
   `agent-sessions:renew` 在认证中间件放行，但 tenant 中间件仍要求 tenant context。
   已加入公共 bypass 并补测试；服务凭据续期恢复为 200。
2. **tunnel-server 不认 Agent session token**：只使用 legacy 服务令牌校验，导致 WebSocket 401。
   已接入 `AgentTunnelTokenVerifier` + agent session store，校验 session 未撤销/未过期且
   generation 一致；旧静态令牌仍保留兼容路径。
3. **freshness 不更新**：projector 只在下发时写 target，未写 `freshness_state`，导致
   observed 目标仍 `UNKNOWN`、`available=false`。已按 observedAt + staleThreshold 写入，
   并增加周期性 sweeper；断开后状态保留但会转 STALE。
4. **capability 规范化列为空**：`capability_snapshots` 只写 `snapshot_json`，未写
   `kube_version/cpu_cores/memory_mb`，导致 `runtimeVersion` 为空。已从 capability content
   填充并更新冲突行。
5. **Provider pinned digest 不匹配**：worker 初始配置为镜像 digest，而 plan 使用 manifest
   identity digest；绑定实际 pinned digest 后 import 成功。

## Lab 恢复与保留的缺口

- Provider manifest 只有 `import` action，conformance 使用独立 Ed25519 签名报告，
  environment 为 `three-host-lab/provider-unit-integration`，有效期 **1 小时**；
  这不是生产 conformance。
- 服务 subject 通过开发库显式 provision（`issuer=lab`）；正式服务 subject API 仍缺失。
- 平台重启后 Agent 的 observer generation 从 1 升到 2；规范要求 source-reset，但 Agent
  尚未接该调用。为继续验证，执行了一次明确的 lab 游标修复
  （generation=2/revision=4）。这属于恢复性操作，**不是正式 source-reset 验收**。
- Agent session/observer token 目前写在 emptyDir；Agent 的 observation sequence 也未见
  跨进程持久化。进程/ Pod 重建后的 sequence 连续性与 source-reset 仍需要补齐。
- 三次失败重试产生的 REGISTERING 测试 target 已作为 lab 清理从数据库删除，仅保留成功目标；
  该清理不用于生成可用性。
- 因此 3.3 标为 functional/partial，3.4/3.5 仍未完成；没有把 lab 游标修复当作产品能力。

## 证据

- 本报告对应 JSON：`20261018-s2-import-agent-observation.json`
- Conformance 报告/测试：`/tmp/opencode/conformance/`（本机临时证据），
  signed report `conformance-signed.json`
- Provider manifest identity：`sha256:124e358d124042ae546f6e3fde9639c32bedc4a0b91af1603b19a66b07617620`
- Provider image：`sha256:121a8efde0c0d687fa47a613f402ec309c50fd0ec5980df7e5579cdae7604ff9`
- Agent image：`sha256:eba5f4197da7fbcc5d1f854ddc195d1a56b4fa65a2e8ea48d250c768e8859383`
- 相关代码：`cmd/tunnel-server/main.go`、`cmd/platform-api/internal/observer/projector.go`、
  `cmd/platform-api/internal/observer/freshness_sweeper.go`、
  `cmd/apiserver/internal/router/router.go`、`cmd/cluster-agent/main.go`
