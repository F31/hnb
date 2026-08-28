## Context

SecretReference 表（migration 007）和 SecretReferenceMsg proto 已存在。Operation Engine（operation-engine-core）执行 Step 时需要通过 SecretReference 解析真实凭据。本 change 补全配置解析层和 Secret/KMS Provider 抽象。

## Goals / Non-Goals

**Goals:**
- 5 层配置覆盖：default → tier → environment → tenant → instance
- 不可变 ConfigSnapshot，每次生效生成 digest
- SecretResolver 将 SecretReference 转为具体值
- KMS Provider 接口 + 本地 AES-GCM 实现
- StepExecutor 集成：执行前解析 inputs
- 审计脱敏

**Non-Goals:**
- 边缘 Secret 落盘加密（CFG-004，单独 edge-pack）
- Vault/HSM 生产级集成（实现桩接口用于测试和替换验证）

## Decisions

### Decision 1: Config Layer Priority
```
Instance (最高优先级, 5)
  └── Tenant (4)
       └── Environment (3)
            └── Tier (2)
                 └── Default (最低优先级, 1)
```

最终配置 = 从 Default 向上叠加，高层覆盖低层同名键。

### Decision 2: ConfigSnapshot
```sql
CREATE TABLE config_snapshots (
    id UUID PRIMARY KEY,
    entity_type TEXT NOT NULL,   -- 'release', 'environment', 'tenant'
    entity_id TEXT NOT NULL,
    config_digest TEXT NOT NULL UNIQUE,
    layers_used JSONB,
    snapshot JSONB NOT NULL,
    superseded_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Decision 3: KMS Provider Interface
```go
type KMSProvider interface {
    Name() string
    Resolve(ctx context.Context, ref SecretReference) ([]byte, error)
    Health(ctx context.Context) error
}
```

内置 Provider:
- `LocalAESProvider` — AES-256-GCM，密钥来自环境变量 `HNB_MASTER_KEY`
- `K8sSecretProvider` — 从 Kubernetes Secret 读取（可选依赖 client-go）
- `VaultProvider` — 桩实现（返回模拟值，验证接口可替换性）

### Decision 4: StepExecutor Integration
Operation Engine 的 StepExecution 流程中增加：
1. 从 StepSpec.Inputs 检测 SecretReference 模式
2. 调用 SecretResolver 解析
3. 替换 inputs 中的引用为解析后值（仅内存，不持久化）
4. 审计记录脱敏

### Decision 5: Desensitization
审计日志中 `database_password`, `api_key`, `token`, `secret` 等字段自动替换为 `***REDACTED***`。
