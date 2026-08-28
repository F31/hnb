## ADDED Requirements

### Requirement: [KERNEL-005] Gateway 不编译进内核
Gateway 控制面（GatewayProfile 管理、能力协商、多租户）作为内核逻辑层但仍在内核进程内；具体 Gateway Provider（Profile→YAML 转换 + K8s Apply）SHALL 作为独立 CapabilityPack 部署，NOT 编译进 HNB Core。

#### Scenario: 卸载 Gateway Provider
- **GIVEN** 环境未部署任何 Gateway Provider
- **WHEN** 内核启动
- **THEN** 内核不加载 Gateway 相关执行逻辑
- **AND** StepType "configure_gateway" 的 Step 无 Provider 可用时进入 Failed
