# Verification: Task 24 - Contract Tests for Operation Lifecycle

## Scope
P1-WRITE-004 — Prove all runtime lifecycle entry points produce/control a canonical Operation.

## Entry Point Inventory and Verification

### POST /v1/intents (Intent Submission)
- Path: `cmd/platform-api/internal/api/server.go` → `SubmitIntentHandler`
- Flow: ParseRuntimeIntent → IntentValidator → Planner → PGStore.SubmitIntent
- SubmitIntent atomically persists: runtime_intent, execution_plan, operation, steps, audit, security_audit_events, outbox_events, operation_read_model
- Idempotency key guarantees no duplicate operations
- Direct write path to operations table is **only** through SubmitIntent (no other code path bypasses intent validation)

### SubmitOperation (Legacy/Backend Path)
- Already exists in store with idempotency via unique constraint on `(tenant_id, idempotency_key)`
- Step 19 task ensured header sanitization prevents public route from passing arbitrary steps

### Approve/Reject/Cancel Operations
- All three go through `lockOperation` with `FOR UPDATE` row lock
- State transitions guarded by current status check
- Audit events recorded for every transition
- Read model updated in same transaction

## Evidence
- `TestE2E_SubmitIntentIdempotency` — confirms only one operation per (tenant, kind, key)
- `TestPGStore_SubmitOperation_idempotent` — confirms idempotent replay returns existing op
- `TestPGStore_ApproveOperation_wrongTenant` — confirms cross-tenant rejection at operation level
- `TestPGStore_GetOperation_crossTenantIsolation` — confirms tenant-scoped reads

All known Operation flows start with Intent submission (for external requests). The legacy SubmitOperation path is internal-only and already passes through typed commands.
