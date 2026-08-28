## ADDED Requirements

### Requirement: [OP-008] Gateway Step 类型
Operation Step SHALL 支持 step_type "configure_gateway"，包含 GatewayProfile 序列化载荷；Worker 执行时 SHALL 调用 Gateway Provider 将 Profile 转为 Gateway API 资源并 Apply 到目标集群。

**Traceability:** GW-01, GW-05

#### Scenario: 灰度发布作为 Operation Step
- **GIVEN** GatewayProfile 配置了 90/10 灰度权重
- **WHEN** Operation Engine 执行 configure_gateway Step
- **THEN** Worker 将 Profile 转为 HTTPRoute 资源
- **AND** 目标集群上 Route 生效且权重正确
