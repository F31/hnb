# HNB Cloud Phase 0 Project-Truth Evidence Baseline

## 1. Capture Identity

| Field | Captured value |
|---|---|
| Repository | `E:\projects\hnb` |
| Capture date | 2026-07-26 (Asia/Shanghai) |
| Git anchor | `db92dbe` (`main`, tracking `origin/main`) |
| Working tree | Dirty before this change; implementation, migrations, deployment assets, web console, active/archived changes, and contract edits include pre-existing modified and untracked files |
| Approved product baseline | HNB Cloud OpenSpec V3.8.6 |
| Phase 0 change | `establish-phase-0-project-truth` |

This is a working-tree truth baseline, not proof of a reproducible release from
`db92dbe`. The dirty-tree condition is blocker `BLK-REPO-001`. Existing user
changes were not reset, staged, deleted, or rewritten.

## 2. Inspection Scope and Method

Read-only inspection covered:

- route registration and HTTP/NATS handlers under `cmd/`;
- database migrations and repository/model code under `database/`, `pkg/`, and
  service-internal stores;
- unit, integration, E2E, and conformance-shaped tests;
- main, active, and archived OpenSpec artifacts;
- platform-api, ExecutionPlan/Operation persistence, outbox relay,
  operation-worker, and provider execution;
- the existing app-market schema, API, engine, artifact/Helm helpers, and
  events;
- cluster-agent, tunnel-server, API proxy, and tunnel library;
- Web Console shell, plugin manifests, permission/capability managers, stores,
  and application-market page;
- Docker Compose, Helm chart, NATS ACL, and provider manifests.

Evidence was collected with bounded `rg` searches, direct file reads, Git
metadata, Node syntax/JSON checks, and repository-local semantic parsing.
Runtime claims requiring PostgreSQL, NATS, Kubernetes, WSL, the Go toolchain,
or the approved OpenSpec CLI are explicitly not inferred when those tools were
unavailable.

## 3. Approved Architecture Truth

The repository itself makes the following constraints authoritative:

| Invariant | Source evidence |
|---|---|
| V3.8.6 is the implementation baseline | `doc/HNB_Cloud_OpenSpec_实施基线_V3_8_6.md:1-5,42` |
| Microkernel plus Provider/CapabilityPack | `openspec/architecture.md:45`; `openspec/specs/platform-kernel/spec.md:8-15` |
| App Market, Artifact Storage, Runtime Governance, and AI Extension are four logically decoupled planes | `openspec/architecture.md:46`; V3.8.6 baseline `:16-18,107-108` |
| The only runtime-target write path is `Release/CompositionRelease -> ExecutionPlan -> Operation` | `openspec/architecture.md:47,245-246`; `openspec/specs/platform-kernel/spec.md:19-27` |
| PostgreSQL is authoritative for Operation and the outbox/JetStream path carries execution intent | `openspec/architecture.md:122-160` |
| Console presentation is capability- and permission-driven | `openspec/specs/portal-experience/spec.md:8-15` |
| Market owns Product/Release/Channel/Entitlement; platform owns target selection and execution | `openspec/architecture.md:245-246` |

Observed deviations below are defects or incomplete seams. They are not
alternate architecture.

## 4. Maturity Rubric

| Level | Required evidence |
|---|---|
| L0 Route | HTTP route, event subject, menu/route declaration, manifest, or other exposed surface exists |
| L1 Handler | A handler, consumer, reconciler, or UI loader receives and processes input |
| L2 Functional/persistence | The path performs its intended behavior and persists authoritative state or produces the intended durable effect |
| L3 AuthN/AuthZ/isolation | Identity is authenticated, authorization is enforced, tenant/scope isolation is verified, and downstream context is trusted |
| L4 Tested | Relevant automated unit plus integration/failure-path tests pass in a representative environment |
| L5 Production readiness | Deployment, SLO/telemetry, capacity, upgrade/rollback, DR, supply-chain, and runbook evidence exists |

`Current` is the highest consecutively satisfied level. `△` means partial or
static evidence. A `✓` in a later column does not skip an earlier failed level.

## 5. Evidence-Based Maturity Matrix

| Surface | L0 | L1 | L2 | L3 | L4 | L5 | Current | Evidence and limiting gap |
|---|---:|---:|---:|---:|---:|---:|---|---|
| API server gateway, IAM, tenant, RBAC | ✓ | ✓ | △ | ✗ | ✗ | ✗ | L1 | Core routes and middleware chain exist (`cmd/apiserver/internal/router/router.go:77-121`), and IAM persists users/tokens/bindings. Login-issued tokens are incompatible with the auth middleware and tenant context is not propagated to handlers (`SEC-API-001`, `SEC-API-002`). No `_test.go` exists under `cmd/apiserver`. |
| Platform API Operation ingress | ✓ | ✓ | ✓ | ✗ | ✓ | ✗ | L2 | Operation/approval/query/target routes exist (`cmd/platform-api/internal/api/server.go:31-41`); submit persists plan, operation, steps, audit, read model, and outbox atomically (`internal/store/operations.go:28-158`). The server has no authentication wrapper and accepts tenant/actor identity from JSON (`internal/api/types.go:23-38`). Unit/integration/E2E tests exist, but L3 is unmet. |
| Operation store, outbox relay, worker | ✓ | ✓ | ✓ | △ | ✓ | ✗ | L2 | Versioned step subject, transactional outbox, shared durable consumer, leases/fencing, provider runner, read model, and extensive unit/integration tests exist. Envelope/tenant fields and fencing are validated, but trusted ingress and service transport authentication are not proven end-to-end. No production SLO/DR evidence was verified. |
| Kubernetes runtime provider | ✓ | ✓ | ✓ | △ | ✓ | ✗ | L2 | HTTP execution contract, namespace/scope validation, idempotency, CAS/fencing, logical delete, and tests exist under `cmd/kubernetes-provider/internal/provider`. Operation-worker calls configured provider endpoints over HTTP(S), but no verified mTLS/service identity is attached in `cmd/operation-worker/internal/driver/http.go`. |
| Runtime-target and extension CRUD | ✓ | ✓ | ✓ | ✗ | ✗ | ✗ | L2 | Database handlers persist runtime targets and extensions. Cluster `Get/Delete` and extension list/install/delete lack verified tenant predicates; extension lifecycle directly inserts/deletes rows instead of creating an Operation (`cmd/apiserver/internal/handler/cluster.go`, `extension.go`). |
| App Market catalog and release APIs | ✓ | ✓ | △ | ✗ | △ | ✗ | L1 | Publisher/product/release/application handlers and SQL repositories exist; market-engine unit tests cover in-memory rules. There is no auth middleware, identity headers are trusted, multiple queries are not tenant-scoped, HTTP/DB integration tests are absent, and release publish references a missing column (`MKT-SEAM-001`, `MKT-SEAM-002`). |
| App Market install/uninstall to Operation | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | L1 | Handlers persist transitional application status and publish `hnb.market.install`/`uninstall`, but no repository consumer connects those subjects to platform-api/Operation (`MKT-SEAM-003`). Publish is non-transactional Core NATS and errors are ignored. |
| App-market ReleaseManifest bridge | — | ✓ | △ | — | ✓ | ✗ | L1 | `ManifestBridge.ToExecutionPlan` and plan tests exist under `cmd/app-market/internal/engine/market`; the active NATS worker does not call the bridge and only emits a generic processed result, so the bridge is disconnected from the HTTP/application flow. |
| Tunnel server and agent transport | ✓ | ✓ | △ | △ | ✗ | ✗ | L1 | WebSocket registration, heartbeat, in-memory registry, and request/response types exist. Standalone tunnel uses an HMAC token, but the token is placed in the URL, origin checks always pass, `/agents` is unauthenticated, and no tunnel tests were found. |
| Cluster-agent Kubernetes proxy | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | L1 | Requests reach `proxyToKubeAPI`; the function always returns fixed HTTP 502 with `kube-proxy not yet implemented` (`cmd/cluster-agent/main.go:113-118`). |
| Web Console plugin shell | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | L1 | Plugin manifest, loader, capability manager, permission store, and plugin route declarations exist. There is no backend capability route, permission store hydration is disconnected, plugin API calls omit auth/context, and no web tests were found (`WEB-SEAM-001` through `003`). |
| Web Console Marketplace page | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | L1 | `/application/market` is declared, but `AppMarket.vue` is a placeholder that says API integration is pending; it does not call the existing app-market API. |
| PostgreSQL migration chain | — | ✓ | ✗ | ✗ | △ | ✗ | L1 | Migrations 001-023 exist and operation migrations have integration-test references. Sequential compatibility is broken by incompatible redefinitions of `projects`, `environments`, and `provider_registry` (`DB-SEAM-001`, `DB-SEAM-002`). No RLS policy was found. |
| Deployment packaging | ✓ | ✓ | △ | △ | ✗ | ✗ | L1 | Docker Compose and Helm subcharts include apiserver, platform-api, operation-worker, app-market, tunnel, agent, and providers. The apiserver route configuration does not wire platform-api/app-market/capabilities, and chart/template validation was not executable in the capture environment. |

## 6. Verified Facts

### FACT-ARCH-001 — The V3.8.6 architecture is explicit

Verified. `openspec/architecture.md:45-47` states the microkernel/provider
boundary, four logical planes, and canonical write path. Main specs
`KERNEL-001` and `KERNEL-002` repeat the enforceable behavior. This baseline
uses those sources instead of older solution documents.

### FACT-MKT-001 — `app-market` directly trusts identity headers

Verified. `cmd/app-market/main.go:103,119,141,187,212,240` reads
`X-Tenant-ID` or `X-User-ID` directly. The server handler is the raw
`http.ServeMux`; no authentication or authorization middleware wraps it.
Repository methods then use those values as scope. Several object-ID paths do
not include tenant scope at all (`pkg/appstore/store/repos.go:63-72,135-160,
178-209`).

Consequence: L3 is not met, and cross-tenant read/write is possible wherever
the service is reachable and object identifiers are known.

### FACT-MKT-002 — Direct Release publish bypasses gates and is also schema-broken

Verified. `POST /api/v1/releases/{id}/publish` calls `relRepo.Publish` directly
(`cmd/app-market/main.go:195-202`). The path has no tenant predicate, permission
check, signature/SBOM/vulnerability/license/conformance evidence, approval, or
Operation. The SQL directly changes status
(`pkg/appstore/store/repos.go:158-162`) and references `releases.updated_at`,
while migration 011 defines `published_at` but no `updated_at` on `releases`
(`database/postgresql/migrations/011_app_market_engine.sql:75-87`).

Consequence: the safety-gate bypass is architecturally invalid even if the SQL
is repaired; in the current schema it is also expected to fail at runtime.

### FACT-MKT-003 — Install/uninstall events do not drive canonical Operation

Verified within the captured repository. Application create/delete publishes
`hnb.market.install` and `hnb.market.uninstall`
(`cmd/app-market/main.go:205-235`). A bounded repository search found no
consumer for either subject. The app-market JetStream worker filters only
`hnb.market.release` (`cmd/app-market/internal/nats/worker.go:64-70`) and does
not submit to platform-api. Operation-worker filters its versioned
step-requested subject.

The application row update and event publish are not in one transaction; the
code uses Core NATS `Publish` without checking the return or flushing. A row can
remain `deploying`/`uninstalling` without durable execution intent.

### FACT-AGENT-001 — Kubernetes proxy is a fixed stub

Verified. `cmd/cluster-agent/main.go:117-118` returns status 502, no headers, and
`kube-proxy not yet implemented` for every request.

### FACT-OP-001 — Substantial canonical Operation machinery exists

Verified statically. Platform API submit builds a plan and writes
`execution_plans`, `operations`, `operation_steps`, `operation_audit`,
`operation_read_model`, and step-requested outbox events in one transaction
(`cmd/platform-api/internal/store/operations.go:28-158`). Operation-worker has
JetStream, leases/fencing, step persistence, provider HTTP execution, retries,
failed-message isolation, and read-model updates.

This is an implementation foundation, not end-to-end conformance:

- platform-api has no auth middleware (`cmd/platform-api/main.go:35-45`);
- tenant, initiator, and approver identities come from request JSON
  (`internal/api/types.go:23-38`);
- callers submit arbitrary steps plus a `releaseId`; the API does not load an
  immutable app-market Release/CompositionRelease or verify its policy result;
- plan persistence writes an empty policy result in submit
  (`internal/store/operations.go:50-58`);
- app-market application events do not reach this API.

### FACT-API-001 — API-server login tokens cannot satisfy its auth middleware

Verified statically:

- router login issues tokens through
  `iam.NewTokenManager("hnb-platform-secret", ...)`
  (`cmd/apiserver/internal/router/router.go:22-29`);
- `pkg/iam/token.go` encodes user-only claims with base64url and the hard-coded
  secret;
- protected-route middleware is created with `cfg.TokenSecret`
  (`cmd/apiserver/main.go:66-70`);
- `cmd/apiserver/internal/middleware/auth.go` expects hex-encoded claims with
  tenant/user fields and verifies with the configured secret.

Unless configuration accidentally equals the hard-coded value, the signature
differs; even then the encoding and claim shape differ. Login can succeed but
the returned access token cannot authenticate protected routes.

### FACT-API-002 — Authenticated context is disconnected from tenant handlers

Verified. Auth middleware stores tenant/user values on its custom context.
Tenant middleware comments that this context is authoritative, then sets
`X-Tenant-ID` on the **response** (`middleware/tenant.go:31-47`). Tenant and
cluster handlers read `X-Tenant-ID` from the **request**
(`handler/tenant.go:19,45`; `handler/cluster.go:19,67`). The request header is
neither derived nor overwritten.

`CheckPermission` also reads `X-User-ID` from the request
(`handler/iam.go:175-181`). CORS explicitly allows client-supplied
`X-Tenant-ID`.

Consequence: even after token compatibility is repaired, tenant-scoped handlers
can use an empty or spoofed identity header.

### FACT-API-003 — Authorization rules are not a verified scope boundary

Verified statically. Middleware maps methods to `read/create/update/delete` and
rules use singular resources such as `workspace` and `cluster`
(`middleware/authorization.go`). Built-in non-global roles use verbs
`get/list` and plural resources such as `workspaces` and `clusters`
(`pkg/iam/rbac.go`). Middleware passes `ctx.TenantID` as the scope ID even for
workspace or cluster rules.

Only a wildcard global role is likely to match consistently. No apiserver tests
exercise these rules.

### FACT-DB-001 — Later migrations are incompatible with earlier tables

Verified statically:

- migration 005 creates `projects` with text `id`, `tenant_id`, and no
  `workspace_id`;
- migration 021 uses `CREATE TABLE IF NOT EXISTS projects` with an incompatible
  UUID/workspace schema, then creates an index on `projects(workspace_id, name)`
  (`021_workspace_hierarchy.sql:18-29`);
- because the table already exists, the new definition is skipped and the
  index references a missing column.

Similarly:

- migration 010 creates `provider_registry` with `runtime_target_id`,
  `tenant_id`, and no `target_id`;
- migration 022 attempts an incompatible `CREATE TABLE IF NOT EXISTS`, then
  indexes `provider_registry(target_id)`
  (`022_extension_framework.sql:26-42`).

These are blockers to claiming a clean 001-023 bootstrap. Migration 021 also
does not transform the earlier `projects`/`environments` rows or types.

### FACT-WEB-001 — Capability/permission-driven Console is scaffolding

Verified:

- the manifest declares required permissions/capabilities;
- `PluginLoader` checks both (`web/shell/src/core/plugin-loader/PluginLoader.ts:
  39-57`);
- `CapabilityManager` calls `/api/v1/capabilities/{name}`
  (`core/capability/CapabilityManager.ts:5-18`);
- no backend route for that endpoint exists in the captured Go services;
- permission state starts empty and `setPermissions` is never called outside
  its store definition;
- auth login stores permissions on the user object, but apiserver login does
  not return permissions and the permission store is not hydrated;
- generic plugin API calls in `AppLayout.vue:35-48` do not attach the bearer
  token or trusted scope context;
- no web test files were found;
- `web/plugins/application/src/pages/AppMarket.vue` is a placeholder marked as
  awaiting API integration.

The JSON manifest parses successfully, but declaration validity is not
functional integration.

### FACT-ROUTE-001 — Configured extension routes are not active

Verified statically. `cmd/apiserver/main.go` loads and watches
`config/routes.yaml` and passes a registry into `router.New`. The `reg`
parameter in `cmd/apiserver/internal/router/router.go:24` is never used when
building or serving routes. Therefore the configured
`/extensions/{name}/**` NATS upstream is not connected by this router.

## 7. Stubs and Partial Integration Seams

| ID | Seam | Evidence | Classification |
|---|---|---|---|
| STUB-001 | Cluster-agent kube API proxy | Fixed 502 at `cmd/cluster-agent/main.go:117-118` | Hard stub |
| STUB-002 | Web Marketplace UI | `AppMarket.vue` contains only placeholder copy | UI stub |
| SEAM-001 | App-market install/uninstall -> Operation | Producers exist; no consumer found | Disconnected |
| SEAM-002 | App-market release worker -> ExecutionPlan | Bridge exists, worker ignores manifest/bridge and emits generic success | Disconnected/partial |
| SEAM-003 | Platform API -> immutable Market Release | Request supplies arbitrary steps and a release string; no market lookup/policy verification | Partial canonical path |
| SEAM-004 | API server -> platform-api/app-market | No core/configured route proxies these services | Disconnected ingress |
| SEAM-005 | Console -> capability API | Frontend caller exists; backend route absent | Disconnected |
| SEAM-006 | Console auth -> permission/plugin API | Permission store not hydrated; generic calls omit bearer token | Disconnected security context |
| SEAM-007 | Config route registry -> router | Registry loaded/passed but not used | Dead integration |
| SEAM-008 | Extension lifecycle -> Operation | Direct inserts/deletes plus separate reconciler | Architecture bypass |
| SEAM-009 | Tunnel -> Kubernetes API | Transport request reaches agent but execution stub stops flow | Incomplete |
| SEAM-010 | Migration 005 -> 021 and 010 -> 022 | Incompatible `IF NOT EXISTS` redefinitions | Bootstrap blocker |

## 8. Security Boundaries

| Boundary | Current evidence | Status |
|---|---|---|
| User -> apiserver | Login/persistence and middleware exist, but token contracts differ | Blocked |
| Auth middleware -> handlers | Identity stays on custom context; handlers read spoofable request headers | Blocked |
| User/service -> app-market | No auth wrapper; tenant/user headers trusted | Blocked |
| User/service -> platform-api | No auth wrapper; tenant/actor body values trusted | Blocked |
| Tenant -> app-market repositories | Some list/search predicates exist; object-ID reads/writes and publish/uninstall lack tenant predicates | Blocked |
| Tenant -> runtime target/extension handlers | Several get/delete/list paths lack tenant/workspace predicates | Blocked |
| Operation worker -> provider | Execution attempt and fencing validated; no verified mTLS/service identity | Partial |
| Agent -> tunnel | HMAC token in URL, permissive origin, unauthenticated standalone agent list | Partial/high risk |
| Web Console -> APIs | Token in localStorage; most API calls do not attach it; capability and permission paths incomplete | Blocked |
| PostgreSQL tenant boundary | Application-level predicates only; bounded search found no RLS enablement/policies | Not defense in depth |

Secrets and credentials additionally require later review:

- app-market defaults include a plaintext development DSN and accepts Harbor
  credentials from environment;
- Helm repository sync accepts username/password in the API request object;
- tunnel token is placed in a query string by `pkg/tunnel/client.go`;
- platform-api validates many plaintext-secret-shaped step inputs, which is
  positive evidence, but it does not authenticate who supplies a
  `SecretReference`.

## 9. Blocker Register

| ID | Priority | Blocker | Exit condition for a later change |
|---|---|---|---|
| BLK-REPO-001 | P0 | Implementation truth is primarily in a dirty/untracked working tree, so release provenance is not reproducible | Preserve/commit intended implementation in reviewable changes and recapture baseline |
| BLK-VAL-001 | P0 | Native OpenSpec 1.3.1 global strict validation fails the pre-existing `add-multi-cluster` change because its two spec files have no delta sections | Repair that change's delta structure, then rerun `openspec validate --all --strict --no-interactive` |
| BLK-VAL-002 | P0 | Repository-local semantic parsing finds pre-existing duplicate IDs, missing traceability, and a missing scenario in active changes | Archive applied changes or reconcile active deltas without losing history; semantic gate passes |
| BLK-DB-001 | P0 | Migration 021 conflicts with migration 005 | Provide forward migration from tenant/project schema to workspace hierarchy and prove clean plus upgrade paths |
| BLK-DB-002 | P0 | Migration 022 conflicts with migration 010 provider registry | Consolidate schema through a forward migration and update repositories/tests |
| BLK-AUTH-001 | P0 security | Apiserver login tokens cannot authenticate protected routes | One versioned token contract, configured secret/key management, negative tests, rotation/rollback |
| BLK-TENANT-001 | P0 security | Apiserver and app-market handlers trust client identity headers or lose authenticated context | Derive context at trusted boundary, overwrite/remove inbound identity headers, enforce repository predicates, test cross-tenant denial |
| BLK-AUTHZ-001 | P0 security | RBAC verbs/resources/scope IDs are inconsistent | Versioned permission model and table-driven route/scope tests |
| BLK-MKT-001 | P0 security/architecture | Direct release publish has no tenant/auth/gates and references missing SQL column | Gate publication with immutable evidence and tenant authorization; schema/tests pass |
| BLK-MKT-002 | P0 architecture | Market install/uninstall subjects do not create/drive canonical Operation | Transactional intent handoff to authenticated platform API/outbox; E2E proves Operation ownership |
| BLK-OP-001 | P0 security | Platform API accepts tenant, initiator, approver, release, and steps from unauthenticated request bodies | Trusted ingress, authorization, immutable release resolution, policy/approval verification |
| BLK-AGENT-001 | P0 functional | Cluster-agent Kubernetes proxy is fixed 502 | Versioned least-privilege agent contract, mTLS, Kubernetes client implementation, scope tests |
| BLK-WEB-001 | P0 integration | Capability endpoint and permission hydration are absent | Authenticated capability/read-model contract, permission hydration, route/action tests |
| BLK-WEB-002 | P0 integration | App Market page does not use existing Marketplace API | Extend existing app-market integration through an approved change; no parallel service |
| BLK-EXT-001 | P0 architecture | Extension install/delete directly mutates persistence outside Operation | Convert runtime-changing lifecycle to canonical Operation and prove reconciliation |
| BLK-PROD-001 | P0 readiness | No complete production evidence set (SLO, capacity, upgrade, rollback, DR, runbooks, supply chain) was verified | Component-specific L5 evidence and drills |

## 10. Test and Validation Inventory

Static inventory found 330 Go test functions across the repository. Strongest
concentrations include:

- operation engine, config/secret handling, HTTP provider driver, NATS worker,
  lease/fencing, and persistence integration;
- platform-api validation, HTTP behavior, PostgreSQL integration, RBAC-shaped
  integration, and E2E outbox flows;
- Kubernetes provider idempotency, fencing, delete/tombstone, and HTTP contract;
- gateway renderer/adapter/multi-tenant validation;
- core provider registry and runtime-target compatibility;
- multi-cluster/GSLB health, DNS, store, and reconciliation;
- app-market in-memory release/channel/entitlement/plan behavior.

Important gaps:

- no test files under apiserver, cluster-agent, tunnel-server, or `pkg/tunnel`;
- no app-market HTTP/repository integration tests;
- no web test files;
- many integration tests require PostgreSQL/NATS/Kubernetes and are not proof
  until executed in the intended environment;
- no clean 001-023 migration-chain test was found for the later schema
  collisions.

## 11. Validation Outcomes

This section is finalized with the change and distinguishes pass, fail, and
not-executable outcomes.

| Command/check | Outcome | Interpretation |
|---|---|---|
| `git status --short --branch` | PASS as inspection; dirty tree observed | Evidence capture preserved pre-existing changes |
| Bare `openspec --version` | NOT EXECUTABLE | Command is not installed globally/on PATH |
| `npx --package @fission-ai/openspec@1.3.1 openspec --version` | PASS | Approved CLI version `1.3.1` executed from a temporary npm cache; no repository dependency was added |
| OpenSpec 1.3.1 `validate establish-phase-0-project-truth --strict --no-interactive` | PASS | Native result: `Change 'establish-phase-0-project-truth' is valid` |
| OpenSpec 1.3.1 `validate --all --strict --no-interactive` | FAIL (pre-existing) | 29 items passed and one failed: `change/add-multi-cluster` |
| OpenSpec 1.3.1 `validate add-multi-cluster --strict --no-interactive` | FAIL (pre-existing) | `gslb/spec.md` and `multi-cluster/spec.md` have no ADDED/MODIFIED/REMOVED/RENAMED delta sections; the change therefore has no parsed deltas |
| `node --check scripts/validate-openspec.mjs` | PASS | Repository validator script parses |
| `node --check scripts/validate-openspec.test.mjs` | PASS | Validator test script parses |
| Repository-local semantic parse of current active/main specs | FAIL (pre-existing) | 43 spec files, 190 requirements, 45 errors before this change: 30 duplicate IDs, 14 missing traceability entries, and one missing scenario |
| `JSON.parse(web/shell/public/config/plugin-manifest.json)` | PASS | Manifest is valid JSON; this does not prove plugin functionality |
| Relevant Go test command | NOT EXECUTABLE | `go` is not on Windows PATH; bundled repository Go is a Linux binary and WSL launch is denied |
| `pnpm.cmd --dir web typecheck` | NOT EXECUTABLE | Workspace dependencies are not installed; pnpm attempted registry access, received `EACCES`, and timed out. No dependency files were created. |
| `helm lint deploy/charts/hnb` | NOT EXECUTABLE | Helm is not installed/on PATH |
| New delta semantic validation | PASS | Six requirements (`P0-BASE-001` through `P0-BASE-006`), complete traceability/scenarios, zero repository-local errors attributable to this change |
| Full semantic parse after this change | FAIL (pre-existing only) | 44 spec files, 196 requirements, 45 errors; `newChangeErrors` is empty |
| Product-behavior diff review | PASS | The change directory contains OpenSpec/evidence Markdown and `.openspec.yaml` only; no runtime, schema, API, event, deployment, or UI file changed |

## 12. Phase 0 Exit Decision

Phase 0 may exit as **Reviewable with blockers** when:

- all artifacts in this change exist;
- `P0-BASE-001` through `P0-BASE-006` pass local semantic validation;
- the final diff contains evidence/OpenSpec files only;
- validation limitations remain explicitly reported;
- reviewers acknowledge that Phase 1 must close blockers through separate
  changes rather than treating them as accepted debt.

Phase 0 exit does **not** mean the platform is secure, end-to-end functional, or
production ready. The maturity matrix intentionally prevents that inference.
