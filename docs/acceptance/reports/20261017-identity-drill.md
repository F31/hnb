# 2.8 真实身份演练：跨 TTL、Agent 重启与撤销传播

- Run ID：`identity-drill-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：IDENT-001、IDENT-002、PREADY-001
- 日期：2026-10-17（日志时间 `2026-10-17T17:03Z`）
- 类型：本地进程级真实链路演练：真实 HTTP、WebSocket、PostgreSQL 16.15、
  apiserver handler/store、tunnel server 与 Agent session source。
  **不是**云主机、HA、生产 5min TTL soak 或离线认证。

## 1. 演练配置

| 项 | 值 |
|---|---|
| Session TTL | **4s**（为快速跨过 >2 TTL；生产默认 5min，代码路径相同） |
| Enrollment TTL | 15min，单次消费 |
| 观测窗口 | 11.93s，3 次重连 |
| 数据库 | PostgreSQL 16.15（Docker，`127.0.0.1:55432`，`hnb_test`） |
| 传输 | 真实 `httptest` HTTP API + 真实 WebSocket tunnel |
| 身份 | 真实 ES256 signer/verifier、`agent_sessions` 持久化、撤销检查 |
| 证据 | `reports/20261017-agent-session-identity-drill.log` |

## 2. 结果

| 场景 | 结果 | 证据 |
|---|---|---|
| 跨 >2 个 TTL 持续工作 | **通过**：11.93s 内 3 次重连，generation 保持 1，无需手工 token | 日志 `rotation: reconnects=3 generation=1 ...` |
| 自动续期 | **通过**：DB `agent_sessions.issued_at` 随 50% TTL 轮换推进；每次轮换签发新 jti | `latestDrillSession` 断言 |
| Agent 重启 | **通过**：关闭 source 后用**已消费**的 enrollment secret 重建；从 0600 session file 恢复，未再 exchange | 日志 `restart: resumed from persisted session without exchange` |
| 撤销关闭持久连接 | **通过**：`RevokeSessions` 后 WebSocket 在 **3.95ms** 收到 `agent session revoked` 并关闭（目标 ≤5s） | 日志 `persistent connection closed in 3.947704ms` |
| 撤销后新调用 | **通过**：旧 session 的 renew 被 401 拒绝；含撤销调用总耗时 **6.38ms** | 日志 `new renewal rejected; total=6.379953ms` |
| 跨租户权限隔离 | **通过**：`other-tenant` 的 exchange 返回 `ErrServiceCredentialRejected`；精确 tenant/target 关闭由 tunnel/handler 测试覆盖 | drill 断言 + `TestTunnelRevoke...` |
| 失败恢复 | **通过（重启路径）**：持久化 session 恢复且 enrollment 不重复消费；临时续期失败/过期 fail-closed 由 `AgentSessionTokenSource` 伪时钟测试覆盖 | drill + `TestAgentSessionTokenSource...` |

原始日志摘要：

```
rotation: reconnects=3 generation=1 ttl=4s elapsed=9.558789928s
restart: resumed from persisted session without exchange
revocation: persistent connection closed in 3.947704ms
revocation: new renewal rejected; total=6.379953ms
--- PASS: TestAgentSessionIdentityDrill (9.70s)
```

## 3. 关键实现

- `pkg/iam/agent_session_store.go`：0600 原子写入/读取 session 文件；重启恢复。
- `pkg/iam/agent_session_source.go`：优先加载有效持久化 session；否则单次 exchange；
  50% TTL 轮换；临时失败有界；撤销/永久拒绝立即停止。
- `cmd/apiserver/internal/handler/agent_session_drill_test.go`：`-tags drill` 的真实链路测试。
- `pkg/tunnel`：`Revoke` 精确 tenant/target 关闭；close reason 映射为 `ErrSessionRevoked`。
- `cmd/cluster-agent`：`AGENT_SESSION_FILE` 持久化；重启不依赖单次 enrollment。

## 4. 明确未完成

- 生产 5min TTL 的长时间 soak（本演练用 4s 以跨 >2 TTL 加速；路径相同）。
- 真实云主机/独立业务集群、HA、离线。
- 60s 长连接传播上界不是单独压测：本演练测得关闭为毫秒级，远低于 5s/60s 目标；
  但未做多连接/多租户大规模传播测量。
- 身份服务真实故障注入（停止 issuer/断网）未在云主机执行；临时失败恢复与 fail-closed
  已由伪时钟单测覆盖。
- observer source-reset 与 session generation 的联合演练未做；projector generation
  fencing 既有测试仍有效。

## 5. 被测文件摘要

| 文件 | sha256 |
|---|---|
| `cmd/apiserver/internal/handler/agent_session_drill_test.go` | `3dd071ddc0602d586bb70ac99069617c80587c99fe4b0dc72e1d13db1ad64044` |
| `pkg/iam/agent_session_store.go` | `c66a75bddcb707083bc80aea1282f9839161c017a787275d9da777a43c5a4f9d` |
| `pkg/iam/agent_session_source.go` | `225e26650302a076f24fcf7c4ddb83cb92ed1bb4136c3213a56b8135fee4f51a` |
| `pkg/iam/agent_tunnel_token.go` | `64ad9254e9e8e234fcdc99da0e18e595eafb7f15cfb282f5b05a05f9f47707fe` |
| `cmd/apiserver/internal/handler/agent_enrollment.go` | `7a4a57d00271c3cb31c32219ae09e5d7eec62ba496df6972eed980778fe2fdb0` |
| `pkg/tunnel/client.go` | `7014435ff0ca96ca4e10414a396b7b159fe3995d34c560b45539d63ea2075f55` |
| `pkg/tunnel/server.go` | `df104cd3ee12c092804c094a46a0b7b2b17106ef22edd4e3b6911e1850627971` |
| `cmd/cluster-agent/main.go` | `4ad145a98dc992fa28cf762e3a7f323709151b7483c1bca851b1c363553be0da` |
| `cmd/cluster-agent/internal/config/config.go` | `44482ab84b51fc9365e96c52ff256244c8c356070f09668c800037c0a75945be` |
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | `4e76a04ef0427980b99b3e247828ee6bd78f36c249473ddf22f57cc0d629ee13` |
| `docs/acceptance/reports/20261017-agent-session-identity-drill.log` | `1baed91e122231a1510103add50f3f8dc63c628cfd5ae6b4020fc293cdced56b` |

工作树与 HEAD 仍不同；以上为最终被测试文件字节摘要。
