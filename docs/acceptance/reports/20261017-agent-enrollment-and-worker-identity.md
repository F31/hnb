# 2.4 worker 按租户续期接线 + 2.5 Agent 单次入网交换

- Run IDs：`operation-worker-tenant-identity-20261017`（2.4）、
  `agent-enrollment-exchange-20261017`（2.5）
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：IDENT-001（2.4）、IDENT-002（2.5）
- 日期：2026-10-17
- 类型：本地单元/竞态/契约 + 独立 PostgreSQL 16.15 集成。**不是**真实云主机、
  real_e2e、HA、生产 rollout 或 5s/60s 撤销时延认证。

## 1. 2.4 worker→Provider 按租户续期

### 交付

| 产物 | 内容 |
|---|---|
| `cmd/operation-worker/internal/config/config.go` | `serviceIdentities[tenant] = {identityId,subjectId,tokenFile}`；全局 `SERVICE_IDENTITY_ISSUER_URL/ISSUER/KEY_MANIFEST_FILE`；UUID/租户/tokenFile 校验 |
| `pkg/iam/service_identity_binding.go` | `NewHTTPTenantTokenSource`：每租户绑定固定 scope，未声明租户直接拒绝 |
| `cmd/operation-worker/main.go` | 加载/轮询公钥；按 provider audience 建 verifier；为每个 tenant 建 `HTTPCredentialIssuer`；驱动按 `execution.TenantID` 取凭据 |
| `pkg/iam/tenant_token_source.go` | `Token()` 明确拒绝；必须使用 `TokenForTenant`，避免误用共享凭据 |

### 语义

- 新路径只在 provider 声明 `serviceIdentities` 时启用；失败不回落 `tokenFile`。
- 每个 tenant 的 renewal source 固定 `(subject, audience, runtimeProvider/providerID, execute, tenant)`。
- 未声明的 tenant 返回 `ErrServiceCredentialRejected`，不会借用其他 tenant 的缓存。
- BFF 继续每请求 delegation（actor 绑定），不改成共享 actor 会话；既有
  `DelegationSigner/Verifier` 测试仍覆盖 actor/scope 绑定。
- legacy `tokenFile` 仅用于未迁移 provider；生产 rollout 与真实跨 TTL 演练仍归 2.8。

## 2. 2.5 Agent 单次入网交换

### 交付

| 产物 | 内容 |
|---|---|
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | enrollment 表增加 `id`、`revoked_at`、`revoked_by_subject_id`；哈希唯一；消费/撤销互斥 |
| `pkg/iam/service_identity.go` | `RevokeEnrollmentCredential`；consume 增加 `revoked_at IS NULL` 与 `id` 返回 |
| `pkg/iam/agent_enrollment.go` | 256-bit opaque secret + SHA-256 哈希 |
| `pkg/iam/agent_tunnel_token.go` | `SignWithTTL`：5min 会话，受 config/24h profile 上限约束 |
| `cmd/apiserver/internal/handler/agent_enrollment.go` | create / exchange / revoke；target 仍存在且为 Kubernetes；审计不含 secret |
| `contracts/schema/identity/v1/agent-{enrollment-create,enrollment-exchange,session}.schema.json` | `enrollmentSecret`/`credential` 为 `writeOnly` |

### API

- `POST /api/v1/resources/clusters/{id}/agent-enrollments`：15min 单次 secret，返回一次。
- `POST /api/v1/agent-enrollments:exchange`：public；必须提交 `tenantId`/`targetId`；
  原子消费后签发 5min `hnb.agent-tunnel/v1` 会话。
- `DELETE /api/v1/resources/clusters/{id}/agent-enrollments/{enrollmentId}`：撤销未消费凭据。
- 重放、过期、已撤销、跨租户、跨目标、错误 target 类型全部拒绝；401 不区分原因。

## 3. 验证

环境：本地 Go 工具链；独立 PostgreSQL 16.15（Docker，`127.0.0.1:55432`，`hnb_test`），
测试后容器已删除。不是云主机或生产数据库。

| 命令 | 结果 |
|---|---|
| `go test ./pkg/iam/...` | 通过（含 `SignWithTTL`、secret、tenant binding、tenant token source） |
| `go test ./pkg/iam/ -run 'TestHTTPTenantTokenSource|TestTenantRenewingTokenSource'` | 通过 |
| `go test ./cmd/operation-worker/internal/config/` | 通过（per-tenant 配置、缺失全局配置、缺失 subject/tokenFile、通配租户） |
| `go test ./cmd/apiserver/internal/handler/ -run TestAgentEnrollment` | 通过（create/exchange/replay/expired/revoked/cross-tenant/cross-target/wrong type） |
| `go test ./cmd/apiserver/internal/middleware/` | 通过（审计正文抑制） |
| `HNB_TEST_DATABASE_URL=... go test -tags integration ./pkg/iam/ -run TestServiceIdentityStoreAtomicConsumptionAndRevocation` | **PASS**：两次迁移、100 并发消费恰好一次、版本冲突、跨租户、幂等撤销、过期、enrollment 撤销 |
| `go build ./cmd/apiserver/... ./cmd/operation-worker/...` | 通过 |
| `npm run contracts:test` | **28/28** |
| 全仓契约门禁 | 通过：80 operations、5 APIs、36 messages、**80 schemas**、compatibility=checked |
| `node scripts/validate-openspec.mjs` | 34/34；48 specs / 287 requirements / 476 scenarios |

## 4. 明确未完成

- Agent 二进制的初次 exchange、凭据文件写入、轮换/重连与 observer generation fencing：2.6。
- 长连接撤销、续期失败诊断与身份服务故障演练：2.7。
- 超过两个 TTL、含服务/Agent 重启的真实演练：2.8。
- 生产 deployment rollout、BFF→领域 API actor 保留的独立集成用例、HA/离线：未做。
- 5s/60s 撤销传播仍是目标；本批无撤销缓存与长连接失效接线。

## 5. 被测文件摘要

| 文件 | sha256 |
|---|---|
| `pkg/iam/service_identity_binding.go` | `c0e647994793ec85febe58901e7bdaff3357665e6ecf635e11cab2bfd0b4cbf7` |
| `pkg/iam/agent_enrollment.go` | `8a19933cc72e63cfa62c4e73c307a89dd43d6bd0765219910fcae38193c50158` |
| `pkg/iam/agent_tunnel_token.go` | `d9c7f459c4012ab62ea271f851606e3bce335d69790a7f329816d5c5170ca61d` |
| `cmd/operation-worker/main.go` | `86fe3be06c1280ef1796221bf782ac07a4267883e7c05e07ec64c8283778bb40` |
| `cmd/operation-worker/internal/config/config.go` | `4d8024a14cdce17ffe321bba99f3a2c0f738190e53a68a444d760e51a215bfff` |
| `cmd/apiserver/internal/handler/agent_enrollment.go` | `742498ed179d035815ab816e185ef7b1aae3f4034808b253bf0a7f80a0ae9fe6` |
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | `3917df81c84975c043879467861a4b73e2f363195d0f31e6db9b97723a5e811d` |
| `contracts/schema/identity/v1/agent-enrollment-create.schema.json` | `5c1a60eac098154bc00f7dcf4aa2f12b1ba1ada3fbdc0631c3d4c59612bf4a3c` |
| `contracts/schema/identity/v1/agent-enrollment-exchange.schema.json` | `45ba7c0cce73b0c3f288b0b44222b321bf7779c09b021eb79faef386fa1b8f62` |
| `contracts/schema/identity/v1/agent-session.schema.json` | `0cc1b11f4ebdb5ae1ade7d236309dd94e8290a096cdc7e3af2d6cc2b3a510876` |

工作树与 HEAD 仍不同；以上为最终被测试文件字节摘要。
