## Context

Gateway 是平台流量入口的控制面，数据面由现有 Gateway Controller（Istio / Envoy Gateway / APISIX）实现。HNB Core 管理用户流量配置、能力协商和多租户隔离，通过 Operation Engine 的 `configure_gateway` Step 下发到目标集群。

## Goals / Non-Goals

**Goals:**
- Gateway API 资源模型（GW-001）
- GatewayProfile 流量治理规则（GW-005）
- GatewayCapabilitySnapshot + 兼容性预检（GW-003）
- 多租户隔离（GW-004）
- 流量分层（GW-006）
- Operation Engine 集成

**Non-Goals:**
- 实现数据面 Gateway Controller（第三方已有 Istio/Envoy Gateway 等）
- Ingress 兼容迁移（存量工具）
- Gateway 数据面性能基准

## Architecture

```
User (Portal/CLI)
  │  GatewayProfile
  ▼
HNB Core Control Plane
  ├── GatewayProfile CRUD + validation
  ├── GatewayCapabilityChecker
  ├── MultiTenantValidator (allowedRoutes, ReferenceGrant)
  ├── TrafficTierValidator
  └── Operation Engine → StepType "configure_gateway"
        │
        ▼
Gateway Provider (Go CapabilityPack)
  │  GatewayProfile → Gateway API YAML
  ▼
K8s API → Gateway Controller → Data Plane
```

## Decisions

### Decision 1: GatewayProfile
GatewayProfile 是面向用户的流量配置抽象，不直接暴露 Gateway API 原始资源：

```go
type GatewayProfile struct {
    Listeners []Listener
    Rules     []RoutingRule
    TLS       *TLSConfig
    Tier      string // "standard", "api_management", "mesh", "ai"
}

type RoutingRule struct {
    Name     string
    Match    MatchCriteria
    Backends []WeightedBackend
    Mirror   *MirrorTarget
    Rewrite  *RewriteRule
    Redirect *RedirectRule
    Headers  HeaderModifier
    Timeout  Duration
}
```

### Decision 2: GatewayCapabilitySnapshot
```go
type GatewayCapabilitySnapshot struct {
    ProviderName     string
    SupportedRoutes  []string // "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute"
    CoreFeatures     []string
    ExtendedFeatures []string // "RequestMirror", "WeightedSplit", "URLRewrite"...
}
```

### Decision 3: 预检流程
1. ReleaseManifest 包含 gateway_requirements（需要哪些 Route 类型 + Extended 特性）
2. CompatibilityChecker 比较 requirements vs GatewayCapabilitySnapshot
3. 不兼容 → ExecutionPlan 拒绝

### Decision 4: 执行流程
1. Operation Engine 执行 `configure_gateway` Step
2. GatewayBridge 将 GatewayProfile 转为 Route 模型的序列化表示
3. Gateway Provider 消费该表示并 Apply 到目标 K8s 集群
