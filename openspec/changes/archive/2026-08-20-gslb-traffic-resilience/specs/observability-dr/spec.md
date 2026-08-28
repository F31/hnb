## ADDED Requirements

### Requirement: [OBS-007] DR 流量层编排对接缝
DRProtectionGroup 编排器 SHALL 以携带 `drGroupRef` 引用的 `gslb.failover` / `gslb.switchback` RuntimeIntent 发起流量层步骤，编排器 SHALL NOT 绕过意图提交入口直接触达 DNS 数据面；平台 SHALL 将 `drGroupRef` 记录于切换请求、关联 Operation 行与领域事件以供追踪；DR 来源的回切 SHALL 强制 `require_approval`，服务级审批降级 SHALL NOT 豁免该约束。

**Traceability:** GSLB-009

#### Scenario: DR 编排器发起流量层切换
- **GIVEN** DRProtectionGroup 已完成数据层切换
- **WHEN** 编排器提交携带 drGroupRef 的 gslb.failover 意图
- **THEN** 平台生成审批门控的切换请求并在 Operation Center 建立对应 Operation 行
- **AND** 请求、Operation 标签与事件均携带 drGroupRef 以供编排器追踪结果

#### Scenario: DR 回切强制人工确认
- **GIVEN** 某 GSLB 服务已配置服务级免审批降级
- **WHEN** DR 编排器提交携带 drGroupRef 的 gslb.switchback 意图
- **THEN** 请求仍进入 PendingApproval 等待显式人工确认
- **AND** 未经审批不派发任何执行命令
