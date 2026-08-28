# Evidence: config-secret-engine

## Database Artifacts
| Artifact | Tables |
|----------|--------|
| `009_config_secret_engine.sql` | config_layers (5 layers, priority 1-5), config_values (key-value per layer), config_snapshots (immutable with digest), kms_providers (pluggable registry), +kms_provider_id on secret_references |

## Go Code Artifacts

### Config Engine (`internal/engine/config/`)
| File | LOC | Key Types |
|------|-----|-----------|
| `layer.go` | 127 | Layer, ConfigResolver (5-layer merge), ConfigSnapshot, ComputeSnapshotDigest |
| `kms.go` | 159 | KMSProvider interface, LocalAESProvider, K8sSecretProvider, VaultProvider, Registry, SecretResolver |
| `integration.go` | 53 | ResolveStepInputs, SecretReference parsing (ref://secrets/ URI) |
| `desensitize.go` | 48 | IsSensitiveKey (10 patterns), Desensitize, DesensitizeMap, ResolveAndDesensitize |
| `config_test.go` | 287 | 26 tests |

### Integration
- StepExecutor in `worker.go` now calls `ResolveStepInputs` before Provider execution
- Audit entries use DesensitizeMap to redact sensitive fields

## Test Results
```
PASS: 26/26 tests
  - Config layering (4): LayerPriority, NewLayer, AddAndResolve, LayerOrder, MultipleKeys
  - Snapshots (2): BuildSnapshot, ComputeSnapshotDigest
  - Desensitization (3): IsSensitiveKey, Desensitize, DesensitizeMap
  - SecretReference (3): IsSecretReference, ParseSecretReference, ParseSecretReference_Plaintext
  - AES Provider (4): EncryptDecrypt, Health, InvalidKey, MissingEnvVar
  - K8s/Vault Providers (2): K8sSecretProvider_Resolve, VaultProvider_Resolve
  - Registry (2): Registry, Registry_Health
  - SecretResolver (2): SecretResolver, WrongProvider
  - Step Inputs (2): Plaintext, SecretRefFails
  - Desensitize Integration (1): ResolveAndDesensitize
```

## Spec Coverage
| Spec | Req | Status |
|------|-----|--------|
| config-secret | CFG-001 配置分层与版本化 | ✅ 5-layer merge + digest |
| config-secret | CFG-002 SecretReference-only | ✅ ref:// URI resolution + audit |
| config-secret | CFG-003 外部 KMS 可替换 | ✅ KMSProvider interface + 3 impls |
| config-secret | CFG-005 Step 执行时配置解析 | ✅ ResolveStepInputs integration |
| composition-operation | OP-004 幂等恢复增强 | ✅ resolution before execution |
