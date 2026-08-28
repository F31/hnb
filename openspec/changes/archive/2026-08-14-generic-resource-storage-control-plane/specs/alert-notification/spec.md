## ADDED Requirements

### Requirement: [ALERT-011] 存储告警资源身份与指标来源
存储 AlertRule 和 AlertInstance SHALL 使用 tenant、target、资源 kind、稳定 UID 与可选 namespace/name 引用后端、Offering、Binding 或 Kubernetes 存储资源；规则 SHALL 声明指标 Adapter、单位、新鲜度和适用 Provider 能力，SHALL NOT 假设所有 CSI 驱动提供容量、IOPS 或延迟指标。

**Traceability:** ALERT-001, ALERT-004, STO-003

#### Scenario: Provider 不提供 IOPS 指标
- **GIVEN** 一个 NFS 后端没有通过 Conformance 的 IOPS Adapter
- **WHEN** 管理员配置 IOPS 延迟告警
- **THEN** 平台将该指标标记为不可用并拒绝保存不可执行规则
- **AND** 容量或连接状态等可用规则不受影响

#### Scenario: PVC Pending 告警跳转
- **GIVEN** 一个 PVC 超过批准的 Pending 时长
- **WHEN** 存储告警 Adapter 创建 AlertInstance
- **THEN** 告警包含 tenant、target、namespace、PVC UID/name 和 Binding/Offering 上下文
- **AND** Portal 可跳转到消费视图、相关 Operation 和 Runbook
