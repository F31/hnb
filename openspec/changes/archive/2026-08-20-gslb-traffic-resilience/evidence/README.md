# Evidence: gslb-traffic-resilience

本目录记录 change 各任务的验收证据（2026-08-19 更新）。

## P0 — 受控流量变更（GSLB-005，消除旁路）✅

| 证据 | 位置 | 结果 |
|---|---|---|
| 类型化 Intent + 校验 + 不可变 Plan | `pkg/gslb`（intent.go / plan.go） | 单测 10 例通过 |
| 禁止执行字段 fail-closed（providerId/command） | `pkg/gslb/intent_test.go` | 通过 |
| 审批门控提交/审批/拒绝 + Outbox 命令事件 | `cmd/apiserver/internal/application/gslb` + `internal/handler/gslb.go` | 单测 10 例 + handler 6 例通过 |
| 控制器拆除直写 DNSEndpoint | `cmd/gslb-controller/internal/reconciler`（无 dns.Manager） | 编译期保证（无 DNS 写引用） |
| DNS 数据面唯一写入口 = executor（SPI 驱动） | `internal/executor` + `internal/provider` | 单测 8 例通过（apply/verify/revert/补偿/演练只读） |
| NATS 执行命令消费（状态守卫 + 幂等重试） | `internal/consumer` | 单测 5 例通过 |
| PG16 集成：切换请求 + outbox + 租户隔离 | apiserver `internal/infrastructure/gslb` | PASS |
| PG16+NATS 端到端：命令 → 执行 → Succeeded | controller `internal/consumer` | PASS |

## T1 — Schema First 契约（GSLB-005/006/007/008）✅

- `contracts/schema/gslb/v1/` 6 个 JSON Schema（service/pool/member/health-check/read-model/intent）
- console OpenAPI 6 个 GSLB 端点 + 契约生成；门禁 **67 operations / 70 schemas 通过**
- 契约测试 25/25 通过（含 gslb-intent 合法/非法 fixture：非法含 providerId/command 被拒）

## T4 — gslb-dns-provider SPI（GSLB-006）✅

- `cmd/gslb-controller/internal/provider`：ApplyRecords / VerifyTargets / DeleteRecords 契约
- ExternalDNS 参考实现（基于现有 dns.Manager，改由 SPI 驱动）
- 单测 3 例通过；执行器经 SPI 调用（测试用 fake provider）

## T5 — Read Model 与查询 API（GSLB-007）✅

- 控制器投影器 `ProjectReadModel`（以探活结果计算健康池/当前目标）
- apiserver `GET /api/v1/gslb/services` + `/{id}`（只读，租户隔离）
- PG16 集成：投影健康→失健康清空目标，通过

## T6 — 多租户隔离（GSLB-008）✅

- `gslb_services`/`gslb_switch_requests`/`gslb_read_model` 均含 tenant_id
- 服务/请求/投影读取按租户过滤；授权 `gslb:list/read/execute/update`
- 集成测试含跨租户读取拒绝断言

## T8 — 只读演练（GSLB-010）✅

- `gslb.drill` 意图 → DrillCompleted（不派发执行命令、无 DNS 变更）
- 单测 `TestSubmitDrillCompletesWithoutDispatch` + `TestExecuteStepDrillIsReadOnly`

## T9 — Web Console（资源 → GSLB）✅

- `plugins/resource/src/gslbApi.ts` + `pages/GSLB.vue` 替换 stub：列表（Read Model）、
  详情、故障转移/回切/演练动作（审批门控提示）、请求状态展示
- 插件 typecheck 通过

## T7 — DR Group 流量层步骤对接缝（GSLB-009）✅（GSLB 侧）

DRProtectionGroup 编排器本体在代码库尚不存在（observability-dr 仅含 OBS-001~006），
本轮落地 **GSLB 侧对接缝**，编排器实现由 observability-dr 独立 change 推进：

| 证据 | 位置 | 结果 |
|---|---|---|
| 意图携带 drGroupRef 引用（仅审计/追踪，fail-closed 校验） | `pkg/gslb/intent.go` + 契约 `gslb-intent.schema.json` | 单测 2 例通过 |
| DR 来源回切强制审批（服务级降级不可豁免） | `cmd/apiserver/internal/application/gslb/service.go` | `TestDRSwitchbackForcesApproval` 通过 |
| drGroupRef 落库 + Operation 标签 + 事件追踪 | 迁移 082 `gslb_switch_requests.dr_group_ref` | PG16 集成断言通过 |
| observability-dr spec delta（OBS-007 对接缝需求） | `specs/observability-dr/spec.md` | `openspec validate --all --strict` 通过 |

## 演练报告结构化落库（GSLB-010 补强）✅

- 迁移 082：`gslb_drill_reports` 表（verdict CHECK + 租户/服务索引）+ `gslb_read_model.last_drill_*`
- apiserver 提交 drill 意图时计算结构化报告（当前/预期目标、健康上下文、检查项、结论），
  同事务落库并投影 Read Model 最近演练
- 查询 API `GET /api/v1/gslb/services/{id}/drills`（租户隔离，请求路径零探测）
- 前端 `GSLB.vue` 详情区展示演练报告（结论徽标 + 检查项），文案入插件 i18n（zh-CN/en-US）
- 单测 2 例 + PG16 集成断言通过

## 平台 Operation 行统一接线 ✅

- 迁移 082：`operations.operation_type` 扩展 `gslb_failover/gslb_switchback/gslb_weight_update/gslb_drill`；
  `gslb_switch_requests.operation_id` 关联平台 operations 行
- apiserver 提交/审批/拒绝同事务建立并同步 operations + operation_steps + operation_read_model
  （pending_approval → queued / cancelled；drill 立即 succeeded 且 step_output 携带报告结论）
- gslb-controller 执行结果同事务同步（Dispatched → in_progress；Succeeded → succeeded，
  apply/verify succeeded + revert skipped；Failed → failed 记录原因）；
  自动故障转移路径同样建立 Operation 行
- PG16 集成：建行/审批同步/执行同步/步骤终态断言全部通过

## T10 — 治理与验证 ✅

- 10.3 E2E：`web/e2e/gslb.spec.ts` 覆盖列表 → 详情 → 演练（报告展示）→ 切换（审批门控提示）
  → 审批 → 回切全流程；Playwright 全量 30/30 通过
- 10.4 性能采样：`postgres_store_perf_test.go`（PG16，200 服务 × 200 次采样）——
  ListReadModels P95 = 1.9ms、GetReadModel P95 = 1.4ms，远低于 200ms 预算

## 待办（非阻塞，后续 change）

- DRProtectionGroup 编排器本体（observability-dr 域）：消费 OBS-007 对接缝，
  编排"数据层 → 流量层"切换链——建议独立 change 推进

## 门禁记录

- `openspec validate --all --strict`：通过（29 项，含新增 observability-dr delta）
- `validate-specs.sh`：通过
- `validate-contracts.mjs`：通过（68 ops / 70 schemas）
- 契约测试 25/25 通过
- `test-migrations.sh`（PG16）：通过（含 082 空库/重复/升级/回滚）
- apiserver 全量 `go test ./...`：通过
- gslb-controller 全量测试 + PG16/NATS 集成：通过
- 前端 vitest 全量（61 文件 / 304 用例）+ 插件 typecheck：通过
- Playwright E2E 全量 30/30：通过（含 `gslb.spec.ts` 全流程）
- Read Model P95 采样（PG16）：List 1.9ms / Get 1.4ms（预算 200ms）
