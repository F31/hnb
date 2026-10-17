# 2.6 Agent 会话轮换/重连 + 2.7 撤销关闭持久连接

- Run IDs：`agent-session-rotation-20261017`（2.6）、
  `agent-session-revocation-20261017`（2.7）
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：IDENT-002、RTLOOP-001（2.6）、IDENT-001、UXLOOP-002（2.7）
- 日期：2026-10-17
- 类型：本地单元/竞态/契约 + 独立 PostgreSQL 16.15 集成 + 真实 websocket 关闭测试。
  **不是**真实云主机、跨 TTL/HA 演练或 5s/60s 传播认证。

## 1. 2.6 Agent 会话轮换

### 交付

| 产物 | 内容 |
|---|---|
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | `agent_sessions`：session ID、tenant/target、generation、有效期、撤销 |
| `pkg/iam/agent_tunnel_token.go` | `agent-tunnel/v1` 可选 `sessionId`/`generation`；`SignSession`、`VerifyClaims`、`ParseAgentSessionLookup` |
| `pkg/iam/agent_session_source.go` | `HTTPAgentSessionIssuer` + `AgentSessionTokenSource`：单次 exchange、50% TTL 轮换、临时故障有界、撤销永久停止 |
| `cmd/apiserver/internal/handler/agent_enrollment.go` | exchange 创建 session 并签发 5min token；`POST /api/v1/agent-sessions:renew` |
| `cmd/cluster-agent/main.go` + config | `AGENT_ENROLLMENT_SECRET_FILE`/`AGENT_EXCHANGE_URL`/公钥 manifest/issuer/audience；重连使用轮换 token |
| `pkg/tunnel` | `ErrSessionRevoked`、close reason 映射；重连读取新 token |

### 语义

- 首次 exchange 创建 generation=1；同一 tenant/target 的新 enrollment 递增 generation。
- renew 保持同一 session ID/generation，更新 `issued_at/expires_at`，签发新 jti（新 token）。
- 旧 token 在 overlap 内仍可验证，但 tunnel 连接在旧 token 到期时关闭，Agent 用新 token 重连。
- 未携带 `sessionId` 的 legacy token 继续按原路径验证；新路径不回落静态文件。
- observer generation 独立：旧观测 generation 仍由 `Projector.Accept` 返回 `ErrFenced`，
  会话轮换不重置/绕过观测代次。

## 2. 2.7 撤销关闭持久连接

### 交付

| 产物 | 内容 |
|---|---|
| `DELETE /api/v1/resources/clusters/{id}/agent-session` | 撤销 tenant/target 的全部活动 session，返回 `revoked` 数 |
| `pkg/tunnel` `TunnelServer.Revoke` | 仅关闭 tenant 与 target 都匹配的连接 |
| `AgentConnection.CloseRevoked` | close frame `agent session revoked` |
| Agent main | 映射为 `ErrSessionRevoked`，记录稳定诊断后退出，不再重连 |
| renew 端点 | 已撤销/过期/generation 不符统一 401 |

### 语义

- 撤销后：新 renew 失败；已建立的 tunnel 连接被关闭；其他 tenant/target 连接不受影响。
- 无活动 session 的撤销幂等（`revoked=0`）。
- 身份服务临时故障：source 保留旧 token 至原 `exp`，过期 fail closed；永久拒绝立即停止。
- 审计事件只含 tenant/target/session/enrollment 引用，不含 secret/credential。

## 3. 验证

环境：本地 Go 工具链；独立 PostgreSQL 16.15（Docker，`127.0.0.1:55432`），测试后容器已删除。

| 命令 | 结果 |
|---|---|
| `go test -race ./pkg/iam/... ./pkg/tunnel/... ./cmd/cluster-agent/...` | 通过 |
| `go test ./cmd/apiserver/internal/... ./cmd/operation-worker/...` | 通过 |
| `HNB_TEST_DATABASE_URL=... go test -tags integration ./pkg/iam/ -run 'TestServiceIdentityStoreAtomicConsumptionAndRevocation|TestAgentSessionStoreLifecycle'` | **PASS**（两次迁移、100 并发消费、enrollment 撤销、session 创建/递增 generation/轮换/跨租户/撤销幂等） |
| `go test ./pkg/tunnel/ -run TestTunnelRevokeClosesOnlyTheExactTenantConnection` | 通过（错误 tenant 不关闭；正确 tenant 返回 `ErrSessionRevoked`） |
| `go test ./pkg/iam/ -run 'TestAgentSessionTokenSource|TestHTTPAgentSessionIssuer|TestAgentTunnelSession'` | 通过（50% 轮换、撤销永久失败、exchange 拒绝、错误脱敏） |
| `go test ./cmd/apiserver/internal/handler/ -run TestAgentEnrollment` | 通过（create/exchange/renew/revoke sessions/重放/跨租户/跨目标） |
| `go build ./cmd/apiserver/... ./cmd/cluster-agent/... ./cmd/operation-worker/...` | 通过 |
| `npm run contracts:test` | **28/28** |
| 全仓契约门禁 | 通过：80 operations、**81 schemas**、compatibility=checked |
| `node scripts/validate-openspec.mjs` | 34/34；48 specs / 287 requirements / 476 scenarios |

## 4. 明确未完成

- 真实 >2 TTL、服务/Agent 重启、5s 新调用/60s 长连接传播测量：2.8。
- 身份服务故障注入、多连接/多租户压测、HA/离线：未做。
- 生产 deployment rollout、Agent 安装清单切换到 enrollment secret：未做（代码路径已可用）。
- observer source-reset/lease 与 Agent 会话 generation 的联合演练：未做；既有 projector
  generation fencing 测试仍有效。

## 5. 被测文件摘要

| 文件 | sha256 |
|---|---|
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | `4e76a04ef0427980b99b3e247828ee6bd78f36c249473ddf22f57cc0d629ee13` |
| `pkg/iam/agent_session_source.go` | `0ab6a4c8566dae1a8f7e78b7b14df3a119432180c16ec8bb459ad4f17bc27331` |
| `pkg/iam/agent_tunnel_token.go` | `64ad9254e9e8e234fcdc99da0e18e595eafb7f15cfb282f5b05a05f9f47707fe` |
| `cmd/apiserver/internal/handler/agent_enrollment.go` | `7a4a57d00271c3cb31c32219ae09e5d7eec62ba496df6972eed980778fe2fdb0` |
| `pkg/tunnel/client.go` | `7014435ff0ca96ca4e10414a396b7b159fe3995d34c560b45539d63ea2075f55` |
| `pkg/tunnel/server.go` | `df104cd3ee12c092804c094a46a0b7b2b17106ef22edd4e3b6911e1850627971` |
| `cmd/cluster-agent/main.go` | `6481555a6bfcf150eac41ac69084466074d3229d31780580f7b85f493f8ac55b` |
| `cmd/cluster-agent/internal/config/config.go` | `04e1a82de40b1845973438ab7ec4a5010d17fbf8d3e0e713a702d29c73ea797c` |
| `contracts/schema/identity/v1/agent-session.schema.json` | `849e4f9b1007abb65ff0108914ec0bd24b1cc62e6843a6b6ff7694dbb1a361ce` |
| `contracts/schema/identity/v1/agent-session-revoke.schema.json` | `7102c882b08089f1614ff1358d91f0c2ee61ac4492ef14da63932380b2a894da` |

工作树与 HEAD 仍不同；以上为最终被测试文件字节摘要。
