# Proposal: dr-protection-group

## Change ID

- **Change**: `dr-protection-group`
- **Created**: 2026-08-20
- **Tier**: T2（标准可选，多地域生产形态）
- **影响平面**: HNB Core（Operation / Read Model）、apiserver
- **受影响 specs**: `observability-dr`（新增 OBS-008）
- **依赖 change**: `gslb-traffic-resilience`（OBS-007 对接缝）、`operation-engine-core`、`identity-tenancy`
- **迁移影响**: 新增数据库迁移 `083_dr_protection_group`（含 rollback）；console OpenAPI 扩展；契约 `schema/dr/v1`
- **回滚策略**: `083` rollback 脚本；DR API 下线

## Why

`gslb-traffic-resilience` 已落地 GSLB 流量层受控变更与 DR 对接缝（OBS-007：`drGroupRef` 引用 +
强制回切审批），但编排器本体缺失：白皮书 §8.1 的"级联而非二选一"容灾模型中，
DRProtectionGroup 负责解决机房/地域级故障的"数据层 → 流量层"顺序编排，目前代码库无任何实现。

## What Changes

- **DRProtectionGroup 资源**：租户隔离的保护组（主/备地域），成员分数据层（引用，预留 Provider 接入）
  与流量层（GSLBService）两类
- **切换链编排**：发起切换 → 数据层成员显式确认完成 → 平台为每个流量层成员提交携带
  `drGroupRef` 的 `gslb.failover` 意图（审批门控、Operation 行、事件追踪全部复用既有链路）
- **回切**：同一编排链路提交 `gslb.switchback`；因携带 `drGroupRef`，强制人工审批
  （服务级免审批降级不可豁免，OBS-007）
- **审计**：每次切换建立平台 Operation 行（`switchover` 类型），记录全部子流量请求引用，
  切换链整体在 Operation Center 可观测
- **契约**：`contracts/schema/dr/v1` + console OpenAPI（DR 组 CRUD / 成员 / 切换 / 数据层确认）

## Capabilities

### New Capabilities

- `observability-dr` OBS-008：DR 保护组编排（详见 delta spec）

### Modified Capabilities

无（不改动既有能力行为；复用 gslb 意图提交链路）

## Impact

- **现有服务改造**：`apiserver`（新增 DR 应用层/仓储/handler/路由）
- **新组件**：无新独立服务
- **新数据库表**：083 迁移 3 张表（dr_protection_groups / dr_group_members / dr_switch_runs）
- **安全风险**：切换链为高风险操作——流量层步骤全部经审批门控 gslb 意图落地；DR 发起需 `dr:execute`
- **兼容性**：纯新增，不影响既有路径
- **退出判据**：OBS-008 验收场景通过；数据层未确认前不发起流量层步骤有测试断言；
  回切强制审批有测试断言；契约与 OpenSpec 门禁通过；PG16 迁移验证通过

## Non-Goals

- 不做数据层复制 Provider（replication_provider / 真实数据层切换执行）——数据层成员为引用 +
  人工确认门，Provider 化接入留待后续 change
- 不做 DR 自动触发（仅人工发起）
- 不做 RPO/RTO 采集与就绪度门禁（`last_drill_at` 等，见技术实现方案 §12，后续 change）
- 不做 Web Console DR 页面（本轮仅 API + 契约；产品面后续 change）
