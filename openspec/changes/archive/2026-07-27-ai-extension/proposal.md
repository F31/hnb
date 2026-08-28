## Why

平台需要支持 AI Extension Plane 的可选部署，提供 AI Gateway、模型资源管理和 Copilot/AIOps 能力，同时确保 AI 不绕过 Operation 执行路径。

## What Changes

- **AI 平面独立启停**：`cmd/ai-extension/` 服务骨架，可独立安装/升级/停用/卸载
- **无执行旁路**：Copilot 和 AIOps 输出的写操作必须经过 Operation 路径
- **高风险自动化限制**：自动修复支持熔断、冷却、回滚和效果验证

## Capabilities

### New Capabilities
- `ai-extension`: AI 平面独立启停、无执行旁路、高风险自动化限制

## Impact

- **新服务**：`cmd/ai-extension/`（AI Extension Plane 服务骨架）
- **T3 分级**：AI Extension 为 T3 POC/Conformance 后可选
- **不涉及**：不修改 Core 内核、不修改 NATS 主题结构、不修改 Provider 契约