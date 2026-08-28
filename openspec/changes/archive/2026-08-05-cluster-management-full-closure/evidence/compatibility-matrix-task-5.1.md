# Task 5.1 Compatibility Matrix Evidence

## Authority and runtime enforcement

- Canonical authority: `contracts/schema/runtime-target/v1/compatibility-matrix.json`.
- Runtime mirror: `cmd/platform-api/internal/engine/runtime-target-compatibility-matrix.json`, embedded by the planner and guarded against the canonical source by `scripts/contracts.test.mjs`.
- The loader rejects an incomplete matrix, duplicate target rows, invalid action cells, unsupported matrix status values, and use outside `effectiveAt`/`expiresAt`.
- Every cluster lifecycle plan evaluates the matrix and pins the matrix decision, Provider version, Provider digest, conformance evidence reference, capability snapshot digest, and exact `Step.providerId` route.

## Provider Registry and capabilities

- `lifecycleProviderResolver` requires the exact matrix `providerId`, protocol `2.0.0`, `production_ready`, a future `conformance_expires_at`, the requested action, and passing evidence for the exact target/action cell.
- Missing, ambiguous, expired, un-conformed, failed-evidence, or protocol-incompatible manifests fail closed before operation persistence.
- `/v1/console/bootstrap` publishes only lifecycle capabilities whose matrix cells and Provider manifests pass the same resolver.
- Edge `create` remains `UNSUPPORTED` in the canonical matrix and is rejected with `TARGET_ACTION_UNSUPPORTED` even if a client bypasses UI capability hiding.

## Contract additions

- `ExecutionPlan.compatibilityDecision` records the immutable matrix decision.
- `ExecutionPlan.steps[].providerId` records the exact executable Provider route.
- Generated TypeScript platform contracts were refreshed and the generated-output drift check passes.

## Verification

- `go test ./... -race -count=1` in `cmd/platform-api`: pass.
- `go test ./... -race -count=1` in `cmd/apiserver`: pass.
- `node --test scripts/contracts.test.mjs`: 20/20 pass.
- `npm run contracts:generate -- --check`: pass.
- `openspec validate cluster-management-full-closure --strict`: pass.
