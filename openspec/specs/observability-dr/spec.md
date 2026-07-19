# observability-dr

## Purpose
定义跨平面的指标、日志、链路与事件上下文，以及 Operation SLO、备份恢复、部署档位故障演练、性能预算和断连补传行为。

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
