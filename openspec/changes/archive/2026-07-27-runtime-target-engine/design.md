## Context

Operation Engine 需要将 `StepSpec.ProviderID` 解析为具体的 RuntimeTarget + 执行 Provider。config-secret engine 提供了 KMS Provider 注册，而 RuntimeTarget 提供运行目标分类和能力兼容性。

## Goals / Non-Goals

**Goals:**
- 4 类 RuntimeTarget 统一模型
- CapabilitySnapshot 版本化存储
- ProviderRegistry 将 ProviderID → RuntimeTarget + Provider
- CompatibilityChecker 预检
- FreshnessTracker → Operation 状态机集成

**Non-Goals:**
- Agent 长连接实现（RT-002；属 agent 开发）
- 具体 CNI/CSI/GPU 探测逻辑（属 Provider 实现）
- mTLS 隧道（属网络层）

## Decisions

### Decision 1: RuntimeTarget 分类

```go
type TargetType string
const (
    TargetKubernetes      TargetType = "kubernetes"
    TargetContainerEngine TargetType = "container_engine"
    TargetEdgeRuntime     TargetType = "edge_runtime"
    TargetExternalService TargetType = "external_service"
)
```

### Decision 2: CapabilitySnapshot

```go
type CapabilitySnapshot struct {
    ID            string
    TargetID      string
    KubeVersion   string
    Arch          string
    CPU           CPUInfo
    MemoryMB      int64
    StorageGB     int64
    CNIPlugins    []string
    CSIDrivers    []string
    GatewayAPI    string      // supported version
    GPU           GPUInfo
    Features      map[string]bool
    ObservedAt    time.Time
}
```

### Decision 3: ProviderRegistry

```
ProviderID "k8s-prod-01" ──→ { target: "KubernetesTarget(uid-xxx)", provider: "k8s-deploy" }
ProviderID "edge-node-05" ──→ { target: "EdgeRuntimeTarget(uid-yyy)", provider: "edge-deploy" }
```

### Decision 4: Freshness → Operation 集成
- `observedAt` < now - threshold → target 标记 stale
- 写操作 → Operation 进入 QueuedOffline（已有状态机支持）
