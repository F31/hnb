## Why

Operation Engine 的每个 Step 包含 `ProviderID`，指明由哪个 Provider/运行目标执行。当前缺少 RuntimeTarget 的统一模型、Provider 注册表、能力发现和兼容性预检。没有这些，Operation Engine 无法将 Step 映射到实际基础设施。

## What Changes

- RuntimeTarget 模型：4 类目标（KubernetesTarget / ContainerEngineTarget / EdgeRuntimeTarget / ExternalServiceConnector）
- CapabilitySnapshot：版本化能力上报（K8s 版本、架构、资源、CNI/CSI、GPU 等）
- ProviderRegistry：将 ProviderID 映射到 RuntimeTarget + 执行 Provider
- CompatibilityChecker：ReleaseManifest vs RuntimeTarget 能力差异比较
- FreshnessTracker：observedAt 新鲜度阈值 → QueuedOffline 状态机集成

## Capabilities

- `runtime-target`: RT-001~RT-005 全部实现
- `composition-operation`: StepSpec.ProviderID → RuntimeTarget 解析
- `platform-kernel`: Provider/Capability Registry 纳入 T0 内核

## Impact
- **代码**: RuntimeTarget model, CapabilitySnapshot, ProviderRegistry, CompatibilityChecker, FreshnessTracker
- **数据**: 新增 runtime_targets, capability_snapshots, provider_registry 表
- **API/事件**: 新增 CapabilityReported, TargetStatusChanged 事件
- **集成**: Operation Engine 的 StepExecutor 通过 ProviderRegistry 获取执行目标
