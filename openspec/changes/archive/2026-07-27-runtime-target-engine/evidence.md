# Evidence: runtime-target-engine

## Database Artifacts
| Artifact | Tables |
|----------|--------|
| `010_runtime_target_engine.sql` | runtime_targets (4 types, 5 statuses, staleness), capability_snapshots (versioned, JSON), provider_registry (6 provider types, capability_pack) |

## Go Code Artifacts

### Runtime Engine (`internal/engine/runtime/`)
| File | LOC | Key Types |
|------|-----|-----------|
| `target.go` | 96 | RuntimeTarget, CapabilitySnapshot, ResourceRequirement, IsDeployable, IsStale |
| `registry.go` | 128 | ProviderRegistry, ProviderEntry, CRUD, ResolveStepProvider |
| `compatibility.go` | 104 | CompatibilityChecker, 7 checks (mem/CPU/storage/GPU/CNI/version/features) |
| `freshness.go` | 71 | FreshnessTracker, per-type policies, Evaluate |
| `runtime_test.go` | 305 | 25 tests |

### Integration Points
- `ResolveStepProvider(providerID)` → used by Operation Engine StepExecutor before execution
- `CompatibilityChecker.Check(req, cap)` → used during ExecutionPlan generation
- `FreshnessTracker.Evaluate(target)` → returns `queue_offline` → Operation state machine transition

## Test Results
```
PASS: 25/25 tests
  - Target classification (1): TargetTypeValues
  - RuntimeTarget behavior (2): IsDeployable, IsStale
  - ProviderRegistry (7): Register, RegisterNilTarget, Unregister,
    ResolveStepProvider(ok/notfound/inactive), List
  - CompatibilityChecker (10): Pass, MemoryFail, CPUFail, StorageFail,
    GPU, CNI, KubeVersion, Features, MultipleIssues
  - FreshnessTracker (5): Default, Fresh, EdgeOffline, SetPolicy, UnknownType
  - Version comparison (1): CompareVersions
```

## Spec Coverage
| Spec | Req | Status |
|------|-----|--------|
| runtime-target | RT-001 运行目标分类 | ✅ 4 types + IsDeployable |
| runtime-target | RT-002 主动连接 | ✅ connection_type model |
| runtime-target | RT-003 能力发现与快照 | ✅ CapabilitySnapshot + Registry |
| runtime-target | RT-004 兼容性预检 | ✅ 7 checks |
| runtime-target | RT-005 新鲜度 | ✅ FreshnessTracker → QueuedOffline |
| composition-operation | OP-001 ProviderID 解析 | ✅ ResolveStepProvider |
| platform-kernel | KERNEL-001 Provider Registry | ✅ Registry 纳入内核边界 |
