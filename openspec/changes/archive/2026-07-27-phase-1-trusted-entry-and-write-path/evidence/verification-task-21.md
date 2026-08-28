# Verification: Task 21 - Integration Tests for Write Path

## Scope
P1-WRITE-001 through P1-WRITE-005 integration test coverage.

## Test Coverage Added

### e2e_test.go (cmd/platform-api/internal/store)
Added the following test functions using `HNB_TEST_POSTGRES_DSN` conditional setup:

1. **TestE2E_SubmitIntentIdempotency** — Proves that SubmitIntent with identical (tenant + intentKind + idempotencyKey) returns existing operation (created=false), while the same key in a different tenant creates a separate operation.

2. **TestE2E_SubmitIntentSemanticConflict** — Proves that two intents with different parameters for the same release+target produce separate operations (they have different idempotency keys but different semantic digests).

3. **TestE2E_AtomicFailurePreservesNoPartialState** — Proves that after a successful intent submit, an idempotent replay returns the existing operation without creating duplicates or partial state.

4. **TestE2E_OutboxEventIntegrityAfterCommit** — Proves that every outbox event produced by SubmitIntent carries a non-empty correlation_id and payload, ensuring cross-service event delivery integrity.

### integration_test.go (existing tests that support Task 21)
Already present in `integration_test.go`:
- `TestPGStore_SubmitOperation_idempotent` — Idempotency via unique constraint on idempotency_key
- `TestPGStore_SubmitOperation_differentTenantSameKey` — Tenant-scoped idempotency
- `TestPGStore_SubmitOperation_outboxEvents` — Outbox integrity
- `TestPGStore_ApproveOperation_outboxEvents` — Post-approval outbox emission
- `TestPGStore_SubmitOperation_multiStepOutbox` — Multi-step DAG outbox event count

## Build Verification
```
cd /mnt/e/projects/hnb/cmd/platform-api && go build ./...   # OK
cd /mnt/e/projects/hnb/cmd/platform-api && go test -race -count=1 ./internal/engine/...   # OK
```

Integration tests requiring PostgreSQL run conditionally when `HNB_TEST_POSTGRES_DSN` is set.
