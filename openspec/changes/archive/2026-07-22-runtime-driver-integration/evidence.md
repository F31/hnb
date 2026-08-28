## Verification Evidence

Date: 2026-07-22

### Contract And Unit Tests

Command:

```text
GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go test -count=1 ./...
```

Result: all Operation Worker packages passed, including configuration parsing and HTTP Runtime Driver routing, success, resumable failure, protocol rejection, response bounds, and cancellation tests.

Command:

```text
GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go test -race -count=1 ./internal/config ./internal/driver
```

Result: both changed packages passed under the race detector.

Command:

```text
GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go vet ./...
```

Result: passed with no findings.

### Configured Startup Smoke Test

Temporary PostgreSQL 16 and NATS 2.10.29 JetStream containers were started. Migration `001_nats_jetstream_outbox.sql` initialized the Worker bootstrap tables. The Worker was started with:

```text
RUNTIME_PROVIDERS={"k8s-prod":"http://127.0.0.1:18080/v1/steps:execute"}
```

Observed result:

```text
operation-worker starting
[worker-...] starting worker
[worker-...] listening on shared consumer operation-worker
shutting down...
operation-worker stopped
```

The configured endpoint was intentionally not invoked because the smoke test dispatched no Step. SIGTERM produced a graceful shutdown. Both temporary containers were removed afterward.

### OpenSpec And Formatting

Commands:

```text
openspec validate --all --strict
git diff --check
```

Result: 25 OpenSpec items passed with 0 failures; diff whitespace validation passed.

## N/A Assessments

- Database migration: N/A. Runtime Provider mappings are process configuration and no persisted Operation or event fields changed.
- New event/schema generation: N/A. The Provider HTTP contract is internal to this change and existing Step events remain authoritative.
- Concrete target-mutation E2E: N/A until a Kubernetes, container, or edge Provider implements the v1 contract and passes Provider-side idempotency/fencing conformance. Reporting a target mutation now would require a simulated Provider and violate the fail-closed objective.
- Backup/restore: N/A. The adapter owns no durable state.

## Rollout And Rollback

Rollout configures an exact Provider execution URL, grants only required egress, starts the Worker, and canaries one Operation after the concrete Provider passes conformance. Rollback removes the mapping or restores the previous Worker image; unknown Provider Steps fail closed and remain governed by existing retry/terminal failure policy. No data rollback is required.
