## Why

UUID lease tokens prevent stale database commits but cannot order external attempts. If a Provider creates a resource and the Worker crashes before committing, redelivery receives a different UUID and currently conflicts with the already-created resource instead of safely adopting it.

## What Changes

- Change ID: `operation-fencing-v2`.
- **BREAKING** replace Runtime Driver contract v1 with v2 after a stopped-dispatch cutover; mixed v1/v2 execution is prohibited.
- Add a PostgreSQL global, non-cycling fencing generation and retain each Step's greatest granted generation after Lease release.
- Keep an opaque UUID execution attempt ID separate from the ordered generation and require both on DB renewal/commit and Provider responses.
- Let a higher generation CAS-adopt an identical Kubernetes Deployment after an ambiguous failure; reject lower generations and strictly replay equal generations.
- Replace physical Deployment deletion with a resourceVersion-CAS logical tombstone (`replicas=0`) requiring `expected_uid`, preventing delayed deploy resurrection.
- Add deterministic PostgreSQL/NATS/HTTP/fake-client/kind failure injection for the external-success-before-DB-commit window.
- Classification: T0 Operation safety protocol plus T1 Kubernetes Provider conformance; affected plane is runtime governance only.
- Dependencies: completed `operation-engine-core`, `runtime-driver-integration`, and `kubernetes-runtime-provider` changes.
- Migration: additive migration 013 with dispatch stopped, no active leases, v1 resource inventory, then Provider/Worker v2 canary.
- Rollback: rollback to v1 is allowed only before v2 target writes; after activation recovery is roll-forward because v1 cannot preserve generations.
- User value: retries converge on one resource and one committed Step rather than reporting false failure after a successful external effect.
- Non-goals: Platform API, Provider Registry, mTLS, pause/cancel, DAG continuation, secrets, generic Deployment update, and physical deletion.
- Compatibility/security: v1 requests are rejected; malformed or stale generations fail closed. Generation is authorization ordering, not caller authentication; mTLS remains a separate required change.
- Resource budget: one BIGINT sequence allocation per acquisition and bounded Kubernetes CAS retries; no new middleware or database.
- Observability: audit records and Provider annotations include attempt ID and generation; errors distinguish fenced, permanent, and transient outcomes.
- Exit criteria: complete 001-013 migration chain, rollback guards, race tests, real kind takeover/tombstone tests, strict OpenSpec validation, and no residual test resources.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `composition-operation`: Step execution gains persistent monotonic fencing and ambiguous-effect recovery.
- `runtime-driver-execution`: wire contract v2 carries and echoes attempt ID plus generation with typed failure codes.
- `kubernetes-runtime-provider`: Deployment writes enforce generation CAS and logical tombstone deletion.

## Impact

- Database: migration 013 changes Worker Lease and Operation Step fencing columns and adds one sequence/FK.
- Code: Operation Store, Worker, HTTP Runtime Driver, Kubernetes Provider, tests, RBAC, and Provider manifest.
- Events: StepRequested and domain-event schemas do not change because generation is allocated after command delivery.
- Install/upgrade: coordinated stopped-dispatch cutover; no new backup system. PostgreSQL backup automatically includes the sequence and retained generations.
- Uninstall/restore: stateless binaries can be removed, but v2-managed resources and generation state must be retained for safe roll-forward recovery.
