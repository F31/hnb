# Tasks: operation-engine-core

## Summary
| | |
|---|---|
| **Change** | `operation-engine-core` |
| **Created** | 2026-07-21 |
| **Specs** | composition-operation (OP-001~OP-006), platform-kernel (KERNEL-001~KERNEL-004) |
| **Status** | Implemented |

## Task List

### T1: Database Migration — Operation Engine Core Tables
- [x] 008_operation_engine_core.sql: execution_plans, operations (10 states), operation_steps, step_checkpoints, compensation_records, operation_audit, operation_read_model
- [x] Rollback SQL

**Files:** `database/postgresql/migrations/008_operation_engine_core.sql`, `database/postgresql/migrations/008_operation_engine_core.rollback.sql`

### T2: Proto Contract Update — Operation Events
- [x] Add StepRequested(26), StepCompleted(27), OperationStateChanged(28), OperationProgress(29) to EventEnvelope oneof

**File:** `contracts/proto/hnb/contracts/v1/contracts.proto`

### T3: ExecutionPlan Engine
- [x] ExecutionPlan data model (immutable, digest, steps, outputs, policy)
- [x] PlanGenerator.GeneratePlan() — from ReleaseManifest to immutable plan
- [x] PlanGenerator.ValidatePlan() — DAG cycle detection, policy check
- [x] ComputePlanDigest() — SHA-256 of serialized plan
- [x] PlanStore — DB persistence (SavePlan, GetPlan, GetPlanByDigest)

**Files:** `cmd/operation-worker/internal/engine/plan.go`, `cmd/operation-worker/internal/engine/engine.go`, `cmd/operation-worker/internal/store/plans.go`

### T4: Operation 10-State Machine
- [x] 10 states: Pending, PendingApproval, Queued, QueuedOffline, InProgress, Paused, Compensating, Succeeded, Failed, Cancelled
- [x] Complete transition matrix (18 valid transitions)
- [x] Terminal state detection
- [x] Approve() workflow
- [x] OperationCreate — type validation, idempotency key generation
- [x] OperationStore — Create/Get/UpdateStatus

**Files:** `cmd/operation-worker/internal/engine/state.go`, `cmd/operation-worker/internal/store/operations.go`

### T5: Step Executor (Idempotent + Checkpoint + Retry)
- [x] StepIdempotencyKey — deterministic SHA-256 hash of operation_id + step_id
- [x] StepResult model with status, outputs, checkpoint, error
- [x] Worker lease/fencing via worker_leases table
- [x] Step timeout and retry policy definitions

**Files:** `cmd/operation-worker/internal/engine/step.go`, `cmd/operation-worker/internal/worker/worker.go`, `cmd/operation-worker/internal/worker/lease.go`

### T6: DAG Orchestration Engine
- [x] DAGResolver.Resolve() — topological sort with cycle detection
- [x] DAGResolver.ExecutionLevels() — parallel execution levels
- [x] DAGResolver.ReadySteps() — dynamic readiness based on completed set
- [x] OutputResolver — cross-step output binding with JSONPath-like expressions

**File:** `cmd/operation-worker/internal/engine/dag.go`

### T7: Compensation Engine
- [x] Per-resource-type compensation strategies (database, volume, deployment, configmap, etc.)
- [x] Stateful resource protection (retain_mark with human approval)
- [x] Custom strategy registration
- [x] CompensationRecord model

**File:** `cmd/operation-worker/internal/engine/compensation.go`

### T8: Audit Module
- [x] 13 audit event types (created, approved, rejected, started, step_completed, etc.)
- [x] AuditEntry model with operation_id, actor_id, state transitions, detail
- [x] AuditEvidence — full evidence chain (initiator, approver, release, plan digest, steps, rollback)

**File:** `cmd/operation-worker/internal/engine/audit.go`

### T9: Read Model Projector
- [x] operation_read_model table — denormalized query projection
- [x] Transactional Read Model upsert — status, step counts, timestamps
- [x] ON CONFLICT DO UPDATE in the fenced Step commit transaction

**File:** `cmd/operation-worker/internal/store/operations.go`

### T10: Worker Binary
- [x] Config — env-based configuration (DB, NATS, lease duration)
- [x] Worker.Start() — shared durable JetStream consumer + message handler
- [x] handleMessage() — authoritative state, idempotency/version and lease checks
- [x] AcquireLease() — atomic fencing token acquisition with conflict detection

**Files:** `cmd/operation-worker/main.go`, `cmd/operation-worker/internal/config/config.go`, `cmd/operation-worker/internal/nats/worker.go`

### T11: Unit Tests
- [x] 17 tests covering: state machine transitions, plan validation, DAG resolution, output binding, compensation defaults, idempotency key, plan digest, step completion evaluation

**File:** `cmd/operation-worker/internal/engine/engine_test.go`

### T12: P0 Execution Reliability Closure
- [x] Align command/event subjects and envelope fields with the versioned messaging registry
- [x] Replace per-instance consumers with the shared `operation-worker` durable
- [x] Use UUID-compatible Operation and ExecutionPlan identifiers
- [x] Reject lease conflicts and require the active fencing token on Step result writes
- [x] Renew active leases and JetStream acknowledgement deadlines during long-running Steps
- [x] Load Step type, Provider, inputs, checkpoint, retry, timeout, and dependencies from authoritative storage
- [x] Persist retry checkpoint/output with fencing before releasing the lease for redelivery
- [x] Fail closed on out-of-order DAG commands instead of blocking the shared consumer
- [x] Commit Step, Operation progress, audit, Read Model, and Outbox in one transaction
- [x] Commit terminal Step failure, failed Operation state, domain event, and failed-subject event in one transaction
- [x] Create the Outbox table in the unpublished 001 baseline and add a retrying JetStream Relay
- [x] Publish poison/exhausted messages to `hnb.failed.>` before terminating delivery
- [x] Remove simulated success; execution is fail-closed until a Runtime Driver supplies StepRunner
- [x] Verify the complete 001-012 migration chain on an empty PostgreSQL database
- [x] Verify shared consumer, lease conflict, rollback, atomic commit, Relay, and race behavior

**Files:** `database/postgresql/migrations/001_nats_jetstream_outbox.sql`, `database/postgresql/migrations/008_operation_engine_core.sql`, `cmd/operation-worker/internal/store/operations.go`, `cmd/operation-worker/internal/nats/worker.go`, `cmd/operation-worker/internal/nats/relay.go`
