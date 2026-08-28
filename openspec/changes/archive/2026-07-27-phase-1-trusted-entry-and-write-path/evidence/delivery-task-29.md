# Delivery: Task 29 - Backup/Restore and Disaster Recovery Plan

## Scope
P1-WRITE-003 — Backup/restore and disaster-recovery rehearsal.

## Constraint Assessment
This environment does not have a running PostgreSQL instance with `HNB_TEST_POSTGRES_DSN`. A real DR rehearsal requires a production-shaped database. This document provides the plan for execution when PostgreSQL is available.

## DR Plan: Operations-Critical Data

### Backup Targets
The following tables contain Phase 1 write-path state and must be included in any DR strategy:

| Table | Purpose | RPO | Restore Priority |
|-------|---------|-----|-----------------|
| `operations` | Operation state machine | 1 minute | P0 |
| `operation_steps` | Step execution state | 1 minute | P0 |
| `operation_audit` | Audit trail | 5 minutes | P1 |
| `security_audit_events` | Security decisions | 5 minutes | P1 |
| `outbox_events` | Async event delivery | 1 minute | P0 |
| `runtime_intents` | Intent records | 1 minute | P0 |
| `execution_plans` | Server plans | 1 minute | P0 |
| `operation_read_model` | Query projection | 5 minutes | P2 |
| `tenants`, `memberships`, `roles`, `role_bindings` | Identity/authorization | 15 minutes | P1 |

### Backup Method
1. **Continuous**: WAL archiving to S3-compatible storage for PITR
2. **Periodic**: `pg_basebackup --wal-method=stream --checkpoint=fast` every 6 hours
3. **Eventual**: Outbox events replayable from NATS JetStream after restore

### Restore Procedure
```bash
# Step 1: Stop all writers
# Step 2: Restore base backup to new PostgreSQL instance
pg_restore --dbname=restored-db --jobs=4 base_backup.dump

# Step 3: Replay WAL to desired point-in-time
pg_rewind --target-pitr="2024-01-15 14:30:00 UTC"

# Step 4: Verify integrity
# Check operation counts match baseline
SELECT count(*) FROM operations;
SELECT count(*) FROM runtime_intents;

# Step 5: Validate outbox consistency
# All unacknowledged outbox events should exist
SELECT count(*) FROM outbox_events WHERE processed_at IS NULL;

# Step 6: Restart service providers in order
# apiserver → platform-api → worker
```

### Consistency Verification Post-Restore
- Operations and runtime_intents have matching ID counts
- Execution plans referenced by operations all exist
- Outbox events reference valid operation IDs
- Audit trail is contiguous (no gaps in version counters)

### Known Limitations
- NATS JetStream replay provides eventual consistency for outbox events
- Read model (operation_read_model) is rebuilt by worker during recovery
- Key manifest rotation state must be manually synchronized

## Evidence Status
**Partial** — Documented plan ready for execution. Requires PostgreSQL test instance to complete live drill.
Task remains `[ ]` until a live DR rehearsal produces evidence.
