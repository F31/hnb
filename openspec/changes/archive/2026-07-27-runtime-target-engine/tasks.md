# Tasks: runtime-target-engine

## Summary
| | |
|---|---|
| **Change** | `runtime-target-engine` |
| **Created** | 2026-07-21 |
| **Specs** | runtime-target (RT-001~RT-005), composition-operation (OP-001), platform-kernel (KERNEL-001) |
| **Status** | Implemented |

## Task List

### T1: Database Migration — RuntimeTarget Tables
- [x] 010_runtime_target_engine.sql: runtime_targets (4 types, connection_type, status, staleness), capability_snapshots (versioned), provider_registry (provider_id → target)
- [x] Rollback SQL

### T2: RuntimeTarget Model (RT-001)
- [x] 4 classifications: KubernetesTarget, ContainerEngineTarget, EdgeRuntimeTarget, ExternalServiceConnector
- [x] IsDeployable() — ExternalServiceConnector returns false
- [x] IsStale() — observedAt + threshold check

### T3: ProviderRegistry (OP-001 / KERNEL-001)
- [x] RegisterProvider / UnregisterProvider
- [x] GetProvider / GetTarget
- [x] ResolveStepProvider(providerID) → (ProviderEntry, RuntimeTarget, error)
- [x] Active/inactive state enforcement

### T4: CapabilitySnapshot (RT-003)
- [x] Capability model: version, arch, CPU, memory, storage, CNI, CSI, GPU, features
- [x] ResourceRequirement model for manifest requirements

### T5: CompatibilityChecker (RT-004)
- [x] Memory, CPU, storage preflight
- [x] GPU availability check
- [x] CNI plugin presence check
- [x] Kubernetes version comparison
- [x] Feature flag support check
- [x] Multiple issues aggregation

### T6: FreshnessTracker (RT-005)
- [x] Per-target-type policies (edge: 2m stale → queue_offline)
- [x] Evaluate(target) → (ok, action)
- [x] Custom policy override

### T7: Unit Tests
- [x] 25 tests covering: target classification, deployable check, stale detection, registry CRUD, step resolution, compatibility checks (memory/CPU/storage/GPU/CNI/version/features), freshness tracking
