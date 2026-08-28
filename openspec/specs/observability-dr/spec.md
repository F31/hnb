## Purpose
定义统一遥测、Operation SLO、备份恢复、故障演练、性能预算和边缘遥测补传能力，使平台具备可观测、可恢复和可验证的生产运维基础。
## Requirements
### Requirement: [OBS-001] 统一遥测上下文
平台、市场、Provider、Gateway、AI 和 Edge 组件 SHALL 输出结构化指标、日志、链路和事件，并包含 Tenant、Correlation ID、Operation ID 和 Resource ID。

**Traceability:** AI-GOV-05, GW-13, EDGE-14

#### Scenario: 追踪一次部署故障
- **GIVEN** 部署跨越市场、平台和 Provider
- **WHEN** 运维打开 Operation 详情
- **THEN** 可以跳转到相关日志、链路和资源指标
- **AND** 敏感字段不被采集

### Requirement: [OBS-002] Operation 滞留 SLO
每个非终态 Operation SHALL 配置最大滞留时间、告警和升级策略；状态展示 SHALL 包含进入时间、重试次数和阻塞原因。

**Traceability:** METH-02

#### Scenario: Operation 长时间 InProgress
- **GIVEN** 步骤超过配置 SLO
- **WHEN** 监控检测到滞留
- **THEN** 触发告警并显示阻塞步骤
- **AND** 允许安全暂停或取消

### Requirement: [OBS-003] 备份与恢复是产品能力
平台元数据库、市场数据库、制品数据、签名密钥和服务实例 SHALL 具有版本化备份策略与可执行恢复 Operation；仅存在备份文件 SHALL NOT 视为恢复能力完成。

**Traceability:** ART-STO-19, ART-STO-23

#### Scenario: 执行整站恢复演练
- **GIVEN** 测试环境具备最近备份
- **WHEN** 运维触发恢复 Runbook
- **THEN** 平台、市场和制品引用恢复一致
- **AND** 恢复结果包含 RPO/RTO 实测

### Requirement: [OBS-004] 部署档位故障演练
Lite HA、Standard HA 和 Enterprise 档位 SHALL 分别定义单 Pod、单节点、数据库主实例、Registry 和对象存储故障场景，并通过演练验证。

**Traceability:** ART-STO-07, ART-STO-21

#### Scenario: Registry 单副本故障
- **GIVEN** Lite HA 有多个无状态 Registry 副本
- **WHEN** 一个副本被终止
- **THEN** 制品读取持续成功
- **AND** 告警和自动恢复符合档位目标

### Requirement: [OBS-005] 性能预算可验证
内核 API、Read Model、制品传输、Gateway 数据面、Operation 调度、AI 调用和边缘重连 SHALL 定义测试条件、P95/P99、吞吐和容量上限；结果 SHALL 绑定版本和环境。

**Traceability:** METH-04, GOV-05

#### Scenario: 发布前性能门禁
- **GIVEN** 一个候选版本完成压测
- **WHEN** 任一 P95 超过批准预算
- **THEN** 版本不得标记 Production Ready
- **AND** 异常需要变更豁免或修复

### Requirement: [OBS-006] 断连遥测补传
边缘指标和日志 SHALL 使用有界本地缓冲；重连后 SHALL 限速补传并保持原始时间戳，缓存溢出 SHALL 产生丢弃计数。

**Traceability:** EDGE-14

#### Scenario: 边缘离线后恢复
- **GIVEN** 节点离线 24 小时并产生遥测
- **WHEN** 网络恢复
- **THEN** 遥测按限速策略补传
- **AND** 平台区分事件发生时间和接收时间

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

