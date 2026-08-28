## Why

当前已有发布模型（Market）、执行引擎（Operation Engine）、运行目标（RuntimeTarget），但缺少流量入口管理。Gateway 作为平台流量治理的控制面，管理 GatewayProfile、能力协商、多租户隔离、流量分层，并通过 Operation Engine 的 StepType `configure_gateway` 写入目标集群。

## What Changes

- Gateway API 资源模型：GatewayClass / Gateway / HTTPRoute / GRPCRoute / ReferenceGrant / BackendTLSPolicy
- GatewayProfile：用户流量配置抽象（匹配、分流、镜像、重写、重定向、Header、TLS、超时）
- GatewayCapabilitySnapshot：Provider 声明的 Route 类型与 Core/Extended 特性
- 兼容性预检：Release 要求的 Gateway 特性 vs Provider 能力
- 多租户验证：allowedRoutes / ReferenceGrant / NamespaceSelector
- 流量分层检查：普通 / API Management / Mesh / AI Gateway 不混用
- Operation Engine 集成：StepType `configure_gateway`

## Capabilities

- `gateway`: GW-001~GW-006 全部实现
- `composition-operation`: 新增 StepType `configure_gateway`
- `platform-kernel`: Gateway 作为 Provider/CapabilityPack 不编译进内核

## Impact

- **代码**: Gateway 模型、GatewayProfile、能力协商、多租户、分层、Operation Bridge
- **数据**: 6 张新表
- **事件**: 无新事件类型（复用 Operation StateChanged）
- **集成**: StepExecutor 的 Provider 类型新增 `configure_gateway`
