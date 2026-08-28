## Context

The Worker currently uses a random UUID as both Lease identity and external fence. Database predicates reject stale commits, but an external effect followed by a Worker crash cannot be ordered against the next attempt. The Kubernetes Provider therefore rejects the new UUID and turns an already-created resource into a false terminal failure.

## Goals / Non-Goals

**Goals:** persist a global monotonic generation, separate attempt identity from ordering, enforce both at DB boundaries, upgrade the HTTP contract to v2, safely CAS-adopt ambiguous Kubernetes effects, and prevent stale deploy resurrection after delete.

**Non-Goals:** Platform API, registry, transport authentication, DAG continuation, pause/cancel, secrets, generic updates, or physical deletion.

## Architecture

```text
PostgreSQL sequence -> Lease {attempt UUID, generation}
        |                         |
        v                         v
operation_steps retained G -> Worker -> HTTP v2 -> Kubernetes Provider
                                                    |
                                                    v
                                      resourceVersion CAS + annotations
```

PostgreSQL remains the Operation authority; Kubernetes annotations enforce ordering only at the external side-effect boundary. Provider and Worker share no database and no execution bypass is added.

## Data Model

- `operation_fencing_generation_seq`: global BIGINT, positive, `NO CYCLE`.
- `worker_leases.lease_id`: opaque UUID active-attempt identity.
- `worker_leases.fencing_generation`: generation for the active Lease.
- `operation_steps.last_lease_id`: last granted attempt for audit.
- `operation_steps.fencing_generation`: greatest generation granted to that Step, retained after Lease deletion.
- Foreign key from `worker_leases.step_id` to `operation_steps.id`.

A global sequence is used instead of a per-Step counter because different Steps can act on the same external resource. Sequence gaps are valid; values are never reset or reused.

## API Contract

Runtime Driver accepts only schema `2.0.0`. The execution request replaces `fencing_token` with `execution_attempt_id` and a canonical positive decimal-string `fencing_generation`. Every response echoes both fields. Failures also carry one of `INVALID_REQUEST`, `SCOPE_DENIED`, `UNSUPPORTED_ACTION`, `RESOURCE_CONFLICT`, `FENCED`, `TARGET_UNAVAILABLE`, `CANCELLED`, or `INTERNAL` plus a retry hint. The Driver validates identity/generation and owns final error classification.

NATS StepRequested remains unchanged because generation is allocated only after delivery and authoritative Lease acquisition.

## State Machine

```text
acquire G -> call Provider -> success -> fenced DB commit
                     | crash/lost response
                     v
redelivery acquire G+N -> CAS adopt -> fenced DB commit

Provider: stored G > request G => FENCED
          stored G = request G => exact replay only
          stored G < request G => identity/spec checked CAS takeover
```

## Decisions

### Transactional Lease acquisition

Acquisition locks Operation and Step, validates runnable state/version, allocates a generation, acquires or takes over an expired Lease, and persists the Step generation in one transaction. Renewal, retry, and commit require owner, attempt ID, generation, expiry, and retained Step generation.

### Kubernetes deploy takeover

Create uses annotations containing generation, attempt, action, tenant, Operation, Step, and idempotency identity. `AlreadyExists` is re-read. Higher-generation takeover is allowed only for the same logical deploy and exact desired spec, using `resourceVersion` CAS. Availability polling aborts if a newer generation appears.

### Logical tombstone delete

Physical deletion removes the only target-side fence and lets a delayed old deploy recreate the name. Delete therefore requires `expected_uid`, CAS-updates generation/action, scales replicas to zero, waits for observation, and retains the Deployment. A future redeploy must use a higher generation and the tombstone UID; generic update remains out of scope.

### Stopped-dispatch cutover

No v1 compatibility path is added. Dispatch and Workers stop, active Leases drain, migration 013 runs, then Provider and Worker v2 deploy together. This minimizes code and avoids an unverifiable mixed-protocol safety state.

## Failure Modes

- Active Lease conflict may burn a sequence value; the gap is harmless.
- Lease/heartbeat loss cancels the call, but correctness relies on generation/CAS rather than prompt cancellation.
- Wrong echoed attempt/generation is a protocol failure and cannot commit success.
- Lower generation is `FENCED`, never a business failure written with a stale Lease.
- Kubernetes 409/AlreadyExists triggers bounded reread and classification.
- Missing/malformed managed-resource generation fails closed.

## Security And Operations

Tenant/Operation/Step/idempotency ownership remains mandatory. Generation prevents stale writers but does not authenticate callers; mTLS is a separate change. Inputs and bodies remain unlogged, no Secret behavior changes, and no new supply-chain component is introduced. Cost is one sequence allocation per acquisition and bounded CAS retries. Audit records both values; Provider annotations expose only control metadata. PostgreSQL backup/restore includes sequence state. Sequence exhaustion or reset is fatal. Deployment permission changes from delete to update. Metrics remain outside this change.

## Compatibility Matrix And Conformance

| Component | Required version |
|---|---|
| Worker / Runtime Driver | 2.0.0 only |
| Kubernetes Provider | 2.0.0 only |
| PostgreSQL schema | 013 |
| Kubernetes | kind v1.36.1 validated |

Conformance injects crashes/lost Leases after external create, delayed stale requests, concurrent takeover, CAS conflicts, response mismatch, and tombstone retries. It asserts one resource, one Step transition, one progress increment, one audit completion, and one completion Outbox event.

## Risks / Trade-offs

- [Logical delete retains objects] -> label tombstones, scale to zero, expose action/checkpoint, and defer garbage collection until a durable admission-enforced fence ledger exists.
- [Global sequence allocation is not commit order across unrelated Steps] -> safety requires a total unique order, not wall-clock order; same-resource Provider CAS selects the greater value.
- [v2 activation cannot safely roll back to v1] -> canary before dispatch; after first v2 write use roll-forward recovery.
- [Existing v1 resources lack generation] -> inventory/migrate offline or fail closed; the current validated kind target is clean.

## Migration Plan

1. Stop dispatch and all v1 Workers; verify no active Lease or ambiguous nonterminal v1 Step.
2. Inventory v1 Provider-managed resources; migrate offline or remove test resources.
3. Apply migration 013 and verify sequence/FK/columns.
4. Deploy v2 Provider, then v2 Worker, while dispatch remains stopped.
5. Run protocol probes and one canary; verify DB, annotations, audit, and Outbox.
6. Resume dispatch. Before any v2 target write, rollback migration is allowed with an empty Lease table; afterward rollback is roll-forward only.

## Open Questions

None. Logical tombstones, `expected_uid`, and stopped-dispatch cutover are approved.
