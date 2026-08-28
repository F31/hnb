## Why

Operation Engine 每个 Step 的 `Inputs` 和 `ProviderID` 依赖配置解析和密钥解析环境。目前 SecretReference 表已在 identity-tenancy 中创建，但缺少:
1. 配置分层覆盖（CFG-001）—— 从默认值到实例的 5 层合并
2. SecretReference-only 边界（CFG-002）—— 确保所有明文路径被 SecretReference 替代
3. 外部 KMS Provider 可替换（CFG-003）—— 抽象接口 + 内置实现

本 change 实现完整的 Config/Secret 引擎，并集成到 Operation Engine 的 StepExecutor 中。

## What Changes

- ConfigSnapshot 模型：不可变版本化配置快照，5 层叠加（default → tier → env → tenant → instance）
- SecretResolver：将 SecretReference 解析为 KMS Provider 返回值
- KMS Provider 接口：内置 AES-256-GCM 本地实现 + K8s Secret Provider + Vault Provider 桩
- StepExecutor 集成：执行 Step 前解析 Inputs 中的配置引用，替换 SecretReference 为解析值
- 敏感字段脱敏：审计/日志路径自动脱敏

## Capabilities

### Modified Capabilities
- `config-secret`: CFG-001（配置分层与版本化）、CFG-002（SecretReference-only）、CFG-003（外部 KMS 可替换）实现；CFG-004（边缘 Secret 保护）保留。
- `composition-operation`: StepExecutor 集成 ConfigResolver + SecretResolver

## Impact
- **代码**: ConfigResolver、SecretResolver、KMS Provider 接口 + 实现、ConfigStore
- **数据**: 新增 config_snapshots、config_values、config_layers、kms_providers 表
- **API/事件**: 新增 SecretResolved 事件
- **依赖**: 复用 PostgreSQL；K8s Provider 依赖 client-go（可选）
