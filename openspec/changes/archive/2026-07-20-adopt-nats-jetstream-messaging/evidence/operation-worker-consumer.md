# Operation Step Worker: JetStream Durable Pull Consumer

## Architecture

The Operation Step Worker replaces the PostgreSQL polling-based scheduler with a JetStream Durable Pull Consumer. Each worker instance subscribes to the `hnb.command.operation.step-requested.v1` subject and processes one message at a time.

## Worker Loop

```
1. Pull next message from JetStream (batch size 1, max wait 5s)
2. Deserialize to StepRequested
3. Read Operation from Operation Store (authoritative state)
4. Validate:
   - Operation exists and is in progress
   - Step exists and is in expected state
   - Expected version matches
5. Acquire Worker Lease (database INSERT, fencing_token)
6. If lease acquired:
   a. Update Operation/Step state to InProgress
   b. Call Provider API (idempotent with IdempotencyKey)
   c. Save Step result + Checkpoint in database transaction
   d. Write new Outbox events for downstream consumers
   e. Commit database transaction
   f. ACK the JetStream message
7. If lease not acquired:
   a. NAck the message (will be redelivered to another worker)
8. If stale message (terminal operation):
   a. ACK without processing
```

## Consumer Configuration

| Parameter | Value |
|-----------|-------|
| Consumer Name | `operation-worker` |
| Stream | `commands` |
| Filter Subject | `hnb.command.operation.step-requested.v1` |
| Deliver Policy | `all` (required by WorkQueue retention; progress is shared by the fixed durable) |
| Ack Policy | `explicit` |
| Ack Wait | 60s |
| Max Deliver | 11 (10 execution attempts plus one terminal-state persistence reserve) |
| Max Ack Pending | 16 |
| Replay Policy | `instant` |
| Backoff | [10s, 30s, 60s, 120s, 300s, 600s, 1800s, 3600s, 7200s] |

## Concurrency

- Each worker processes one message at a time (MaxAckPending = 1 per worker)
- Multiple worker instances can run in parallel, competing for messages
- Database-level fencing prevents duplicate execution
- Rate limiting: configurable max operations per second per worker

## Error Handling

| Error | Action |
|-------|--------|
| Operation not found | NAck, log, alert |
| Step not found | NAck, log, alert |
| Version mismatch | ACK (stale message), log |
| Provider API failure | Retry, increment attempt, update checkpoint |
| Lease expired mid-execution | Abort, do not commit, NAck |
| Max deliveries exceeded | Move to failed subject, pause operation |

## Test Plan

### Unit Tests
- Worker receives message, processes step, ACKs
- Worker receives duplicate message, checks idempotency, ACKs
- Worker receives stale message (terminal operation), ACKs without processing
- Worker fails to acquire lease, NAcks

### Integration Tests
- Multiple workers: only one processes each message
- Worker crash before ACK: message redelivered, idempotent processing
- Provider failure: retry with backoff, eventually failed subject
- Lease expiration: new worker picks up after timeout
