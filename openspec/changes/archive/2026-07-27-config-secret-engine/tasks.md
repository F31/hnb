# Tasks: config-secret-engine

## Summary
| | |
|---|---|
| **Change** | `config-secret-engine` |
| **Created** | 2026-07-21 |
| **Specs** | config-secret (CFG-001~CFG-003, CFG-005), composition-operation (OP-004) |
| **Status** | Implemented |

## Task List

### T1: Database Migration — Config/Secret Tables
- [x] 009_config_secret_engine.sql: config_layers (5 layers with priority), config_values, config_snapshots (immutable digest), kms_providers (pluggable registry), +provider_id on secret_references
- [x] Rollback SQL

### T2: Config Layering Engine (CFG-001)
- [x] Layer model with 5 priorities (default=1 → instance=5)
- [x] ConfigResolver — multi-layer merge with priority overwrite
- [x] ConfigSnapshot — immutable resolved config with SHA-256 digest
- [x] ComputeSnapshotDigest — deterministic hash for rollback

### T3: KMS Provider Interface + Implementations (CFG-003)
- [x] KMSProvider interface: Name(), Type(), Resolve(), Health()
- [x] LocalAESProvider — AES-256-GCM encrypt/decrypt with HNB_MASTER_KEY
- [x] K8sSecretProvider — pluggable K8s Secrets integration
- [x] VaultProvider — pluggable Vault integration (simulated)
- [x] Registry — provider registration/lookup/health

### T4: SecretResolver (CFG-002)
- [x] SecretResolver — route SecretReference to KMSProvider by provider_id
- [x] SecretReference parsing with ref://secrets/ URI scheme
- [x] Error handling for missing providers, empty values

### T5: Step Inputs Resolution (CFG-005 / OP-004)
- [x] ResolveStepInputs — resolve all ref://secrets/ in step inputs before execution
- [x] Desensitization — sensitive key patterns auto-redacted in audit
- [x] Audit-safe resolved inputs map (refs preserved, plaintext redacted)

### T6: Unit Tests
- [x] 26 tests covering: layer priority, config resolution, snapshot digest, sensitive key detection, secret ref parsing, AES encrypt/decrypt, K8s/Vault providers, registry, secret resolver, step input resolution
