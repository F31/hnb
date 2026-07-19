# edge-pack

## Purpose
定义 Edge Pack 作为 T3 运行扩展的零侵入边界、云边统一执行链、断连自治、批量 OTA、设备接入治理和离线交付行为。

## Requirements

### Requirement: [EDGE-001] Edge Pack 零侵入
未安装 Edge Pack 时 SHALL 不增加 HNB Core 启动依赖、资源占用、菜单和故障面；启用后仍由运行治理平面统一管理。

**Traceability:** EDGE-01

#### Scenario: 标准环境不安装 Edge Pack
- **GIVEN** 执行 T0/T1 全量验收
- **WHEN** 平台完成验收
- **THEN** 所有核心验收通过
- **AND** 无 Edge 相关错误和告警

### Requirement: [EDGE-002] 云边统一执行链
边缘应用 SHALL 来源于市场 Release/CompositionRelease，并经 ExecutionPlan、Operation、权限、策略、审批和审计下发到 NodeGroup。

**Traceability:** EDGE-03, EDGE-05

#### Scenario: 灰度发布边缘应用
- **GIVEN** Release 已声明 edge 兼容性
- **WHEN** 用户选择节点组和批次
- **THEN** 平台创建统一 Operation
- **AND** 批次状态进入 Read Model

### Requirement: [EDGE-003] 断连自治与重连对账
边缘节点 SHALL 按最后已知期望状态在断连期间继续运行和自重启；重连后 SHALL 对账、补传、执行排队 Operation 和撤销处置。

**Traceability:** EDGE-04, EDGE-18

#### Scenario: 节点断网 24 小时
- **GIVEN** 边缘负载已正常运行
- **WHEN** 网络中断后恢复
- **THEN** 负载持续运行且状态最终收敛
- **AND** 收敛时间可观测

### Requirement: [EDGE-004] 批量部署与 OTA
EdgeApplication 和 EdgeCore 升级 SHALL 支持节点组、灰度批次、预检、健康门禁、暂停、失败容忍、回滚和维护窗口。

**Traceability:** EDGE-05, EDGE-08, EDGE-09

#### Scenario: OTA 中某批次失败
- **GIVEN** 100 个节点按 5%/25%/70% 升级
- **WHEN** 首批健康门禁失败
- **THEN** 后续批次暂停且失败节点回滚
- **AND** 已有业务负载保持可用

### Requirement: [EDGE-005] 设备接入治理
Device Mapper SHALL 容器化并通过市场门禁；设备读访问 SHALL 绑定 DeviceModel、权限、审计和站点网络策略；设备写操作还 SHALL 通过 Operation 执行并记录命令审计。

**Traceability:** EDGE-11, EDGE-12

#### Scenario: 向工业设备写入参数
- **GIVEN** 用户拥有只读权限
- **WHEN** 提交设备写请求
- **THEN** 请求被拒绝
- **AND** 越权事件进入审计

### Requirement: [EDGE-006] 入网与离线交付
边缘入网 SHALL 检查 NTP/PTP、架构、磁盘、运行时、证书和网络；纯离线站点 SHALL 支持签名 Bundle 导入本地 Registry。

**Traceability:** EDGE-17, EDGE-20

#### Scenario: 离线站点首次交付
- **GIVEN** 站点无互联网连接
- **WHEN** 现场导入签名 Bundle
- **THEN** 制品、撤销列表和计划通过验证后可部署
- **AND** 全过程保留导入审计
