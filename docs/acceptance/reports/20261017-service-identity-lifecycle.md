# S1 服务身份生命周期：持久化、发行/续期/撤销与客户端 adapter

- Run IDs：`service-identity-lifecycle-20261017`（存储）、
  `service-identity-api-20261017`（HTTP/授权/审计）、
  `service-identity-worker-renewal-20261017`（客户端 adapter）
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：IDENT-001、IDENT-002（2.1/2.2 完成；2.4 仅 adapter 部分）
- 日期：2026-10-17
- 类型：本地单元/竞态/契约 + 独立 PostgreSQL 16.15 集成测试。**不是**真实云主机、
  real_e2e、HA、5s/60s 撤销时延认证或生产资格。

## 1. 交付物

| 产物 | 内容 |
|---|---|
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | 三张 owning-plane 表；`service_enrollment_credentials` 单次原子消费；身份撤销 CAS version |
| `pkg/iam/service_identity.go` | 存储接口与 PostgreSQL 实现；跨租户查询、版本冲突、幂等撤销 |
| `pkg/iam/service_credential_authority.go` | 注册/签发/续期/撤销；签名前从存储身份重算 scope；撤销权威状态在 DB |
| `pkg/iam/http_credential_issuer.go` | bootstrap→renew 客户端；内存轮换；有界响应；错误脱敏 |
| `pkg/iam/tenant_token_source.go` | 每租户固定 scope 的 source；禁止跨租户复用 |
| `cmd/apiserver/internal/handler/service_identity.go` | 受限 HTTP 端点；`no-store`；错误不回显 token |
| `cmd/apiserver/internal/{router,middleware}` | `serviceIdentity` 路由/权限；renew 公共路由；审计正文抑制 |
| `contracts/schema/identity/v1/service-identity-create.schema.json` | 注册请求精确 scope |
| `contracts/schema/identity/v1/service-credential.schema.json` | 签发响应；credential 为 `writeOnly` |
| `pkg/audit/store.go` | `service_subject_id` 区分 actor 与服务主体 |

## 2. 验证命令与结果

环境：Linux（WSL）/ amd64；Go 工具链由仓库 `go.work`（go 1.26.0）解析；
独立测试数据库 `postgres:16`（PostgreSQL 16.15），仅监听 `127.0.0.1:55432`，
数据库 `hnb_test`，测试后即可丢弃。**不是**云主机或生产数据库。

| 命令 | 结果 |
|---|---|
| `go test -race ./pkg/iam/... -count=1` | 通过（3.171s） |
| `go vet ./pkg/iam/...` | 无输出，退出 0 |
| `go test -race ./cmd/apiserver/internal/handler/ -run TestServiceIdentityHandler -count=1` | 通过（1.031s） |
| `HNB_TEST_DATABASE_URL=... go test -tags integration ./pkg/iam/ -run TestServiceIdentityStoreAtomicConsumptionAndRevocation -count=1 -v` | **PASS**（PostgreSQL 16.15，0.14s） |
| `go build ./cmd/apiserver/...` | 通过 |
| `go build ./cmd/operation-worker/...` | 通过 |
| `npm run contracts:test` | **27/27** 通过 |
| `HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs` | 通过：80 operations、5 APIs、36 messages、**77 JSON schemas**、compatibility=checked |
| `node scripts/validate-openspec.mjs` | 34/34；48 specs / 287 requirements / 476 scenarios |
| `gofmt -l`（改动 Go 文件） | 干净 |

### 2.1 数据库原子性证据

集成测试 `TestServiceIdentityStoreAtomicConsumptionAndRevocation` 在真实 PostgreSQL 上验证：

- 同一迁移连续执行 **两次** 成功；
- 100 个并发 `ConsumeEnrollmentCredential` 对同一未消费凭据：**恰好 1 个成功**，
  其余全部 `ErrEnrollmentUnavailable`；
- 已消费后跨租户/跨目标重放被拒绝；
- 撤销使用错误 version → `ErrServiceIdentityVersionConflict`；
- 跨租户读取/撤销 → `ErrServiceIdentityNotFound`；
- 重复撤销同一身份 → `ErrServiceIdentityRevoked`；
- 重复撤销同一凭据幂等；
- 过期 enrollment 无法消费。

### 2.2 HTTP/授权/审计证据

- 注册 → 签发 → 续期 → 撤销整个身份：续期得到的凭据在身份撤销后返回
  `ErrServiceIdentityRevoked`，即使 JWT 签名仍有效。
- 跨租户 `GET`/`Issue` → 404；非法续期 bearer → 401，响应体不含所提交 token。
- 审计事件均含 actor subject 与 service subject，且 tenant 正确。
- 契约测试覆盖：非 UUID subject/credentialId、通配 audience/tenant/resourceKind、
  空 resourceId、未知字段、缺失字段、`expiresIn` 0/61。

## 3. 明确未完成

- 2.4 部署级 per-tenant 身份声明/bootstrap、operation-worker factory 接线、BFF actor
  保留验证；当前仅交付客户端 adapter 与驱动按租户取凭据的能力。
- 2.5–2.7 Agent 单次入网/轮换/长连接撤销；`service_enrollment_credentials` 表已具备，
  但 Agent exchange 路径尚未接线。
- 5s 新调用 / 60s 长连接撤销时延、撤销缓存、多 verifier/缓存重启演练。
- 超过两个 TTL 且含服务/Agent 重启的真实演练（2.8）。
- 真实云主机、独立业务集群、HA 或离线资格。

## 4. 被测文件摘要

| 文件 | sha256 |
|---|---|
| `database/postgresql/migrations/105_service_identity_lifecycle.sql` | `9a30951423d2a9d7dd6c972131c6cab90c609454a655a218320479eb93b8434c` |
| `pkg/iam/service_identity.go` | `d2cb9195fff27975b0218c0f05daed3c57c84b962cb65dc3b35366916a82bc08` |
| `pkg/iam/service_credential_authority.go` | `63fdbf90eb2094a12487575b38cd6f47b2b1acccdaefd8b3234d7edceead965f` |
| `pkg/iam/http_credential_issuer.go` | `55f681673ad5f27fb99fc91a3d4d609ae53eca9d2c2c5e22bb96fa530bf78a42` |
| `pkg/iam/tenant_token_source.go` | `dad51d80bcc36e3009c498706ffacc8f0ce56cee8e101e827d1dfbf7ebd52241` |
| `cmd/apiserver/internal/handler/service_identity.go` | `dcddb6e732e056a2b0b72bb0bc00322386b8f05c99d6fe3fa6afed3ff52e8e90` |
| `contracts/schema/identity/v1/service-identity-create.schema.json` | `77ca793cfb41b5e71cf30b84aa9e93ff419965ea781e632dd1add0cefb723b16` |
| `contracts/schema/identity/v1/service-credential.schema.json` | `0732abf0d76a98d678849f4485bb053ce2c518158d84b43d44432513e490d244` |

工作树与 HEAD 仍不同；以上为最终被测试文件字节摘要。
