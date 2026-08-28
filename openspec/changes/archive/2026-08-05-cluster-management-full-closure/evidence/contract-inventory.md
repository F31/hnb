# Contract Inventory: Cluster Management Full Closure

Audit date: 2026-08-01

Scope: OpenSpec task 1.1 only. This is a read-only inventory of current cluster, RuntimeIntent, Operation, observation, PageSchema, and dictionary contracts plus handwritten DTOs. It does not define the replacement contracts and does not change application code or `tasks.md`.

## Classification

- **Retain**: current source contract or domain model that remains useful, usually with additive extension or narrower use.
- **Replace**: handwritten transport/view contract or legacy shape that must be superseded by the source contract required by this change.
- **Generated authority**: generated Go/TypeScript output. It is the only allowed application-facing representation after generation, but its authority is derivative: the source OpenAPI, JSON Schema, or Proto remains normative. Generated files must not be edited by hand.

## Executive Inventory

| Area | Current source authority | Current generated authority | Handwritten DTOs/consumers | Classification |
|---|---|---|---|---|
| Cluster browser API | None under `contracts/openapi/`; handler JSON is de facto contract | None | `cmd/apiserver/internal/handler/resource_cluster.go`; `web/plugins/resource/src/pages/cluster-management/types/cluster.ts`; `web/packages/types/src/models.ts` | Replace with generated OpenAPI types |
| RuntimeIntent | `contracts/openapi/platform/v1/openapi.yaml` plus `contracts/schema/platform/v1/runtime-intent.schema.json` and `contracts/schema/common/v1/secret-reference.schema.json` | `contracts/generated/go/openapi/platform/platform.gen.go`; `contracts/generated/typescript/platform-openapi/` | Three Go copies in apiserver/platform-api plus resource-plugin TypeScript copy | Retain source; regenerate; replace handwritten copies |
| Operation browser API | No Operation paths in platform OpenAPI; platform-api handlers are de facto HTTP contract | No list/detail/action HTTP models | `cmd/platform-api/internal/api/types.go`; `web/packages/types/src/models.ts` | Replace with generated OpenAPI types |
| Operation events | `contracts/schema/messaging/v1/*.schema.json`, registry, and `contracts/proto/hnb/contracts/v1/contracts.proto` | generated Proto Go/TypeScript | worker/store domain structs | Retain event sources; do not use them as browser DTOs |
| RuntimeTarget observation | Only generic EventEnvelope, tunnel heartbeat, direct heartbeat/discovery structs | EventEnvelope generated from Proto only | `pkg/tunnel/types.go`, `cmd/platform-api/internal/store/clusters.go`, `pkg/core/kubeedge.go`, `pkg/core/target.go` | Retain envelope; replace heartbeat/discovery transport as observation authority |
| PageSchema | No OpenAPI/JSON Schema source | None | Go `Page` graph and TypeScript schema-engine graph; local page constants | Replace duplicate Go/page constants with one published schema and generated types; retain renderer runtime concepts |
| Dictionaries | No dictionary schema/source file | None | hardcoded Go response plus hardcoded TypeScript semantic map | Replace with generated dictionary contract and server-owned entries |

## Cluster Contracts

### Current Contracts and DTOs

| Exact path | Current contents | Classification | Disposition |
|---|---|---|---|
| `cmd/apiserver/internal/handler/resource_cluster.go` | Handwritten `clusterCapabilitySnapshot`, `clusterSummary`, `clusterListPayload`, `clusterNode`, and `clusterNodeListPayload`; de facto `/api/v1/resources/clusters` JSON contract | Replace | Define list/detail/node shapes in browser OpenAPI and consume generated Go models in the BFF |
| `web/plugins/resource/src/pages/cluster-management/types/cluster.ts` | Handwritten `ClusterKind`, `ClusterStatus`, `ClusterFreshness`, `CapabilitySnapshot`, `ClusterSummary`, node/list types, and query params | Replace | Derive/import API models and enums from generated TypeScript; keep only genuinely UI-local view types if needed |
| `web/packages/types/src/models.ts` | Older global `Cluster` interface using `id`, `targetType`, and one free-form `status` | Replace | Remove from the cluster-management path after consumers use generated browser models |
| `cmd/platform-api/internal/api/types.go` | Handwritten `createRuntimeTargetRequest`, `runtimeTargetResponse`, and `listRuntimeTargetsResponse` | Replace at HTTP boundary | Domain mapping may remain internal, but HTTP types should be generated |
| `cmd/platform-api/internal/store/store.go` | Persistence/domain `RuntimeTarget` using database-oriented names and one `Status` | Retain as internal during migration | Do not expose as browser contract; extend/normalize separately through database tasks |
| `pkg/core/target.go` | Core `TargetType`, `TargetStatus`, `RuntimeTarget`, and `CapabilitySnapshot` | Retain as internal legacy/domain input | Do not treat as browser or observation schema; four-dimensional projection will supersede single-status transport use |
| `cmd/platform-api/internal/store/clusters.go` | Separate legacy `Cluster` registry and `ClusterHeartbeat` models | Replace for this change's cluster surface | Cluster management must project the two RuntimeTarget kinds rather than expose this parallel cluster model |
| `cmd/platform-api/internal/service/cluster_service.go` | A third handwritten `Cluster` service response | Replace | Supersede the direct cluster CRUD response with generated RuntimeTarget cluster Read Model contracts |
| `staging/api/types.go` | Staging copies of `RuntimeTarget` and `Cluster` | Replace/not authoritative | Must not become a source for the new browser contract |
| `pkg/network/types.go` | Network-provider-specific `RuntimeTarget` containing kubeconfig/API server fields | Retain only as provider-internal input pending separate hardening | Explicitly exclude it from cluster browser DTO generation and never expose its sensitive fields |

No cluster list/detail/node path or schema exists in any of:

- `contracts/openapi/platform/v1/openapi.yaml`
- `contracts/openapi/foundation/v1/openapi.yaml`
- `contracts/openapi/app-market/v1/openapi.yaml`

Therefore the active handler and TypeScript interfaces, not a source contract, currently determine wire compatibility.

## RuntimeIntent Contracts

### Source and Generated Artifacts

| Exact path | Role | Classification |
|---|---|---|
| `contracts/schema/platform/v1/runtime-intent.schema.json` | Normative RuntimeIntent JSON Schema; currently includes four cluster intent kinds | Retain and extend in later tasks |
| `contracts/schema/common/v1/secret-reference.schema.json` | Normative shared `SecretReference` | Retain |
| `contracts/openapi/platform/v1/openapi.yaml` | Normative `/v1/runtime-intents` submission/read paths and `RuntimeIntentRecord` | Retain and extend; later browser BFF contract must be explicit |
| `contracts/proto/hnb/contracts/v1/contracts.proto` | Proto copy of RuntimeIntent metadata/spec/kind | Replace as a parallel RuntimeIntent authority unless explicitly required for an internal boundary; currently drifted |
| `contracts/generated/go/openapi/platform/platform.gen.go` | Generated Go OpenAPI models/client | Generated authority; regenerate, never hand-edit |
| `contracts/generated/typescript/platform-openapi/models/RuntimeIntent.ts` | Generated TypeScript alias | Generated authority |
| `contracts/generated/typescript/platform-openapi/models/runtime_intent_schema.ts` | Generated TypeScript RuntimeIntent shape | Generated authority, currently stale |
| `contracts/generated/typescript/platform-openapi/models/RuntimeIntentRecord.ts` | Generated TypeScript response | Generated authority |
| `contracts/generated/typescript/platform-openapi/services/RuntimeIntentsService.ts` | Generated TypeScript service | Generated authority |
| `contracts/generated/typescript/platform-openapi/index.ts` | Generated public exports | Generated authority |
| `contracts/generated/go/proto/hnb/contracts/v1/contracts.pb.go` | Generated Proto Go models | Generated authority for Proto only; not browser HTTP authority |
| `contracts/generated/typescript/proto/hnb/contracts/v1/contracts_pb.ts` | Generated Proto TypeScript models | Generated authority for Proto only; not browser HTTP authority |

### Handwritten Duplicates

| Exact path | Duplicated types | Classification |
|---|---|---|
| `cmd/apiserver/internal/handler/resource_cluster.go` | `bffIntentEnvelope`, `bffIntentMetadata`, `bffIntentSpec`, `bffIntentSecretRef`, `runtimeIntentRecord` | Replace with generated OpenAPI Go models |
| `cmd/platform-api/internal/api/intent_types.go` | `IntentRequest`, `IntentMetadata`, `IntentSpec`, `IntentSecretRef`, `IntentResponse` | Replace HTTP DTOs with generated models; preserve only explicit domain mapping |
| `cmd/platform-api/internal/engine/intent.go` | `RuntimeIntent`, `IntentMetadata`, `IntentSpec`, `SecretReferenceEntry`, duplicated enum validation | Retain only as an internal validated domain model if needed; parsing must be fed by generated/validated contract rather than act as a second wire schema |
| `web/plugins/resource/src/pages/cluster-management/types/cluster.ts` | `ClusterIntentKind`, `SecretReference`, `RuntimeIntentEnvelope`, `RuntimeIntentStatus`, `RuntimeIntentRecord` | Replace with generated TypeScript imports |
| `web/plugins/resource/src/pages/cluster-management/api/clusterApi.ts` | Handwritten builders and generic `post<RuntimeIntentRecord>` instead of generated service/models | Replace contract usage; business-specific builders may remain if they return generated types |

## Operation Contracts

### HTTP/Read Model

| Exact path | Current contents | Classification |
|---|---|---|
| `cmd/platform-api/internal/api/types.go` | Handwritten `stepResponse`, `operationResponse`, `operationSummaryResponse`, `listOperationsResponse`, action and submit requests | Replace at HTTP boundary with OpenAPI-generated types |
| `cmd/platform-api/internal/store/store.go` | Internal `Operation`, `Step`, `OperationSummary`, and `ListQuery` persistence/read-model structs | Retain as internal domain/persistence models |
| `web/packages/types/src/models.ts` | Handwritten, incomplete `Operation` with a status vocabulary different from the platform state machine | Replace with generated browser Operation list/detail models |
| `cmd/operation-worker/internal/engine/step.go` | Worker-internal `Operation` using snake_case JSON fields and worker state enums | Retain as worker-internal; do not expose to browser |

`contracts/openapi/platform/v1/openapi.yaml` currently contains no `/v1/operations` paths or Operation schemas even though `cmd/platform-api/internal/api/server.go` serves list, detail, approve, reject, and cancel routes. There is also no apiserver Operation BFF handler. The current platform handler response is therefore the de facto domain HTTP contract and has no generated browser counterpart.

### Event Contracts

| Exact path | Role | Classification |
|---|---|---|
| `contracts/schema/messaging/v1/state-changed.schema.json` | Operation state transition payload | Retain |
| `contracts/schema/messaging/v1/progress.schema.json` | Non-authoritative Operation progress payload | Retain |
| `contracts/schema/messaging/v1/step-requested.schema.json` | Operation step command payload | Retain |
| `contracts/schema/messaging/v1/step-completed.schema.json` | Operation step completion payload | Retain |
| `contracts/schema/messaging/v1/message-types-registry.json` | Registered Operation message types and size limits | Retain and extend only if event needs change |
| `contracts/proto/hnb/contracts/v1/contracts.proto` | `OperationStateChanged`, `OperationProgress`, step messages, and EventEnvelope payload union | Retain as internal message source |
| `contracts/generated/go/proto/hnb/contracts/v1/contracts.pb.go` | Generated Go event models | Generated authority |
| `contracts/generated/typescript/proto/hnb/contracts/v1/contracts_pb.ts` | Generated TypeScript event models | Generated authority; browsers must not consume NATS events directly |

## Observation Contracts

### Existing Sources and DTOs

| Exact path | Current contents | Classification |
|---|---|---|
| `contracts/schema/common/v1/event-envelope.schema.json` | Generic versioned EventEnvelope with tenant, correlation, aggregate, payload/payloadRef fields | Retain and use as the envelope source |
| `contracts/proto/hnb/contracts/v1/contracts.proto` | Proto EventEnvelope; payload union has no RuntimeTarget observation variants | Retain envelope, extend or map after observation schemas are published |
| `contracts/mappings/outbox-event-envelope.json` | Outbox-to-envelope field mapping | Retain |
| `pkg/tunnel/types.go` | Handwritten tunnel `Message`, `RegisterPayload`, and `HeartbeatPayload`; no generation, sequence, inventory mode, source identity, or tenant in payload | Replace as observation authority; retain tunnel transport mechanics |
| `cmd/cluster-agent/main.go` | Sends only a 30-second empty cluster heartbeat | Replace producer payload with the versioned observation contract in later tasks |
| `cmd/platform-api/internal/store/clusters.go` | Direct `ClusterHeartbeat` DTO | Replace for canonical projection |
| `pkg/core/kubeedge.go` | Handwritten `EdgeNodeInfo`, `KubeEdgeVersionInfo`, and `KubeEdgeDiscoveryResult` | Retain only as CloudCore adapter-internal discovery models; normalize to generated observation models before publication |
| `pkg/core/target.go` | `CapabilitySnapshot` and single observed timestamp | Retain as legacy/internal data input; not observation wire authority |
| `database/postgresql/migrations/048_runtime_target_nodes.sql` | Current node projection table; no tenant column, source UID, generation/sequence, cursor, digest, inventory mode, or tombstone | Retain table as migration base, not contract authority |

There is no RuntimeTarget observation, capability observation, node inventory, or source-reset JSON Schema under `contracts/schema/`. The read-only contract search for `RuntimeTargetObservation`, `observerGeneration`, `inventoryMode`, source-reset names, and the four dictionary IDs returned no matches.

## PageSchema Contracts

| Exact path | Current contents | Classification |
|---|---|---|
| `web/packages/schema-engine/src/types.ts` | Handwritten TypeScript `SchemaEnvelope`, `PageSchema`, endpoint, DataSource, condition, action, and region types | Retain runtime concepts; replace wire authority with generated schema types |
| `cmd/apiserver/internal/application/schema/service.go` | Independent handwritten Go `Page`, endpoint, DataSource, condition, action, and region graph plus one static `cluster-list` fixture | Replace DTO graph with generated PageSchema types and registered source documents |
| `web/plugins/resource/src/pages/cluster-management/schemas/cluster.list.ts` | Local PageSchema constant; comment says it is for a later server-schema switch | Replace as authority; server-published PageSchema must drive rendering |
| `web/plugins/resource/src/pages/cluster-management/schemas/cluster.detail.ts` | Local PageSchema constant and local detail field definitions | Replace as authority; retain only UI composition metadata that belongs in registered components |
| `web/shell/src/pages/SchemaPage.vue` | Fetches `PageSchema` using the handwritten TypeScript generic | Retain behavior; consume generated schema type after publication |

No PageSchema OpenAPI or JSON Schema exists under `contracts/`. The Go and TypeScript definitions already drift: Go allows arbitrary string methods/types/templates and omits TypeScript-only condition fields (`role`, `exists`, `fieldValue`, `notEmpty`, `resourceState`), context requirements, responsive layout, confirm/event/result fields, and several action/DataSource variants. `web/packages/schema-engine/src/types.ts` also declares `EndpointDefinition` twice in the same file.

The served static schema uses ID `cluster-list`, endpoint `/api/v1/clusters`, permission `schema:read`, and columns `id`/`name`; the resource plugin's local schema uses ID `resource.cluster.list`, endpoint `/api/v1/resources/clusters`, permission `cluster:create`, and cluster Read Model fields. These are separate contracts, not generated copies.

## Dictionary Contracts

| Exact path | Current contents | Classification |
|---|---|---|
| `cmd/apiserver/internal/handler/resource_cluster.go` | Handwritten `dictionaryItem`, `statusDictionaryPayload`, and hardcoded eight-entry `resource.cluster.status` aggregate dictionary | Replace with generated dictionary response and server-owned versioned entries |
| `cmd/apiserver/internal/router/router.go` | Serves `GET /api/v1/dictionaries/cluster.status` | Replace route ID/path with the authoritative dictionary contract |
| `cmd/apiserver/internal/middleware/authorization.go` | Authorizes only `/api/v1/dictionaries/cluster.status` | Replace alongside route contract |
| `web/plugins/resource/src/pages/cluster-management/schemas/cluster.status.ts` | Hardcoded duplicate semantic/terminal mapping and client-side `canMutate` policy | Replace; labels and semantic tokens come from server dictionaries, while mutation authorization/policy remains server-owned |
| `web/plugins/resource/src/pages/cluster-management/components/ClusterStatusBadge.vue` | Consumes the local hardcoded mapping | Replace consumption with dictionary DataSource/generated model |
| `web/plugins/resource/src/pages/cluster-management/schemas/cluster.list.ts` | Hardcoded status and kind filter options | Replace with generated enums and dictionaries |

No dictionary JSON Schema exists. Only the compatibility aggregate `resource.cluster.status` is implemented; lifecycle, health, connectivity, and freshness dictionaries do not exist. The route path says `cluster.status`, the payload ID says `resource.cluster.status`, and the change requires four additional `resource.cluster.*` dictionaries.

## Drift Register

1. **Cluster identity drift:** current browser/BFF fields are `clusterId`/`kind` with values `kubernetes`, `edge`, and `container-engine`; the change requires `targetId`/`targetKind` with only `KubernetesTarget` and `EdgeRuntimeTarget`.
2. **Cluster scope drift:** `cmd/apiserver/internal/handler/resource_cluster.go` explicitly maps and filters `container_engine`, and the local UI offers `container-engine`; it must be excluded from cluster results and exact counts.
3. **State drift:** BFF and UI expose one `status`; freshness can overwrite lifecycle as `STALE`. Required authority has independent lifecycle, health, connectivity, and freshness dimensions plus `lastKnownStateAt`.
4. **Pagination/count drift:** the BFF scans at most 2,000 targets then filters/counts in memory. Required contract calls for database filtering, stable ordering, exact count, and pagination.
5. **Node drift:** node DTOs expose `lastHeartbeatAt`, not per-node `observedAt`, `lastKnownStateAt`, freshness, stable source node identity, or tombstone semantics.
6. **RuntimeIntent source/generated drift:** `contracts/schema/platform/v1/runtime-intent.schema.json` includes cluster intent kinds, while committed generated TypeScript `runtime_intent_schema.ts` and Proto `RuntimeIntentKind` include only Release kinds. Generated Go leaves `kind` as `interface{}` and still requires `releaseId`, while the source JSON Schema no longer requires `releaseId`.
7. **RuntimeIntent path drift:** OpenAPI publishes `/v1/runtime-intents`; platform-api code serves `/v1/intents`; apiserver forwards to `/v1/intents`; browser submits `/api/v1/runtime-intents`.
8. **RuntimeIntent shape drift:** the required change uses explicit `targetId`/`targetKind` and STALE risk confirmation, but current source and handwritten DTOs use generic `targetRef`, `scopeRef`, and parameters. `version` from shared SecretReference is omitted by every handwritten cluster copy.
9. **RuntimeIntent response drift:** apiserver's forwarded response sets `semanticDigest` to empty and omits the original `intent`, although OpenAPI marks both required. Its local fallback creates Operation/domain records directly, conflicting with the required BFF boundary.
10. **Proto drift:** Proto duplicates RuntimeIntent as a second source, lacks cluster kinds, and makes `release_id` an ordinary scalar; it cannot represent JSON Schema required/optional semantics exactly.
11. **Operation HTTP drift:** implemented platform list/detail/actions have no OpenAPI source or generated consumer models. `last_state_changed_at` mixes snake_case with camelCase, and browser `Operation` statuses (`running`, `completed`, `approved`, `rejected`) differ from the authoritative ten-state machine.
12. **Operation browser-boundary drift:** no apiserver Operation BFF exists, and current generated platform SDK exports no Operation service/models.
13. **Observation drift:** only generic envelopes and ad hoc heartbeat/discovery DTOs exist; there is no schema version, observer generation, sequence, Full/Delta mode, source reset, payload bounds, or identity-binding payload contract.
14. **PageSchema drift:** there is no source schema/generation. Go, schema-engine TypeScript, served static page, and resource-plugin local pages are independent definitions with incompatible fields, IDs, endpoints, and permissions.
15. **Page execution drift:** local `clusterListSchema` and `clusterDetailSchema` are not imported by the cluster pages; the pages remain custom Vue consumers rather than executing those L2 schemas.
16. **Dictionary drift:** server and client duplicate one aggregate mapping; no four-dimensional dictionaries exist, the route and dictionary ID differ, and client code owns semantic colors and mutation policy.
17. **Permission drift visible in contracts:** cluster frontend uses `cluster:view`, static PageSchema uses `schema:read`, while the change requires `cluster:list/read/create/update/delete`.

## Duplicate DTO Search Evidence

Command executed from repository root:

```bash
rg -n --glob '!contracts/generated/**' --glob '!**/*_test.go' --glob '!**/__tests__/**' --glob '!openspec/**' --glob '*.{go,ts,tsx,vue}' '(^|[[:space:]])(type|interface)[[:space:]]+(RuntimeIntent|RuntimeIntentRecord|IntentRequest|IntentMetadata|IntentSpec|SecretReference|Operation|OperationSummary|operationResponse|operationSummaryResponse|Cluster|ClusterSummary|ClusterListResponse|ClusterNodeListResponse|PageSchema|SchemaEnvelope|EndpointDefinition|DataSourceDefinition|Condition|ConditionTerm)([[:space:]]|$)' cmd pkg web
```

Evidence summary:

- RuntimeIntent family: duplicated in `cmd/platform-api/internal/engine/intent.go`, `cmd/platform-api/internal/api/intent_types.go`, and `web/plugins/resource/src/pages/cluster-management/types/cluster.ts`; the separate lower-case apiserver copies were confirmed by a follow-up targeted search in `cmd/apiserver/internal/handler/resource_cluster.go`.
- Operation family: `cmd/platform-api/internal/store/store.go`, `cmd/platform-api/internal/api/types.go`, `cmd/operation-worker/internal/engine/step.go`, and `web/packages/types/src/models.ts` each define a different Operation shape.
- Cluster family: distinct declarations occur in `cmd/platform-api/internal/store/clusters.go`, `cmd/platform-api/internal/service/cluster_service.go`, `cmd/gslb-controller/internal/store/cluster_store.go`, and `web/packages/types/src/models.ts`; cluster-management adds its own `ClusterSummary` and list/node responses.
- PageSchema family: Go definitions in `cmd/apiserver/internal/application/schema/service.go` duplicate TypeScript definitions in `web/packages/schema-engine/src/types.ts`; `EndpointDefinition` is duplicated twice inside the TypeScript file itself.
- SecretReference family: a cluster-browser copy exists in `web/plugins/resource/src/pages/cluster-management/types/cluster.ts`, while unrelated worker-local copies also exist; the source authority is `contracts/schema/common/v1/secret-reference.schema.json`.

Contract-presence command executed from repository root:

```bash
rg -n --glob 'contracts/schema/**/*.json' --glob 'contracts/openapi/**/*.{yaml,yml,json}' 'RuntimeTargetObservation|observerGeneration|inventoryMode|source-reset|runtime-target\.observed|resource\.cluster\.(lifecycle|health|connectivity|freshness|status)|PageSchema|/api/v1/resources/clusters|/api/v1/operations' contracts
```

Result: no output. This confirms that the requested cluster browser, Operation browser, observation, PageSchema, and dictionary source contracts are absent from current OpenAPI/JSON Schema files; their current shapes are handwritten/de facto contracts.

## Authority Decision for Follow-up Tasks

1. Retain `contracts/openapi/platform/v1/openapi.yaml`, `contracts/schema/platform/v1/runtime-intent.schema.json`, shared SecretReference, EventEnvelope, Operation event schemas/registry, and the contract generation pipeline as the source foundation.
2. Add cluster, browser RuntimeIntent, Operation, observation, PageSchema, and dictionary source contracts only in their designated later tasks; this audit does not pre-empt those designs.
3. Regenerate Go and TypeScript exclusively through `scripts/generate-contracts.mjs`; generated directories are derivative authority and must pass its `--check` clean-diff mode.
4. Replace handwritten HTTP/browser DTOs listed above with generated models. Keep persistence, worker, adapter, and renderer-domain types only where they are intentionally internal and mapped at a contract boundary.
5. Do not promote Proto RuntimeIntent, legacy `clusters` models, tunnel heartbeat payloads, local PageSchema constants, or local dictionary maps into competing authorities.
