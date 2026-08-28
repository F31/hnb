## ADDED Requirements

### Requirement: [OP-009] ExecutionPlan 节点组亲和性
ExecutionPlan SHALL 支持 `node_group_affinity` 字段，指定目标节点组列表。Worker 在路由 Step 时 SHALL 将该字段传递给 Provider，Provider SHALL 将其映射为目标运行环境（Kubernetes 节点标签或 KubeEdge 节点组）的亲和性约束。

**Traceability:** EDGE-04, ENG-001, ERP-005

#### Scenario: 指定节点组的 ExecutionPlan
- **GIVEN** 边缘环境包含 node-group-a 和 node-group-b
- **WHEN** 用户创建 ExecutionPlan 指定 `node_group_affinity: ["node-group-a"]`
- **THEN** 所有 Step 均路由到该节点组
- **AND** 非该节点组的目标不会执行这些 Step