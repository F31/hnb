## ADDED Requirements

### Requirement: [ENG-001] 节点组定义
平台 SHALL 支持在 EdgeRuntimeTarget 下定义节点组（NodeGroup），包含组名称、节点选择器、标签和描述。

**Traceability:** EDGE-04, EDGE-05

#### Scenario: 创建节点组
- **GIVEN** EdgeRuntimeTarget 已注册
- **WHEN** 用户创建名为 group-a 的节点组并指定节点选择器
- **THEN** 平台保存节点组并关联到目标
- **AND** 节点组出现在目标详情中

### Requirement: [ENG-002] 灰度批次
平台 SHALL 支持在 ExecutionPlan 中按节点组指定灰度批次，包含批次顺序、百分比/数量、健康门禁等待时间和失败容忍度。

**Traceability:** EDGE-04

#### Scenario: 三批次灰度
- **GIVEN** 100 个节点分布在 3 个节点组
- **WHEN** 用户创建 ExecutionPlan 指定 5%/25%/70% 批次
- **THEN** 批次 1 完成后等待健康门禁通过
- **AND** 批次 1 失败时批次 2/3 暂停

### Requirement: [ENG-003] 健康门禁
批次间的健康门禁 SHALL 检查目标节点组中应用实例的 Available 状态、Pod 重启次数和自定义健康端点。

**Traceability:** EDGE-04

#### Scenario: 健康门禁失败
- **GIVEN** 批次 1 完成后健康检查失败
- **WHEN** 门禁超时
- **THEN** 后续批次暂停
- **AND** 失败批次节点自动回滚

### Requirement: [ENG-004] 批次暂停与恢复
平台 SHALL 支持手动暂停/恢复正在进行的灰度批次，以及在失败时自动暂停。

**Traceability:** EDGE-04

#### Scenario: 手动暂停灰度
- **GIVEN** 批次 2 正在执行
- **WHEN** 用户手动暂停
- **THEN** 批次 2 中的进行中节点完成当前步骤
- **AND** 批次 3 不启动
- **AND** 用户可恢复或回滚