# Tasks 5.2 / 5.3 Lifecycle Plan Closure

## Authority and immutability

- The cluster lifecycle ExecutionPlan schema in `contracts/schema/platform/v1/execution-plan.schema.json`
  pins each step's `providerId`, `providerVersion`, `providerDigest`, `providerProtocolVersion`,
  `targetRef`, `targetKind`, `inputSchema`, `inputs`, `secretReferences`, `idempotencyKey`,
  `fencingPolicy`, `retryPolicy`, `timeoutSeconds` and `compensation`.
- The published runtime input snapshots remain
  `contracts/schema/runtime-target/v1/kubernetes-lifecycle-step-input.schema.json` and
  `contracts/schema/runtime-target/v1/edge-lifecycle-step-input.schema.json`.

## Planner behaviour

- For every cluster RuntimeIntent the planner produces a single explicit side-effect step
  named `runtime_target.<kubernetes|edge>.<provision-and-register|register|upgrade|unregister>`.
  The previous generic `validate/import/verify` DAG is removed.
- The planner resolves the exact Provider (`runtime-target.lifecycle.kubernetes` or
  `runtime-target.lifecycle.edge`), pins its `version`, `digest`, and `providerProtocolVersion`
  (`2.0.0`), and freezes those values into every step.
- The planner allocates a deterministic server-owned UUID for the target via
  `uuid.NewSHA1`, records it in `ExecutionPlan.TargetSnapshot`, and never uses
  `kind:displayName` as an identifier.
- `SecretReferenceEntry` values, including the cluster `credentialSecretRef`, are pinned
  on every step and on the plan top-level. No secret values are read or stored.
- `retryPolicy.maxAttempts`, `timeoutSeconds`, `fencingPolicy=monotonic-worker-lease-v2`
  and a `compensation` block (none, unregister with bounded ownership scope) are part of
  the immutable step payload.
- `expectedVersion` is propagated as the target `projectionVersion` and snapshot's
  `observationVersion`, while `fencingGeneration` is initialised at 1 and remains
  authoritative at the worker lease layer.

## Persistence and database immutability

- Migration `database/postgresql/migrations/055_lifecycle_plan_immutability.sql`
  adds `provider_version`, `provider_digest`, `provider_protocol_version`,
  `input_schema`, `secret_references`, `compensation`, `target_ref`, `target_kind` to
  `operation_steps`.
- The migration installs two triggers: `execution_plans_immutable` rejects mutations of
  `plan_digest`, `plan_json`, `release_id`, `tenant_id` and `runtime_intent_id`, while
  `operation_steps_immutable` rejects changes to step routing and inputs (Provider ID,
  version, digest, protocol, schema, secret references, compensation, target, idempotency
  key, retry/timeout, etc.). Status, retry counters, checkpoint, lease, output, error and
  timestamps remain mutable for execution bookkeeping.
- Migration 055 was verified in a PostgreSQL 16 container:
  - forward apply succeeds after migrations 001-054 are in place;
  - apply is idempotent (`CREATE FUNCTION`, `CREATE TRIGGER`, `CREATE INDEX` guarded by
    `IF NOT EXISTS`);
  - rollback restores the prior state (`DROP TRIGGER`, `DROP CONSTRAINT`,
    `DROP COLUMN`);
  - post-rollback reapply succeeds and the protection triggers are rearmed.
- Live mutation tests in the same container confirm:
  - `UPDATE execution_plans SET plan_digest='sha256:hacked' ...` is rejected;
  - `UPDATE execution_plans SET status='superseded' ...` is permitted (status transition);
  - `UPDATE operation_steps SET provider_version='9.9.9' ...` is rejected;
  - `UPDATE operation_steps SET status='succeeded' ...` is permitted;
  - INSERT with non-sha256 `provider_digest` is rejected;
  - Legacy no-Provider `install.*` rows continue to be accepted for backward compatibility.
- `plan_json` is now stored as the canonical `ExecutionPlan` JSON (including the
  per-step provider pin, secret references, compensation and target snapshot) instead of
  the lossy step summary.
- `runtime_intents.intent_document` now persists every cluster field
  (`displayName`, `kubernetesVersion`, `cloudCoreEndpoint`, `credentialSecretRef`,
  `nodeGroupMappings`, `riskConfirmation`) so the persisted document can reconstruct
  the canonical semantic digest.

## Golden and conformance tests

- New `cmd/platform-api/internal/engine/lifecycle_plan_golden_test.go` plus seven JSON
  fixtures under `cmd/platform-api/internal/engine/testdata/` lock the plan output for:
  - `kubernetes-create`, `kubernetes-import`, `kubernetes-upgrade`, `kubernetes-unmanage`
  - `edge-import`, `edge-upgrade`, `edge-unmanage`
  - The fixtures assert the namespaced step type, exact Provider pin, deterministic
    server-allocated target UUID, schema-valid inputs, fenced SecretReferences,
    retry/timeout/compensation metadata, and forbid any secret value markers
    (`token:`, `secretvalue`, `kubeconfig:`, `privatekey`, `-----begin`).
- A digest stability test proves two identical intents produce an identical plan digest
  while a mutated intent produces a different digest, demonstrating the planner pins
  the Provider, observation snapshot, and inputs into the canonical hash.
- Compatibility matrix negative tests remain in place:
  - Edge `create` is rejected with `TARGET_ACTION_UNSUPPORTED`;
  - Missing Provider route returns `PROVIDER_ROUTE_NOT_FOUND` without mutating state.

## Verification

- `go test ./... -race -count=1` in `cmd/platform-api`: pass.
- `go test ./... -race -count=1` in `cmd/apiserver`: pass.
- `npm run contracts:generate -- --check`: pass (clean diff of generated contracts).
- `node --test scripts/contracts.test.mjs`: 20/20 pass.
- `openspec validate cluster-management-full-closure --strict`: pass.
- PostgreSQL 16 container validation of migration 055 (forward / idempotent /
  rollback / reapply / live trigger checks): pass.
