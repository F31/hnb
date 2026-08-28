# Evidence: dr-protection-group

本目录记录 change 各任务的验收证据（2026-08-20）。

## 1. 契约与规格基线 ✅

- `contracts/schema/dr/v1/` 3 个 JSON Schema：dr-protection-group / dr-group-member / dr-switch-run
  （方向、状态机、成员类型枚举与 `additionalProperties: false` fail-closed 约束）
- console OpenAPI 7 个 DR 端点（组 CRUD / 成员 / 运行列表 / 切换 / 数据层确认），tag `DR Protection Groups`
- 契约测试合法/非法 fixture 各 3 组注册进 `scripts/contracts.test.mjs` structuralExamples，全量 25/25 通过

## 2. 数据库迁移 ✅

- `083_dr_protection_group.sql`：dr_protection_groups（租户+名唯一）/ dr_group_members
  （gslb_service | data_layer_ref）/ dr_switch_runs（7 态状态机 + operation_id 外键 +
  traffic_request_ids UUID[] + 组内幂等键唯一）
- `083_dr_protection_group.rollback.sql` 非破坏性回滚
- `test-migrations.sh`（PG16）6/6 通过：空库前向 / 幂等重跑 / 混合版本升级 / 回滚演练 / 快照恢复

## 3. 编排核心（OBS-008）✅

| 证据 | 位置 | 结果 |
|---|---|---|
| `dr` 资源类型（create/list/read/update/execute） | `pkg/iam/authorization.go` | 编译 + 单测权限断言通过 |
| 组 CRUD / 成员管理 / 租户隔离 | `cmd/apiserver/internal/application/dr/service.go` | 单测通过（跨租户读取拒绝见集成） |
| **顺序保证**：数据层成员确认前 SHALL NOT 提交流量层意图 | `service_test.go::TestInitiateSwitchDataLayerGate` | submitter 零调用断言通过 |
| 确认后每个 gslb_service 成员各提交携带 drGroupRef=组 ID 的意图 | `TestConfirmDataLayerDispatchesTraffic` | drGroupRef / 成员级幂等键断言通过 |
| 回切方向 = gslb.switchback + 主池目标（审批门控由 gslb 链路强制，GSLB 侧 OBS-007 已保证） | `TestSwitchbackUsesPrimaryPool` | 通过 |
| 幂等重放不重复触达流量层 | `TestInitiateSwitchIdempotentReplay` | 通过 |
| 活跃运行冲突 ErrConflict | `TestInitiateSwitchActiveRunConflict` | 通过 |
| 运行审计：平台 Operation 行（switchover）+ traffic_request_ids + Outbox 事件 | `infrastructure/dr/postgres_store.go` | PG16 集成断言通过 |
| 运行终态聚合（子 gslb 请求全 Succeeded → Completed；任一 Failed/Rejected → Failed） | `TestAggregateRunsTerminalStates` | 通过 |
| 目标池解析失败 fail-closed（run 置 Failed 记录原因） | `TestDispatchFailureFailsRun` | 通过 |

## 4. 验证与治理 ✅

- 单测：`application/dr` 10 例通过（顺序/幂等/权限/冲突/聚合/失败路径）
- PG16 集成：`infrastructure/dr` 2 例通过——CreateRun 同事务 operations(switchover/in_progress)
  + operation_read_model + outbox 落库；UpdateRun 终态同步 succeeded + completed_at；
  GetGSLBBackupPool（非活跃池 priority DESC）/ GetGSLBPrimaryPool（priority ASC）解析正确；
  TrafficRequestStatuses 聚合查询正确
- `openspec validate --all --strict`：29 项通过（含 OBS-008 delta）
- `validate-specs.sh`：Quality Gate PASSED
- 契约生成 `generate-contracts.mjs --cluster-management` + `contracts.test.mjs` 25/25：通过
- `validate-contracts.mjs`：通过（75 ops / 73 schemas / compatibility=checked）
- apiserver 全量 `go build` / `go vet` / `go test ./cmd/apiserver/...`：通过；gofmt 干净

## 非目标（proposal Non-Goals，留待后续 change）

- 数据层 Provider 真实执行：本期为引用 + 人工确认门
- Web Console DR 页面
- 自动触发 / RPO-RTO 门禁

## 门禁记录

- `openspec validate --all --strict`：通过（29 项）
- `validate-specs.sh`：通过
- 契约测试 25/25 通过（含 dr 3 组 fixture）
- `test-migrations.sh`（PG16）：通过（含 083）
- apiserver 全量 go test：通过
- 测试容器已清理（docker 无残留）
