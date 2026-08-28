# Task 5.4 Operation-Worker Lifecycle Provider Routing

## Strict lifecycle routing

- `cmd/operation-worker/internal/driver/http.go` now requires every configured
  `runtime-target.lifecycle.kubernetes` and `runtime-target.lifecycle.edge`
  provider to declare `requiredProvider`, `providerVersion` and `providerDigest`
  (startup validation in `NewHTTPRunner`).
- The HTTP request body includes `providerProtocolVersion`, `providerVersion`
  and `providerDigest` for every call, alongside the existing `executionAttemptId`,
  `idempotencyKey` and `fencingGeneration`.
- The driver rejects lifecycle invocations when the planner-pinned
  `providerVersion` or `providerDigest` does not match the worker-configured
  values (`pinned version ... does not match configured version` /
  `pinned digest ... does not match configured digest`).
- Provider echo mismatches on `schemaVersion`, `executionAttemptId`,
  `idempotencyKey`, `providerVersion`, `providerDigest` or `fencingGeneration`
  are returned as `INVALID_REQUEST` `ProviderError`s without mutating Operation
  state (`mismatched ...`).

## Schema and contract guard

- The worker pins `schemaVersion = "2.0.0"` on every request and refuses any
  response that returns a different version.
- `executionAttemptId` must be a UUID and must echo back from the provider.
- `idempotencyKey` echoes the planner-derived seed and is rejected when the
  provider returns a different value.
- `fencingGeneration` is allocated by `worker_leases` from
  `operation_fencing_generation_seq`; the driver echoes the canonical decimal
  string back and rejects any non-canonical / stale generation. The Operation
  is never marked successful on a fencing mismatch.

## Replay, wrong echo and resume

- `TestHTTPRunnerReplaysWithCurrentAttempt` proves a step can be replayed with
  the current attempt, idempotency key and fencing generation and that the
  provider response with a non-empty `checkpoint` is preserved for resume.
- `TestHTTPRunnerRejectsLifecycleEchoMismatch` enumerates all six echo
  mismatches (schema, attempt, idempotency, providerVersion, providerDigest,
  fencing) and asserts each is rejected with a stable error message.

## Persistence and store updates

- `cmd/operation-worker/internal/store/operations.go` now reads `provider_version`
  and `provider_digest` alongside the existing columns when loading step state.
- The store serializes step inputs as `map[string]any` to preserve the
  authoritative lifecycle input snapshot written by migration 055.

## Configuration

- `cmd/operation-worker/internal/config/config.go` adds `protocolVersion`,
  `providerVersion`, `providerDigest` and `requiredProvider` to the
  `RUNTIME_PROVIDERS` JSON contract and validates the lifecycle family at load
  time. Default `protocolVersion` is `2.0.0` and any other value is rejected.
- `cmd/operation-worker/main.go` threads the new fields into the driver
  `ProviderConfig`.

## Verification

- `go build ./...` in `cmd/operation-worker`: pass.
- `go test ./... -race -count=1` in `cmd/operation-worker`: pass
  (config, driver, engine, engine/config, nats, store).
- `go test ./... -race -count=1` in `cmd/platform-api`: pass.
- `go test ./... -race -count=1` in `cmd/apiserver`: pass.
- `npm run contracts:generate -- --check`: pass.
- `openspec validate cluster-management-full-closure --strict`: pass.
