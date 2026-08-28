## ADDED Requirements

### Requirement: [KRP-005] Kubernetes generation CAS 接管
Kubernetes Provider SHALL 在每次 Deployment 副作用前比较目标记录的 generation，并 SHALL 通过 resourceVersion CAS 执行更高 generation 的受控接管。更低 generation SHALL 被拒绝，相同 generation SHALL 仅允许完全相同身份与规格的幂等重放。

**Traceability:** KRP-003, OP-008, RDI-005

#### Scenario: 响应丢失后的接管
- **GIVEN** Deployment 已由较低 generation 创建但 Worker 未收到可提交结果
- **WHEN** 相同逻辑 Step 以更高 generation 重试
- **THEN** Provider CAS 更新 fencing 元数据并观察同一 Deployment
- **AND** 不创建重复资源

### Requirement: [KRP-006] 可 fencing 的逻辑删除
Provider SHALL 要求 delete 输入匹配目标 UID，SHALL 以更高 generation 和 resourceVersion CAS 将 Deployment 缩容为零并保留墓碑，且 SHALL NOT 物理删除作为 fence 的 Deployment。

**Traceability:** KRP-003, OP-008

#### Scenario: 延迟 deploy 在删除后恢复
- **GIVEN** 更高 generation 已提交逻辑删除墓碑
- **WHEN** 较低 generation 的 deploy 延迟到达
- **THEN** Provider 返回 `FENCED`
- **AND** Deployment 不会被重新扩容或重新创建
