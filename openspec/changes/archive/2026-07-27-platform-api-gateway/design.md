## Context

`operation-worker` 已拥有权威 Operation Store、10 态状态机、租约/fencing、重试与 outbox relay，但没有面向调用方的同步入口：外部组件无法提交 Operation、执行审批或查询状态。本 change 在不动 worker 的前提下新增 `platform-api`，承担"守门写入 + Read Model 查询"两个职责。

利益相关方：Portal/CLI/Copilot 调用方、平台运维、审计方、operation-worker 维护者。

## Goals / Non-Goals

**Goals:**
- 提供 REST 提交入口，单事务落 ExecutionPlan + Operation + Steps + Audit + Read Model + outbox（OP-007）。
- 高风险类型强制 pending_approval → approve 后 queued 并补发 outbox 事件（TENANT-003）。
- 幂等提交（tenant_id + idempotency_key 唯一约束兜底）。
- 查询只走 operation_read_model，响应携带 last_state_changed_at/lastObservedAt（KERNEL-003）。
- 全链路租户过滤与 Secret 引用约束（TENANT-002、CFG-002）。

**Non-Goals:**
- pause/resume、补偿、步骤级 API、事件推送。
- 认证/鉴权（由前置 Gateway 承担）；tenant_id 为显式必填参数。
- 新增表或列（复用 001/008 schema）。
- 修改 operation-worker。

## Architecture

```text
Portal/CLI/Copilot
        | HTTP /v1/operations (+ approve/reject/cancel)
        v
  platform-api -- 单事务写 --> PostgreSQL
        |                     operations / operation_steps /
        |                     execution_plans / operation_audit /
        |                     operation_read_model / outbox_events
        |                           ^
        |                           | relay 轮询
        v                           v
   (查询只读 read model)    outbox relay -> NATS -> operation-worker
```

platform-api 不连 NATS、不执行 Step；worker 不感知 platform-api 存在。

## Data Model

无 schema 变更。复用：

- `execution_plans`（plan_digest 唯一索引用于去重，`ON CONFLICT (plan_digest) DO NOTHING` + 回读 id）。
- `operations`（`idx_operations_tenant_idempotency` 唯一索引支撑幂等提交，23505 冲突时回读已有 Operation 返回 200）。
- `operation_steps`（status=pending、version=0、idempotency_key = `<opKey>:<planStepId>`，≤128 字符以满足 worker 校验）。
- `operation_read_model`（与 worker 的 upsert 逻辑完全同构，保证 last_state_changed_at）。
- `outbox_events`（`UNIQUE(message_type, idempotency_key)` 天然防止 approve 重试造成重复派发）。
- `operation_audit`（created/approved/rejected/cancelled）。

## API Contract

- `POST /v1/operations`：body `{tenantId, namespaceId, releaseId, operationType, idempotencyKey, initiatedBy, projectId?, environmentId?, correlationId?, tags?, steps[]}`；step `{id?, name, stepType, providerId?, dependsOn?, optional?, inputs?, secretReference?, maxRetries?, timeoutSeconds?}`。201 创建；幂等重放 200。响应为 Operation 详情（含 steps、last_state_changed_at、lastObservedAt）。
- `POST /v1/operations/{id}/approve|reject|cancel`：body `{tenantId, actorId, reason?}`；200 返回更新后详情；404 不存在或跨租户；409 状态不允许。
- `GET /v1/operations?tenant_id=&status=&type=&limit=&offset=`：limit 默认 50、上限 200；返回 `{operations[], total, limit, offset}`。
- `GET /v1/operations/{id}?tenant_id=`：详情 + steps。
- `GET /healthz`：DB ping，200/503。
- 错误统一 `{"error":{"code","message"}}`；请求体上限 1 MiB；未知字段与多文档 JSON 拒绝。

## Outbox 事件契约

与 worker 消费者严格对齐（以 worker 代码为准，而非任务描述中的旧 subject 名）：

- subject = message_type = `hnb.command.operation.step-requested.v1`，schema_version `1.0.0`。
- envelope：correlation_id（UUID）、idempotency_key = step 幂等键、aggregate_id/operation_id/step_id、expected_version = step.version（提交/审批时为 0）。
- payload：`{operationId, stepId, stepType, idempotencyKey, expectedVersion}`，与 `stepRequestMessage` 一致。
- 仅无依赖（depends_on 为空）的 ready Steps 在 queued 时派发；有依赖 Steps 的派发由后续 step-completed 链路负责（本期不在 platform-api 做 DAG 推进）。

## State Machine

platform-api 只发起以下迁移，其余归 worker：

```text
create:  (none) -> queued            (低风险类型)
         (none) -> pending_approval  (delete/rollback/config_change)
approve: pending_approval -> queued
reject:  pending_approval -> cancelled
cancel:  pending|pending_approval|queued|paused|compensating -> cancelled
```

迁移 SQL 带 `WHERE status = <locked>` 乐观护栏并检查 RowsAffected；行锁（SELECT ... FOR UPDATE）保证并发审批/取消互斥。

## Decisions

### 复用现有表，不新增 014 迁移
001/008 已提供全部所需表与唯一索引，platform-api 纯属新增写入方/读取方。新迁移只会增加运维面，故证据中记录迁移为 N/A。

### 事务内直写 Read Model
worker 只在 step 提交时 upsert Read Model；若 platform-api 不写，则新提交/审批中的 Operation 对查询不可见，违反 KERNEL-003 的查询出口职责。选择与 worker 完全同构的 upsert 语句（COALESCE 保护 started_at/completed_at），避免双写方竞争产生回退。

### Handler 依赖 Store 接口
store.Store 接口让 handler 测试用内存 fake 走通全部 HTTP 语义（状态机、租户拒绝、分页、幂等），无需 sqlmock 新依赖（本机模块缓存无 go-sqlmock，且网络受限）。

### maxRetries/timeoutSeconds 默认值
请求省略时 maxRetries=3、timeoutSeconds=300，与表默认值一致；显式 0 视为"取默认"（API 层不区分未设置与 0，文档化）。

## Security And Operations

- 租户隔离：所有 SQL 以 tenant_id 为强制谓词；跨租户返回 404 而非 403，不泄露资源存在性。
- Secret：输入键命中 `(?i)(password|passwd|secret|token|credential|private[_-]?key|api[_-]?key)` 且不以 reference 结尾即拒绝；私钥材料正则拒绝；secretReference 仅作不透明字符串透传进 step_input，日志与错误不输出请求体。
- 供应链：无新依赖（lib/pq、google/uuid 与 worker 同版本）。
- 权限：服务账号仅需既有表的 CRUD；无 DDL。
- 审计：created/approved/rejected/cancelled 全部落 operation_audit（actor、前后状态、reason）。
- 性能/容量：单请求单事务；列表走 tenant+status 索引；1 MiB 请求体上限。
- 可观测：5xx 记录方法+路径+错误（不含 body）；审计表为权威证据链。
- 升级/回滚/灾备：无状态服务，滚动部署；回滚即摘除服务，outbox 中已写入事件由 worker 正常消费；DR 沿用 PostgreSQL 策略。
- 安装/卸载：配置 `DB_*`、`LISTEN_ADDR` 环境变量；卸载不影响在途 Operation。

## Failure Modes

- 提交中途失败 → 整体回滚，无半持久化 Operation。
- 幂等冲突与原始事务未提交的竞态 → 回读 miss 时返回 404/错误，客户端重试即可收敛。
- approve 与 cancel 并发 → 行锁 + 状态护栏，一方 409。
- outbox 事件 UNIQUE(message_type, idempotency_key) 冲突 → 事务回滚，调用方重试；不会重复派发。
- Read Model 与 operations 瞬时不一致（worker 并发投影） → 查询返回最近投影，last_state_changed_at 标识新鲜度。

## Risks / Trade-offs

- [有依赖 Steps 的 DAG 推进不在 platform-api] -> 依赖 worker 的 step-completed 处理链；若该链路缺失，多步 DAG 仅首层 ready Steps 会被派发，需在后续 change 补齐。
- [tenant_id 由请求自报] -> 认证/租户断言属前置 Gateway 职责；在引入 Gateway 前 platform-api 不应直接暴露公网。
- [显式 maxRetries=0 不可表达] -> API 文档化默认语义；如确需 0 重试再演进为可空字段。

## Migration Plan

1. 确认 001~013 迁移已应用。
2. 部署 platform-api（`DB_*`、`LISTEN_ADDR`），健康检查通过后接入 Gateway/Ingress。
3. 用一个低风险 deploy Operation 做金丝雀：提交 → worker 执行 → 查询状态推进。
4. 回滚：摘除 platform-api；无需数据回滚。

## Open Questions

- DAG 后续层 Step 的派发归属（worker step-completed 链 vs 独立 dispatcher）。
- Gateway 认证落地后 tenant_id 改由身份断言注入的时间点。
