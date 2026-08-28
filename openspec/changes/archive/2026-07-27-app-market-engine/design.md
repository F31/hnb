## Context

三个前置 change 已实现执行路径：Operation Engine（状态机 + DAG）、Config/Secret（凭据解析）、RuntimeTarget（运行目标）。App Market 补齐"发布什么"——产品模型和发布管线。

## Goals / Non-Goals

**Goals:**
- Publisher/Product/Package/Artifact 统一模型
- Release 不可变 + Channel 晋级
- ReleaseManifest → ExecutionPlan 转换桥
- Entitlement/Subscription 授权检查

**Non-Goals:**
- 市场 UI/Portal（由 portal-experience 覆盖）
- ISV 沙箱实现（MKT-005 后半部分，运行时隔离）
- 独立市场部署拓扑（MKT-001；架构约束而非代码）

## Decisions

### Decision 1: Entity Model

```
Publisher (1) → (N) Product
Product (1) → (N) Package
Package (1) → (N) Artifact (OCI/Helm/container image)
Product (1) → (N) Release (immutable, versioned)
Release (1) → (N) Channel (promotion states)
Product (1) → (N) Entitlement (who can use)
Tenant (1) → (N) Subscription (which product)
```

### Decision 2: ReleaseManifest → ExecutionPlan Bridge

Market Release → ReleaseManifest JSON → PlanGenerator.GeneratePlan() → ExecutionPlan

```go
type ReleaseManifest struct {
    ReleaseID    string
    ProductID    string
    Version      string
    Packages     []PackageRef
    Artifacts    []ArtifactRef
    Dependencies []DependencySpec
    Config       map[string]ConfigEntry
}
```

### Decision 3: Channel State Machine

```
dev ──→ staging ──→ stable ──→ deprecated ──→ withdrawn
  ↕          ↕          ↕
  ←── rollback ──┘
```

### Decision 4: Market Never Stores Credentials
市场仅存储 ReleaseManifest + Entitlement context。运行凭据由平台侧 Operation Engine + SecretResolver 独立处理。
