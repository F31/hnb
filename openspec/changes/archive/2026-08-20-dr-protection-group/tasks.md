# Tasks: dr-protection-group

## Summary
| | |
|---|---|
| **Change** | `dr-protection-group` |
| **Created** | 2026-08-20 |
| **Specs** | observability-dr (OBS-008) |
| **Status** | Completed |

## 1. 契约与规格基线

- [x] 1.1 新增 `contracts/schema/dr/v1/`：DRProtectionGroup、DRGroupMember、DRSwitchRun JSON Schema
- [x] 1.2 console OpenAPI 增加 DR 端点（组 CRUD / 成员 / 切换 / 数据层确认）
- [x] 1.3 运行契约生成并过门禁；契约测试合法/非法样例

## 2. 数据库迁移

- [x] 2.1 `083_dr_protection_group.sql`：dr_protection_groups / dr_group_members / dr_switch_runs
- [x] 2.2 `083_dr_protection_group.rollback.sql`
- [x] 2.3 `test-migrations.sh`（PG16）验证

## 3. 编排核心（OBS-008）

- [x] 3.1 `pkg/iam` 增加 `dr` 资源类型（list/read/execute/update）
- [x] 3.2 apiserver 应用层：组 CRUD、成员管理（gslb_service / data_layer_ref）、租户隔离
- [x] 3.3 切换链：数据层成员全部确认完成前 SHALL NOT 提交流量层意图（顺序保证 + 测试断言）
- [x] 3.4 流量层步骤：为每个 gslb_service 成员构造并提交携带 drGroupRef 的 gslb.failover /
      gslb.switchback 意图（复用 gslb 应用层，审批门控生效）
- [x] 3.5 切换链审计：run 建立平台 Operation 行（switchover）+ traffic_request_ids 追踪 + Outbox 事件
- [x] 3.6 运行状态聚合：子流量请求终态推导 run 终态（Completed/Failed）

## 4. 验证与治理

- [x] 4.1 单测：编排顺序、幂等、权限、回切强制审批链路
- [x] 4.2 PG16 集成：迁移 + 完整切换链（数据层确认 → 流量意图 → Operation 行）
- [x] 4.3 `openspec validate --all --strict` + 契约门禁 + `validate-specs.sh`
- [x] 4.4 evidence 记录，全部通过后归档

## N/A 项说明

- 数据层 Provider 执行：本期为引用 + 人工确认门（见 proposal Non-Goals）
- Web Console 页面：后续 change
- 自动触发 / RPO-RTO 门禁：后续 change
