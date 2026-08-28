## ADDED Requirements

### Requirement: [OBS-008] DR 保护组编排
平台 SHALL 提供 DRProtectionGroup 作为跨地域容灾编排单元：组成员分数据层与流量层两类；
切换 SHALL 按"数据层 → 流量层"顺序编排——数据层成员全部确认完成前，平台 SHALL NOT
为流量层成员发起任何切换；流量层步骤 SHALL 通过携带 `drGroupRef` 的 gslb 受控意图落地
（OBS-007），继承其审批门控、Operation 行与事件追踪；每次切换 SHALL 建立平台 Operation
行并记录全部子流量请求引用，切换链整体可审计；回切 SHALL 经同一编排链路并强制人工审批。

**Traceability:** GSLB-009, OBS-007

#### Scenario: 数据层未完成不发起流量层切换
- **GIVEN** DRProtectionGroup 含至少一个数据层成员
- **WHEN** 运维发起地域级切换但数据层确认未完成
- **THEN** 切换运行停留在 DataLayerPending
- **AND** 不产生任何 gslb 切换请求与 DNS 变更

#### Scenario: 数据层确认后执行流量层步骤
- **GIVEN** 切换运行的全部数据层成员已确认完成
- **WHEN** 数据层确认生效
- **THEN** 平台为每个流量层成员提交携带 drGroupRef 的 gslb.failover 意图
- **AND** 每个意图建立审批门控的切换请求与关联 Operation 行
- **AND** 切换运行的 Operation 记录全部子请求引用

#### Scenario: 回切强制人工确认
- **GIVEN** 服务已配置服务级免审批降级
- **WHEN** 经 DRProtectionGroup 发起回切
- **THEN** gslb.switchback 意图携带 drGroupRef 并强制进入 PendingApproval
- **AND** 未经显式人工审批不派发任何执行命令
