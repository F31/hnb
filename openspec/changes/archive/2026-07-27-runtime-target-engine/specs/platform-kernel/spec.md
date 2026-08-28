## MODIFIED Requirements

### Requirement: [KERNEL-001] 最小内核边界
HNB Core SHALL 仅包含身份与租户上下文、Operation Engine、ExecutionPlan Engine、Read Model、Resource Graph、Provider/Capability Registry、Policy Hook、Audit 与 Agent Gateway；具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和边缘实现 SHALL NOT 编译进入内核。Provider/Capability Registry SHALL 是 RuntimeTarget 类型注册和 capability 快照的权威存储。

**Traceability:** CTN-01, AI-ARCH-01, ART-STO-24

#### Scenario: 内核不包含具体 Provider 实现
- **GIVEN** ProviderRegistry 注册了 K8sTarget 和 EdgeTarget 两类目标
- **WHEN** 内核启动
- **THEN** 注册表可用但不包含具体 CNI/CSI/数据库部署实现
- **AND** 具体实现由 CapabilityPack 扩展提供
