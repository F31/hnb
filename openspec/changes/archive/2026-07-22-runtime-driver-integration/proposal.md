## Why

`operation-worker` currently fails closed because no Runtime Driver is wired into its `StepRunner`; therefore no valid Operation can reach a real Provider. This change adds the smallest production-safe execution boundary while preserving Operation Store authority, retries, checkpoints, timeouts, and fencing.

## What Changes

- Change ID: `runtime-driver-integration`.
- Add a versioned HTTP Runtime Driver contract that routes authoritative Step execution contexts by `provider_id`.
- Configure Provider endpoints explicitly at process startup and reject missing, duplicate, invalid, or unknown Provider mappings.
- Propagate tenant scope, idempotency key, checkpoint, deadline, and fencing token to the Provider and strictly validate Provider responses.
- Wire the driver into `operation-worker` instead of the current `nil` runner while retaining fail-closed behavior.
- Add contract and integration-style HTTP tests for success, resume, Provider errors, malformed responses, routing, cancellation, and fencing propagation.
- Classification: T0 integration adapter in Operation Worker; called Runtime Drivers/Providers remain replaceable T1 capability implementations.
- Affected planes: runtime governance/control plane only. Artifact, data, AI Extension, and Portal planes are unchanged.
- Dependency changes: depends on completed `operation-engine-core` and `runtime-target-engine`; no new middleware, database, or third-party library.
- Migration: set `RUNTIME_PROVIDERS` before dispatching executable Steps. Existing database and NATS data require no migration.
- Rollback: remove `RUNTIME_PROVIDERS` or roll back the worker image; startup or execution remains fail-closed and no successful Step is fabricated.
- User value: approved Operations can invoke independently deployed Providers through one auditable execution path.
- Non-goals: concrete Kubernetes/container/edge resource logic, Provider discovery, Agent transport, DAG dispatch, pause/cancel lease revocation, and Provider-side persistence.
- Compatibility: the existing internal `StepRunner` shape remains unchanged; the new wire contract is additive and versioned.
- Security: endpoint configuration must not contain credentials; transport credentials are not introduced here, sensitive Inputs remain subject to existing SecretReference rules, and Providers must enforce fencing tokens at side-effect boundaries.
- Resource budget: one bounded HTTP request per active Step, response body limited to 1 MiB, no background goroutines or persistent connections beyond Go's shared HTTP transport.
- Observability: log Provider ID and classified execution failures without logging Inputs or response bodies; existing Operation audit/outbox records remain authoritative.
- Exit criteria: targeted driver tests, full module tests, `go vet`, strict OpenSpec validation, and a Worker startup test with a configured Provider all pass.

## Capabilities

### New Capabilities
- `runtime-driver-execution`: Versioned, fenced, resumable Step invocation from Operation Worker to an explicitly configured Runtime Provider.

### Modified Capabilities
- `composition-operation`: Executable Steps use a configured Runtime Driver instead of unconditional fail-closed rejection, while preserving authoritative commit and retry behavior.

## Impact

- Affected code: `cmd/operation-worker/internal/runtime`, worker configuration, `main.go`, and tests.
- Affected API: new internal HTTP `POST /v1/steps:execute` request/response contract between Operation Worker and Runtime Providers.
- Operations: Provider endpoint mappings become required for real execution; malformed configuration prevents startup.
- Data/install/upgrade/backup/restore/uninstall: no schema, middleware, backup, or restore impact; uninstalling a Provider mapping makes its Steps fail closed.
