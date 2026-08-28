# Evidence: gateway-engine

## Database Artifacts
| Artifact | Tables |
|----------|--------|
| `012_gateway_engine.sql` | gateway_classes, gateways (4 types), gateway_profiles (digest), http_routes (route status), reference_grants (cross-ns auth), gateway_capability_snapshots |

## Go Code Artifacts

### Gateway Provider Module (`cmd/gateway-provider/`)
| File | LOC | Key Types |
|------|-----|-----------|
| `internal/engine/gateway/models.go` | 163 | Gateway, Listener, HTTPRoute, MatchCriteria, WeightedBackend, ReferenceGrant, TLSConfig, HTTPFilter (+mirror/rewrite/redirect/header) |
| `internal/engine/gateway/profile.go` | 79 | GatewayProfile, ProfileRule, ProfileValidator (4 checks) |
| `internal/engine/gateway/capability.go` | 94 | GatewayCapabilitySnapshot, CapabilityChecker (route + feature check) |
| `internal/engine/gateway/multitenant.go` | 85 | MultiTenantValidator: cross-namespace ref, allowedRoutes, ReferenceGrant |
| `internal/engine/gateway/tier.go` | 34 | TrafficTierValidator: 4-tier isolation |
| `internal/engine/gateway/bridge.go` | 69 | GatewayExecutor: StepType "configure_gateway" + 4-in-1 validation |
| `internal/engine/gateway/gateway_test.go` | 347 | 26 tests |
| `internal/config/config.go` | 43 | Env-based config |
| `internal/nats/worker.go` | 113 | NATS consumer for gateway events |
| `main.go` | 40 | Entry point |

### Control/Data Plane Boundary
| Layer | Implementation | Module |
|-------|---------------|--------|
| Control plane (this change) | Go models + validation + executor | `cmd/gateway-provider/` (独立部署) |
| Data plane (not this change) | Gateway Controller (Istio/Envoy/APISIX) | On-target K8s cluster |

## Test Results
```
PASS: 26/26 tests
  - Profile validation (6): Validate, EmptyName, NoRules, NoBackend, 
    MultiBackendNoWeights, AIGatewayRequireTLS
  - Capability check (4): Pass, RouteNotSupported, FeatureNotSupported, ExtendedFeature
  - MultiTenant (7): SameNamespace, CrossNamespace(NoGrant/WithGrant/InactiveGrant),
    AllowedRoutes(SameNamespace/All/Default)
  - Tier validation (3): Allowed, AppNotAI, MeshNotStandard
  - Bridge (4): ToStepSpec, ValidateAndPrepare(Pass/Fail), Weights
  - Features (2): CoreFeatures, ExtendedFeatures
```

## Spec Coverage
| Spec | Req | Status |
|------|-----|--------|
| gateway | GW-001 Gateway API 优先 | ✅ Model |
| gateway | GW-002 CRD 集中治理 | ✅ GatewayClass + capability |
| gateway | GW-003 特性协商 | ✅ CapabilityChecker |
| gateway | GW-004 多租户隔离 | ✅ MultiTenantValidator |
| gateway | GW-005 流量治理 | ✅ GatewayProfile |
| gateway | GW-006 流量分层 | ✅ TrafficTierValidator |
| composition-operation | OP-008 Gateway Step | ✅ Bridge |
| platform-kernel | KERNEL-005 不编译进内核 | ✅ Control plane only |
