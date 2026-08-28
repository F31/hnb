# Evidence: operation-engine-core

## Architecture Baseline
Frozen architecture V3.8.6 defines:
- **Operation Engine** as the sole write path (KERNEL-002)
- **Read Model** for query/control decoupling (KERNEL-003)
- **T0 kernel** excludes specific Provider implementations (KERNEL-001)
- OP-007 requires transactional Outbox + NATS JetStream and authoritative database fencing

## Database Artifacts
| Artifact | Description |
|----------|-------------|
| `008_operation_engine_core.sql` | 7 tables: execution_plans, operations (10-state), operation_steps, step_checkpoints, compensation_records, operation_audit, operation_read_model |
| `008_operation_engine_core.rollback.sql` | Full rollback |

## Proto Contract Artifacts
| Message | Type |
|---------|------|
| StepRequested (26) | EventEnvelope oneof payload |
| StepCompleted (27) | EventEnvelope oneof payload |
| OperationStateChanged (28) | EventEnvelope oneof payload |
| OperationProgress (29) | EventEnvelope oneof payload |

## Go Code Artifacts

### Engine Library (`internal/engine/`)
| File | LOC | Key Types |
|------|-----|-----------|
| `state.go` | 90 | OperationStatus (10 states), StepStatus, validTransitions matrix (18 entries), CanTransition, IsTerminal |
| `plan.go` | 66 | ExecutionPlan, StepSpec, OutputBinding, RetryPolicy, PolicyResult |
| `dag.go` | 109 | DAGResolver (topological sort, execution levels, ready steps), OutputResolver |
| `step.go` | 81 | StepExecution, ExecutionContext, StepResult, Operation, NewIdempotencyKey |
| `compensation.go` | 79 | CompensationEngine, 8 default strategies (database/volume/backup retain, deployment/configmap/service/secret rollback) |
| `audit.go` | 67 | AuditEntry, AuditEvidence, 13 event types |
| `engine.go` | 112 | OperationStateMachine, PlanGenerator, EvaluateStepCompletion |
| `engine_test.go` | 278 | 17 tests covering all components |

### Store Layer (`internal/store/`)
| File | Key Methods |
|------|-------------|
| `operations.go` | AcquireLease, GetStepState, CommitStepSuccess (fenced atomic Step/Operation/Audit/ReadModel/Outbox transaction) |
| `plans.go` | SavePlan, GetPlan, GetPlanByDigest with UUID-compatible IDs and full-plan decoding |

### NATS Consumer (`internal/nats/`)
| File | Key Methods |
|------|-------------|
| `worker.go` | Shared durable consumer, contract validation, failed-subject isolation, fail-closed StepRunner dispatch |
| `relay.go` | Transactional Outbox polling, stable JetStream message IDs, PubAck, retry and terminal failure state |

### Config & Main
| File | LOC |
|------|-----|
| `config.go` | 48 |
| `main.go` | 51 |

### Total: 12 Go source files, ~1,300 LOC

## Test Results
```
=== RUN   TestCanTransition                --- PASS
=== RUN   TestIsTerminal                   --- PASS
=== RUN   TestStateMachine_CreateOperation --- PASS
=== RUN   TestStateMachine_Transition      --- PASS
=== RUN   TestStateMachine_InvalidTransition --- PASS
=== RUN   TestStateMachine_Approve         --- PASS
=== RUN   TestPlanGenerator_Validate       --- PASS
=== RUN   TestPlanGenerator_ValidateCycle  --- PASS
=== RUN   TestDAGResolver_Resolve          --- PASS
=== RUN   TestDAGResolver_ExecutionLevels  --- PASS
=== RUN   TestDAGResolver_ReadySteps       --- PASS
=== RUN   TestOutputResolver               --- PASS
=== RUN   TestCompensationEngine_Defaults  --- PASS
=== RUN   TestCompensationEngine_Register  --- PASS
=== RUN   TestNewIdempotencyKey            --- PASS
=== RUN   TestComputePlanDigest            --- PASS
=== RUN   TestEvaluateStepCompletion       --- PASS
PASS: 17/17
```

## P0 Reliability Verification (2026-07-22)

Environment:
- Go `1.24.5` downloaded toolchain (the host `/usr/local/go` 1.24.2 standard library is corrupt)
- PostgreSQL `16-alpine`, empty database
- NATS Server `2.10.29` with JetStream

Verified behavior:
- The complete `001` through `012` forward migration chain succeeds on an empty database.
- Two distinct Worker instances resolve to the same `operation-worker` durable consumer.
- A second Worker cannot acquire an unexpired Step lease.
- A wrong fencing token rolls back without changing Step/Operation state or creating an Outbox row.
- A valid token commits Step, Operation progress, Audit, Read Model, lease release, and Outbox atomically.
- Long-running Step execution renews both the database lease and JetStream acknowledgement deadline.
- Runner context is loaded from authoritative Step type, Provider, input, checkpoint, retry, timeout, and dependency fields.
- Retryable failures persist output/checkpoint and increment retry count under fencing before releasing the lease.
- Terminal failures atomically persist failed Step/Operation state plus domain and failed-subject Outbox events.
- Commands dispatched before DAG prerequisites are ready fail closed instead of occupying shared Consumer capacity.
- The Relay publishes the stored envelope with the stable `Nats-Msg-Id` and marks the row `published` only after PubAck.
- Operation and ExecutionPlan generators produce PostgreSQL-compatible UUIDs.
- No `simulated_ok` success path remains; a missing Runtime Driver retries and isolates instead of committing success.

Commands:
```text
HNB_TEST_POSTGRES_DSN=... HNB_TEST_NATS_URL=... GOTOOLCHAIN=go1.24.5 go test -count=1 ./...
HNB_TEST_POSTGRES_DSN=... HNB_TEST_NATS_URL=... GOTOOLCHAIN=go1.24.5 go test -race -count=1 ./internal/nats ./internal/store
GOTOOLCHAIN=go1.24.5 go vet ./...
```

Results: all packages PASS; race-enabled NATS and Store integration tests PASS; `go vet` reports no findings.
