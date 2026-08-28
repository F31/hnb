# Tasks: gateway-engine

## Summary
| | |
|---|---|
| **Change** | `gateway-engine` |
| **Created** | 2026-07-21 |
| **Specs** | gateway (GW-001~GW-006), composition-operation (OP-008), platform-kernel (KERNEL-005) |
| **Status** | Implemented |

## Task List

### T1: Database Migration
- [x] 012_gateway_engine.sql: gateway_classes, gateways (4 types), gateway_profiles (digest verified), http_routes (status tracking), reference_grants (cross-namespace auth), gateway_capability_snapshots

### T2: Gateway API Resource Models (GW-001/GW-003)
- [x] GatewayClass, Gateway, Listener, TLSConfig
- [x] HTTPRoute, HTTPRouteRule, MatchCriteria, WeightedBackend, HTTPFilter
- [x] ReferenceGrant (cross-namespace authorization)

### T3: GatewayProfile (GW-005)
- [x] ProfileRule with matches, backends, mirror, rewrite, redirect, headers, timeout
- [x] ProfileValidator: name required, backend/redirect exclusivity, multi-backend weights, AI/Mesh require TLS

### T4: GatewayCapabilitySnapshot + Compatibility Check (GW-003)
- [x] CapabilityChecker: route type support, core feature support, extended feature support
- [x] Standard core features (7) and extended features (7) catalog

### T5: Multi-Tenant Validation (GW-004)
- [x] CrossNamespaceRef: same namespace auto-allow, ReferenceGrant lookup
- [x] AllowedRoutes: SameNamespace, Selector, All, default
- [x] Inactive grant rejection

### T6: Traffic Tier Validation (GW-006)
- [x] 4 allowed combinations (app→standard, app→api, api→api, api→standard, mesh→mesh, ai→ai)
- [x] Rejection with reason

### T7: Operation Engine Bridge (OP-008)
- [x] ToStepSpec: StepType "configure_gateway"
- [x] ValidateAndPrepare: Profile + Capability + MultiTenant + Tier

### T8: Unit Tests
- [x] 26 tests covering: profile validation, capability check, multi-tenant auth, tier validation, bridge integration
