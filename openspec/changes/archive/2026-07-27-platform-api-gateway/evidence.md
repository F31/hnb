## Verification Evidence

Date: 2026-07-22

### 构建与单元测试

环境说明：与任务简报相反，本机 Go 工具链（go1.24.2，/usr/local/go）实际可用；依赖（lib/pq v1.10.9、google/uuid v1.6.0）命中本地模块缓存，`GOPROXY=off` 完成 `go mod tidy` 与全部编译。

Command:

```text
cd cmd/platform-api
GOPROXY=off go mod tidy
GOPROXY=off go build ./...
GOPROXY=off go vet ./...
```

Result: 全部通过，无告警。

Command:

```text
GOPROXY=off go test -count=1 ./...
GOPROXY=off go test -race -count=1 ./...
```

Result:

```text
ok  github.com/F31/hnb/cmd/platform-api/internal/api
ok  github.com/F31/hnb/cmd/platform-api/internal/config
ok  github.com/F31/hnb/cmd/platform-api/internal/store
```

（race 运行同样全绿。）覆盖点：初始状态分类（高风险 → pending_approval）、validTransitions 取消矩阵、提交校验（必填字段、幂等键长度、依赖存在性/自依赖/环）、明文 Secret 键与私钥材料拒绝、secretReference 接受；handler 层经 httptest + 内存 fake store 覆盖：201/200 幂等重放、approve/reject/cancel 状态机与 409/404、跨租户 404、列表过滤与分页、响应携带 lastObservedAt/last_state_changed_at、healthz。

### OpenSpec 与格式

Commands:

```text
openspec validate platform-api-gateway --strict
gofmt -l cmd/platform-api
git diff --check -- cmd/platform-api openspec/changes/platform-api-gateway
```

Result: change 校验通过（"Change 'platform-api-gateway' is valid"）；gofmt 无输出。

### 契约核对（静态）

- outbox subject/message_type/schema 与 `cmd/operation-worker/internal/nats/worker.go` 消费者逐项核对：`hnb.command.operation.step-requested.v1`、`1.0.0`、payload 字段 `{operationId, stepId, stepType, idempotencyKey, expectedVersion}`。**注意：任务简报中的 `hnb.operation.step.requested` 与代码不符，实现以 worker 实际 subject 为准。**
- outbox 写入列与 `001_nats_jetstream_outbox.sql` 表结构逐项核对；Read Model upsert 与 `store/operations.go:upsertReadModel` 同构。
- 状态机常量与 validTransitions 复制自 `engine/state.go`（platform-api 仅使用 pending→queued/pending_approval、pending_approval→queued/cancelled、各源状态→cancelled 子集）。

## 未验证项 / N/A

- **真实 PostgreSQL 集成测试：已完成。** 2026-07-24 在 PostgreSQL 16.14（端口 5433）上运行全部 38 项集成测试（33 项新增 + 5 项既有），覆盖：

  **写入路径：**
  - 低风险类型（deploy）→ 初始状态 queued ✅
  - 高风险类型（delete/rollback/config_change）→ 初始状态 pending_approval ✅
  - 幂等提交：相同 (tenant_id, idempotency_key) 返回已有 Operation（200）✅
  - 跨租户相同键：不同租户各自创建新 Operation ✅
  - 审批流程：pending_approval → queued，记录 approved_by ✅
  - 驳回流程：pending_approval → cancelled ✅
  - 取消流程：queued → cancelled ✅
  - 已终态（cancelled）取消 → ErrInvalidState ✅
  - 跨租户审批/查询 → ErrNotFound（404 语义）✅
  - 不存在 Operation 的审批/取消 → ErrNotFound ✅

  **查询路径（Read Model）：**
  - GetOperation 返回正确的状态、步骤、last_state_changed_at ✅
  - 跨租户查询返回 ErrNotFound ✅
  - ListOperations 按租户+状态过滤、分页、总计 ✅

  **事务完整性：**
  - 单事务中持久化 execution_plans + operations + operation_steps + operation_audit + operation_read_model ✅
  - 低风险提交时 outbox 写入 step-requested 事件 ✅
  - 审批后 outbox 写入 step-requested 事件 ✅
  - 多步 DAG 仅就绪步骤（无依赖）写入 outbox ✅
  - 审计：created + approved 等事件全部落 operation_audit ✅

  **数据合规：**
  - Tags 正确持久化到 operations 表 ✅
  - SecretReference 正确透传 ✅
  - 依赖步骤（depends_on）正确持久化 ✅
  - 相同计划内容复用 plan_digest ✅
  - CorrelationID 正确持久化；空时自动生成 UUID ✅

  **RuntimeTarget CRUD：**
  - Create/Get/List/Delete/UpdateStatus 全部通过 ✅

  所有测试带有 t.Cleanup 清理，与环境隔离，由 `HNB_TEST_POSTGRES_DSN` 环境变量门控。

- E2E（platform-api → outbox relay → worker 执行）：N/A，依赖 PostgreSQL + NATS 运行时环境。
- 数据库迁移：N/A。复用 001/008 已建表与唯一索引，无新表/新列，故未创建 014_platform_api.sql；如后续需要平台 API 专用审计索引再单独提出。
- 对现有服务的修改：无。operation-worker 及其他服务代码零改动。

## Rollout And Rollback

部署时配置 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME/DB_SSLMODE` 与 `LISTEN_ADDR`（默认 :8080），要求 001~013 迁移已应用。回滚只需摘除 platform-api 实例：已提交 Operation 与 outbox 事件由既有 relay/worker 链路继续处理，无数据回滚。认证与租户断言由前置 Gateway 承担，接入 Gateway 前不应直接对外暴露。
