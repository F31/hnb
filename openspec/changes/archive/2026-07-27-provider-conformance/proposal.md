## Why

当前 HNB 平台已有 10+ Provider（kubernetes、edge、gateway、config-secret、gslb 等），但缺乏统一的 Provider 兼容性认证框架。Provider 升级可能引入行为不兼容，导致生产故障。需要一个 conformance 测试框架来验证 Provider 满足标准契约，确保平台稳定性和 Provider 生态质量。

## What Changes

- **新增 Provider Manifest 注册校验**：Provider 注册时验证 Manifest 完整性（名称、版本、协议、Capability、生命周期动作、权限、资源需求、依赖、兼容范围）
- **新增 Conformance 测试框架**：`cmd/provider-conformance/` CLI 工具，支持契约测试、功能测试、故障测试、安全测试和性能基线
- **新增版本兼容矩阵**：维护 HNB Core ↔ Provider ↔ RuntimeTarget 版本兼容性关系，阻止不兼容组合
- **新增 Provider 认证状态**：Provider 注册时带 `conformance_level` 字段（none/basic/production_ready），认证过期后自动降级

## Capabilities

### New Capabilities
- `provider-conformance`: Provider Manifest 注册校验、Conformance 测试框架、版本兼容矩阵、认证生命周期管理

### Modified Capabilities
- `composition-operation`: Provider 调用前检查 conformance 状态，未认证或已过期 Provider 阻止执行
- `runtime-target`: RuntimeTarget 注册时验证 Provider 兼容性

## Impact

- **新服务**：`cmd/provider-conformance`（CLI 认证测试工具）
- **新依赖**：无新中间件，复用现有 PostgreSQL + NATS
- **数据库**：新增 `provider_manifests` 表 + `provider_conformance` 表
- **T2 分级**：Provider Conformance 为 T2 标准可选能力
- **不涉及**：不修改 Portal、不修改 Operation 状态机、不修改 NATS 主题结构