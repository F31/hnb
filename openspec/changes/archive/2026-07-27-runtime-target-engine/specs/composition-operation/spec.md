## MODIFIED Requirements

### Requirement: [OP-001] 不可变 ExecutionPlan
平台 SHALL 将 ReleaseManifest 或 CompositionRelease 解析为不可变 ExecutionPlan，固定目标、Artifact digest、参数、SecretReference、Provider 版本、策略结果和步骤 DAG。执行 Step 前 SHALL 通过 ProviderRegistry 将 StepSpec.ProviderID 解析为 RuntimeTarget 并执行兼容性检查。

**Traceability:** CMPOS-03, INT-03

#### Scenario: Step 引用已注销 Provider
- **GIVEN** ExecutionPlan 引用 provider_id: "k8s-prod-01"
- **WHEN** Worker 准备执行并查询 ProviderRegistry
- **THEN** 若 Provider 已注销则拒绝执行并返回错误
- **AND** Operation 进入 Failed 终态
