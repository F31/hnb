## MODIFIED Requirements

### Requirement: [OP-001] 不可变 ExecutionPlan
平台 SHALL 将 ReleaseManifest 或 CompositionRelease 解析为不可变 ExecutionPlan，固定目标、Artifact digest、参数、SecretReference、Provider 版本、策略结果和步骤 DAG。PlanGenerator SHALL 接受 ReleaseManifest 作为输入并自动转换为 ExecutionPlan。

#### Scenario: 市场 Release 生成执行计划
- **GIVEN** 市场创建了一个 ReleaseManifest
- **WHEN** PlanGenerator 解析该 Manifest
- **THEN** 生成包含全部 Artifact digest、Package 依赖和配置覆盖的 ExecutionPlan
- **AND** 市场不直接参与 ExecutionPlan 解析过程
