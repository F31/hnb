## Why

平台缺少 Operation 治理平面的统一 REST 入口。KERNEL-002 要求所有运行时变更必须经过持久化 Operation 状态机，KERNEL-003 要求查询与控制解耦；目前只有 `operation-worker` 直接消费数据库与 NATS，外部调用方（Portal、CLI、Copilot）没有受守门、可审计、按租户隔离的提交/审批/查询 API。本 change 新增 `cmd/platform-api` 服务作为 Operation 唯一写路径的守门入口和 Read Model 的查询出口。

## What Changes

- Change ID: `platform-api-gateway`。
- 新增 T0 服务 `cmd/platform-api`（Go 独立 module，与 `cmd/operation-worker` 同布局）：`POST /v1/operations`（提交，事务写入 execution_plans + operations + operation_steps + operation_audit + operation_read_model + step-requested outbox 事件，OP-007）、`POST /v1/operations/{id}/approve|reject|cancel`、`GET /v1/operations`、`GET /v1/operations/{id}`、`GET /healthz`。
- 高风险 operation_type（delete/rollback/config_change）初始进入 pending_approval，审批后才 queued 并补发 outbox 事件（TENANT-003）。
- 幂等提交：`(tenant_id, idempotency_key)` 唯一约束冲突时返回已有 Operation（HTTP 200）。
- 查询只读 `operation_read_model`（详情附带 operation_steps），响应始终携带 `last_state_changed_at` / `lastObservedAt`（KERNEL-003）。
- 所有请求按 tenant_id 过滤，跨租户访问一律 404 拒绝（TENANT-002）；只接受 `secretReference`，拒绝疑似明文 Secret 的输入键与私钥材料（CFG-002）。
- platform-api 只写 `outbox_events`，不直连 NATS；outbox subject 与 payload 与 `operation-worker` 现有消费者契约（`hnb.command.operation.step-requested.v1`，schema 1.0.0）保持一致。
- 分级：T0 内核组件。影响平面：运行治理/控制面；制品、数据、AI Extension、Portal 平面不变。
- 依赖：已完成 `operation-engine-core`、`operation-fencing-v2`；无新中间件、数据库或第三方库（仅复用 lib/pq、google/uuid）。
- 数据库迁移：N/A——复用 001/008 已建表（execution_plans、operations、operation_steps、operation_audit、operation_read_model、outbox_events），无新表/新列，故不新增 014 迁移。
- 回滚：停止并移除 platform-api 部署即可；已提交 Operation 由既有 worker/outbox 链路继续处理，无数据回滚。
- 用户价值：Portal/CLI/Copilot 获得唯一、可审计、租户隔离的 Operation 提交与审批入口；高风险变更强制审批门控。
- 非目标：pause/resume、补偿触发、步骤级操作、Operation 事件订阅（SSE/WebSocket）、认证鉴权中间件（由前置 Gateway 承担）、Read Model 独立投影器。
- 兼容性：不改写现有表结构，不修改 operation-worker 代码；outbox 事件契约与 worker 消费者现有校验完全对齐。
- 安全：请求/响应/日志不含明文 Secret；日志不记录请求体；错误响应不泄露 SQL 细节。
- 资源预算：每请求一次有界事务，请求体上限 1 MiB，列表分页上限 200 条。
- 可观测：操作结果写入 operation_audit 证据链；HTTP 层记录方法与路径级错误日志（不含请求体）。
- 退出判据：module 单测（含 race）与 `go vet` 通过，`openspec validate --strict` 通过，evidence.md 记录验证与未验证项。

## Capabilities

### New Capabilities
- 无新顶层能力域；本 change 在 `platform-kernel` 下细化 KERNEL-002/003 的 API 行为。

### Modified Capabilities
- `platform-kernel`：新增 PAG-001~PAG-005  Requirement，细化 Operation 提交事务、审批门控、取消语义、只读查询出口、租户与 Secret 约束。

## Impact

- 受影响代码：新增 `cmd/platform-api/**`；新增 `openspec/changes/platform-api-gateway/**`。不修改任何现有服务代码。
- 受影响 API：新增 `/v1/operations` REST 契约；新增对 `hnb.command.operation.step-requested.v1` outbox 事件的写入方（契约本身不变）。
- 运维：新服务需配置 `DB_*` 与 `LISTEN_ADDR` 环境变量；部署顺序要求在 001/008 迁移之后。
- 数据/安装/升级/备份/恢复/卸载：无 schema 变更；卸载 platform-api 不影响在途 Operation。
