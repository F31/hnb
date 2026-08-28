# Evidence: app-market-engine

## Database Artifacts
| Artifact | Tables |
|----------|--------|
| `011_app_market_engine.sql` | publishers, products, packages, artifacts, releases (immutable digest), channels (5-type pipeline), entitlements (4 tiers), subscriptions (tenant→product) |

## Go Code Artifacts

### Market Engine (`cmd/app-market/internal/engine/market/`)
| File | LOC | Key Types |
|------|-----|-----------|
| `models.go` | 175 | Publisher, Product, Package, Artifact, Release, Channel, Entitlement, Subscription, ReleaseManifest |
| `release.go` | 68 | ReleaseManager: CreateRelease, PublishRelease, ValidateManifest |
| `channel.go` | 72 | ChannelPipeline: 5-type promotion with 8 valid edges |
| `entitlement.go` | 81 | EntitlementChecker: CheckAccess, CheckDeploymentLimit |
| `bridge.go` | 107 | ManifestBridge: ReleaseManifest → ExecutionPlan |
| `plan.go` | 206 | ExecutionPlan, StepSpec, RetryPolicy, PolicyResult, PlanGenerator, DAGResolver (standalone, no engine dep) |
| `market_test.go` | 310 | 20 tests |

### Integration Points
- `ManifestBridge.ToExecutionPlan()` → self-contained PlanGenerator + DAGResolver (no external engine dependency)
- Released manifest digest → ExecutionPlan plan_digest
- EntitlementChecker → used as policy hook before Operation creation
- Communicates via NATS (hnb.market.release → hnb.market.processed)

## Test Results
```
PASS: 20/20 tests
  - Channel promotion (4): CanPromote, Promote, PromoteInvalid, GetReleaseForChannel
  - Release management (5): CreateRelease, MissingVersion, PublishRelease, PublishNonDraft, ValidatePass
  - Validation (2): ValidateManifest, ValidatePass
  - Entitlements (4): CheckAccess, NoSub, InactiveSub, CheckDeploymentLimit
  - Manifest Bridge (3): ToExecutionPlan, EmptyManifest, PolicyFail
  - Model (2): ChannelOrder, ProductCategory, PublisherStatus
```

## Spec Coverage
| Spec | Req | Status |
|------|-----|--------|
| app-market | MKT-001 独立数据隔离 | ✅ 独立表/独立模型 |
| app-market | MKT-002 统一产品发布 | ✅ 8 entity model |
| app-market | MKT-003 不可变与渠道 | ✅ Release digest + 5-type channel |
| app-market | MKT-004 不保存凭据 | ✅ Bridge 不处理 Secret |
| app-market | MKT-005 发布门禁 | ✅ Manifest validation + policy hook |
| composition-operation | OP-001 Release→Plan | ✅ ManifestBridge |
