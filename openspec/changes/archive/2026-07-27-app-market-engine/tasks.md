# Tasks: app-market-engine

## Summary
| | |
|---|---|
| **Change** | `app-market-engine` |
| **Created** | 2026-07-21 |
| **Specs** | app-market (MKT-001~MKT-005), composition-operation (OP-001), platform-kernel (MKT-004-boundary) |
| **Status** | Implemented |

## Task List

### T1: Database Migration — Market Tables
- [x] 011_app_market_engine.sql: publishers, products, packages, artifacts, releases (immutable digest), channels (promotion pipeline), entitlements (access control), subscriptions (tenant→product)
- [x] Rollback SQL

### T2: Market Data Models (MKT-002)
- [x] Publisher (3 statuses), Product (7 categories), Package (6 types), Artifact (5 types), Release (immutable)
- [x] ReleaseManifest model linking to Operation Engine

### T3: Release Manager (MKT-003)
- [x] CreateRelease — versioned, digest-verified
- [x] PublishRelease — draft → published
- [x] ValidateManifest — schema checks (artifacts must have digests)

### T4: Channel Promotion Pipeline (MKT-003)
- [x] 5 channel types: dev→staging→stable→deprecated→withdrawn
- [x] Valid promotion matrix (8 edges)
- [x] ChannelPipeline with Promote / GetReleaseForChannel

### T5: Entitlement & Subscription (MKT-002)
- [x] EntitlementChecker: CheckAccess, CheckDeploymentLimit
- [x] 4 entitlement tiers: evaluate, standard, premium, enterprise

### T6: ReleaseManifest → ExecutionPlan Bridge (MKT-004 / OP-001)
- [x] ManifestBridge: converts ReleaseManifest → ExecutionPlan
- [x] Step generation from packages with artifact binding
- [x] Dependency graph construction from manifest dependencies
- [x] PolicyResult integration
- [x] Manifest never contains plaintext credentials

### T7: Unit Tests
- [x] 20 tests covering: channel promotion, release CRUD, manifest validation, entitlement access, deployment limits, execution plan bridge
