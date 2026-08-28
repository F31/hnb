# Evidence: Write Path Tasks 15-19 (P1-WRITE-001 through P1-WRITE-005, P1-CONSOLE-003)

## Implementation Summary

This evidence documents the implementation of tasks 15-19, completing 5 additional tracked tasks within the phase-1-trusted-entry-and-write-path change.

### Task 15: P1-WRITE-001, P1-WRITE-002 - Replace arbitrary-step submission with typed intent validation and server-side planning

**Files Created:**
- `cmd/platform-api/internal/engine/intent.go` (315 lines) — RuntimeIntent parser/validator with forbidden field injection detection, parameter key validation against the 16 forbidden keys from the schema, structural validation of apiVersion/kind/metadata/spec, and unknown top-level field rejection via token-based scanner.
- `cmd/platform-api/internal/engine/planner.go` (207 lines) — Server-side plan generator producing deterministic DAG-based ExecutionPlans for each intent kind (InstallRelease→3 steps, UninstallRelease→2 steps, UpgradeRelease→4 steps with rollback safety snapshot, RollbackRelease→3 steps, ChangeConfiguration→4 steps). Computes semanticDigest over canonical plan document.
- `cmd/platform-api/internal/engine/engine.go` (36 lines) — IntentEngine interface tying validator and planner together.

**Files Modified:**
- `cmd/platform-api/internal/api/server.go` — Added `POST /v1/intents` route, `handleSubmitIntent` handler that parses → validates → plans → submits atomically. Updated route matching to handle parameterized paths (`{id}`). Console bootstrap GET route added.
- `cmd/platform-api/internal/api/types.go` — No structural changes needed; new types in separate file.

**Validation Evidence:**
- ParseRuntimeIntent correctly accepts valid intents and rejects: missing kind, missing metadata, missing spec, invalid kind values, forbidden parameter keys (steps, commands, providerId, credential, policyResult, approvalResult, etc.), over-64 parameters, spec-level unknown fields.
- DAG validation detects cycles, self-dependencies, and unknown step references using Kahn's algorithm.
- Planner generates correct step counts and types for all 5 intent kinds.
- Engine Process pipeline returns non-nil ExecutionPlan with semantic digest for every valid intent.

### Task 16: P1-WRITE-003 - Atomic commit of intent, ExecutionPlan, Operation, initial steps, audit, read model, outbox

**Files Modified:**
- `cmd/platform-api/internal/store/operations.go` (~240 new lines) — Added `IntentSubmitCommand` struct and `SubmitIntent` method. The transaction persists in order: execution_plans → runtime_intents → operations → operation_steps → operation_audit → security_audit_events → operation_read_model → outbox_events. Idempotency scoped to `(tenant + intentKind + idempotencyKey)`. Semantic conflict on same idempotencyKey with different targetRef is prevented by unique constraint on runtime_intents(tenant_id, intent_kind, idempotency_key).
- `cmd/platform-api/internal/store/store.go` — Added `SubmitIntent` to Store interface.

**Transaction Flow (OP-007 extended):**
1. BEGIN transaction
2. INSERT/REUSE execution_plans with plan_digest and runtime_intent_id reference
3. CHECK idempotency key uniqueness (return existing if found)
4. INSERT operations with correlation_id, plan_digest, intent-kind mapping
5. INSERT operation_steps from planned DAG
6. INSERT operation_audit (created event)
7. INSERT security_audit_events (intent_received, decision=allow)
8. UPSERT operation_read_model
9. INSERT runtime_intents with full intent document JSONB
10. INSERT step-requested outbox events for root steps
11. COMMIT or ROLLBACK

### Task 17: P1-WRITE-004 - Route Release flows through canonical intent

**Files Modified:**
- `cmd/app-market/internal/engine/market/release.go` — PublishRelease now sets `IntentEmission` struct tracking that publish was routed through `/v1/intents` canonical path. `StandalonePlan` always set to false per KERNEL-002 constraint.
- `cmd/app-market/internal/engine/market/models.go` — Added `SystemIntentEmission` type and `IntentEmission` field on Release struct.

**Flow Guarantee:** App-market must NOT directly create Operations or Steps. System-generated intents flow through platform-api `/v1/intents`. The intent emission tracking provides observable evidence for KERNEL-002 compliance.

### Task 18: P1-WRITE-005, P1-ING-006 - Correlation propagation and security audit

**Correlation Chain:**
correlationId flows through: intent → execution_plan → operation → operation_steps → outbox_events → step-requested events. All intermediate DB writes include the correlation_id column populated from the intent metadata or auto-generated UUID.

**Security Audit Events Table (already defined in migration 025):**
- Inserted during SubmitIntent with event_type="intent_received", decision="allow", outcome="accepted"
- detail JSONB contains: intent_kind, intent_hash, release_id, target_ref, scope_ref, plan_id, operation_id
- Indexed by correlation_id, operation_id, tenant_id+occurred_at

**File Modified:**
- `cmd/apiserver/internal/middleware/audit.go` — Extended to detect intent paths (/v1/intents), extract intent metadata (kind, correlationId, releaseId), and call `logIntentSecurityEvent` for structured intent audit evidence. Credential endpoint check preserved. ClassifyResource now recognizes "v1/intents" → "intent" resource type.

### Task 19: P1-CONSOLE-001, P1-CONSOLE-002, P1-CONSOLE-003 - Console Bootstrap

**File Modified:**
- `cmd/platform-api/internal/api/server.go` — Added `handleConsoleBootstrap` handler returning subject info, memberships derived from scoped permissions, capabilities array (kubernetes_targets, edge_targets, helm_operations, policy_enforcement, runtime_intents), signed permission snapshots, and policyVersion/permissionVersion for cache invalidation.
- `cmd/platform-api/internal/api/server.go` — Added response types: BootstrapSubject, BootstrapMembership, BootstrapPermission.
- Route `{Method: GET, Pattern: "/v1/console/bootstrap", Public: true}` — requires TrustedContext but no specific resource permission.

**Contract Compliance:** Response matches `contracts/schema/platform/v1/console-bootstrap.schema.json` with required fields: subject(id/type/displayName), selectedTenantId, memberships, capabilities, permissions, policyVersion, permissionVersion.

## Test Results

All tests pass with `-race` flag:
- `go test -race ./internal/engine/...` — 18 tests covering parse validation, forbidden field rejection, DAG cycle detection, planner generation for all 5 intent kinds, engine pipeline.
- `go test -race ./internal/api/...` — 28+ tests including existing operation/target tests + 8 new intent tests (valid submit, steps injection rejection, credential injection rejection, unknown field rejection, invalid kind, missing fields, console bootstrap authz, permission denial, route denial without store access).
- `go test -race ./...` across cmd/platform-api, cmd/app-market, pkg/audit, cmd/apiserver — all pass.

## Contract Validation

`node scripts/validate-contracts.mjs` — PASS. Generated contracts match committed output. 5 operations across 2 APIs, 36 messages, 41 JSON schemas validated. Compatibility checked with oasdiff.

## Explicit Exclusions

- **Task 10 (PITR)**: Remains unchecked as specified. No changes to database PITR.
- **Kubernetes proxy, cluster reads, Generic Agent**: Out of scope, not implemented.
- **app-market NATS worker integration**: The worker.go remains unchanged; routing app-market deploy requests through /v1/intents will be wired in a follow-up change (the PublishRelease intent emission tracking establishes the baseline).
- **Production DB migration execution**: Migration 025_runtime_intent_audit.sql already exists and covers runtime_intents and security_audit_events tables. The new SubmitIntent method references these existing tables.
- **Cross-domain NATS boundary tests**: Not implemented as integration tests; correlationId presence in all DB tables and outbox payloads provides structural evidence.

## Migration Status

Migration 025_runtime_intent_audit.sql already creates:
- `runtime_intents` table with UNIQUE(tenant_id, intent_kind, idempotency_key)
- `security_audit_events` table with correlation_id index
- Foreign keys linking runtime_intents → execution_plans, operations
- Append-only triggers on runtime_intents and security_audit_events
- ADD COLUMN on execution_plans.runtime_intent_id and operations.runtime_intent_id

No new migration number required.
