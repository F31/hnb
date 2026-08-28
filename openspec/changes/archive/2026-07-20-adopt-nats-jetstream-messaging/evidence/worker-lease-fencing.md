# Worker Lease, Fencing Token, Checkpoint, and Stale Message Handling

## Worker Lease

Each Operation Step Worker acquires a lease before executing the step. The lease:
- Is associated with a specific `step_id`
- Has a unique `fencing_token` (UUID v4)
- Has an `expires_at` timestamp
- Is acquired atomically via database INSERT with unique constraint

### Lease Acquisition

```sql
INSERT INTO worker_leases (step_id, owner_id, fencing_token, expires_at)
VALUES ($1, $2, gen_random_uuid(), now() + interval '30 seconds')
ON CONFLICT (step_id) DO UPDATE
SET fencing_token = gen_random_uuid(),
    owner_id = $2,
    expires_at = now() + interval '30 seconds',
    version = worker_leases.version + 1
WHERE worker_leases.expires_at < now();
```

- If insert succeeds: lease acquired, proceed with execution
- If insert conflicts (existing valid lease): another worker holds the lease, skip

### Lease Renewal

During long-running steps, the worker renews the lease periodically:

```sql
UPDATE worker_leases
SET expires_at = now() + interval '30 seconds',
    version = version + 1
WHERE step_id = $1
  AND fencing_token = $2
  AND expires_at > now();
```

- If zero rows updated: lease lost (fencing token mismatch or expired)
- Worker must stop producing new side effects when lease is lost

## Fencing Token

The fencing token prevents split-brain scenarios:

1. Worker A acquires lease with fencing_token = `abc-123`
2. Worker A calls external Provider API
3. Worker A crashes before ACK
4. Worker B acquires lease (token rotated to `def-456`)
5. Worker B checks Provider state, finds operation already completed (idempotency)
6. Worker B updates checkpoint, ACKs message

All writes to the Operation Store include the fencing_token:

```sql
UPDATE operation_steps
SET state = 'succeeded',
    checkpoint = $1,
    fencing_token = $2,
    updated_at = now()
WHERE id = $3
  AND (fencing_token IS NULL OR fencing_token = $2);
```

If fencing_token doesn't match, the update is rejected — preventing stale workers from corrupting state.

## Checkpoint

Checkpoints track execution progress within a step:

- Stored as JSONB in `operation_steps.checkpoint`
- Represents the last known good state before potential failure
- On re-delivery, the worker reads the checkpoint and resumes from there
- Checkpoints are updated atomically with the step state

### Checkpoint Schema

```json
{
  "lastCompletedAction": "provisioned_compute",
  "resources": ["res-1", "res-2"],
  "observedState": "halfway",
  "version": 3
}
```

## Stale Message No-Op

When a worker receives a message for a terminal operation:

```sql
SELECT state FROM operations WHERE id = $1;
```

If `state IN ('succeeded', 'failed', 'cancelled')`:

1. Log "stale message received for terminal operation" with message_id, operation_id
2. Increment stale_message counter (metric)
3. ACK the message (do not redeliver)
4. Do not execute any Provider call
5. Do not write any database state

## Lease Lifecycle States

```
Available --> Acquired (by Worker A)
  |              |
  |              +--> Expired (lease timeout)
  |              |       |
  |              |       v
  |              |   Available (can be acquired by Worker B)
  |              |
  |              +--> Released (step completed)
  |                      |
  |                      v
  |                  Available
  |
  +--> Never acquired (step not yet scheduled)
```

## Test Plan

### Unit Tests
- Lease acquisition: success, conflict (valid lease), conflict (expired lease)
- Lease renewal: success, lost token, expired lease
- Fencing token: write rejected with wrong token
- Checkpoint: save, read, resume from checkpoint
- Stale message: no-op for terminal operation, ACK without side effects

### Integration Tests
- Two workers competing for same step: only one succeeds
- Worker crashes mid-execution: lease expires, re-delivery, recovery
- Stale command after operation completed: no-op, no Provider call
- Concurrent operations: independent leases per step