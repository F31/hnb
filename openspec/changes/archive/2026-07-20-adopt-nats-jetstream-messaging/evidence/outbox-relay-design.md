# Outbox Relay: Transactional Outbox Pattern

## Overview

The Outbox Relay implements the Transactional Outbox pattern for HNB Cloud's NATS JetStream messaging backbone. Business facts and their corresponding Outbox events are committed in the same database transaction, ensuring that either both are persisted or neither is.

## Components

```
API Request
     |
     v
[Database Transaction]
     |
     |-- 1. Write business fact (Operation, Step result, etc.)
     |-- 2. Write OutboxEvent record (Pending state)
     |
     v
[Transaction Commit]
     |
     v
Outbox Relay (polling or LISTEN/NOTIFY)
     |
     |-- 3. Read Pending OutboxEvent with stable Message ID
     |-- 4. Publish to NATS JetStream
     |-- 5. On JetStream acknowledge: mark OutboxEvent as Published
     |-- 6. On failure: increment attempt, set next_attempt_at, keep Pending
     |
     v
[OutboxEvent Published]
```

## OutboxEvent Record

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Primary key |
| message_id | UUID | Stable message ID (also used as JetStream dedup ID) |
| message_type | TEXT | Versioned message type (e.g., `hnb.command.operation.step-requested.v1`) |
| schema_version | TEXT | Semantic version of payload schema |
| occurred_at | TIMESTAMPTZ | When the business fact occurred |
| tenant_id | TEXT | Tenant scope |
| correlation_id | UUID | Trace correlation |
| idempotency_key | TEXT | Idempotency key for consumer-side dedup |
| operation_id | UUID | (optional) Associated operation |
| step_id | UUID | (optional) Associated step |
| aggregate_id | TEXT | (optional) Aggregate identifier |
| aggregate_version | INTEGER | (optional) Expected version for optimistic concurrency |
| payload | JSONB | Message payload |
| payload_ref | TEXT | (optional) Reference to external payload |
| status | TEXT | `pending` / `published` / `failed` |
| attempt | INTEGER | Publish attempt count |
| max_attempts | INTEGER | Maximum publish attempts before manual intervention |
| next_attempt_at | TIMESTAMPTZ | When to retry publishing |
| last_error | TEXT | Last publish error message |
| created_at | TIMESTAMPTZ | Record creation time |
| updated_at | TIMESTAMPTZ | Last update time |

## Stable Message ID

- `message_id` is generated once when the OutboxEvent is created (within the business transaction).
- It is used as the JetStream `MsgId` header for deduplication within the configured `duplicateWindow`.
- If the Relay crashes after publishing but before marking as Published, the redelivery uses the same `message_id` and JetStream deduplicates it.

## Retryable Publish State Machine

```
Pending
  |-- publish success --> Published
  |-- transient error --> Pending (increment attempt, set next_attempt_at with backoff)
  |-- max_attempts exceeded --> Failed (requires manual intervention)
```

- Backoff: exponential with jitter: `base * 2^attempt + random(0, base)`
- Base interval: 5 seconds
- Max interval: 300 seconds (5 minutes)
- Max attempts: 10 (configurable)

## Query for Pending Events

```sql
SELECT * FROM outbox_events
WHERE status = 'pending'
  AND next_attempt_at <= now()
  AND attempt < max_attempts
ORDER BY next_attempt_at ASC
LIMIT 100
FOR UPDATE SKIP LOCKED;
```

## Graceful Shutdown

1. Signal SIGTERM/SIGINT to Outbox Relay
2. Stop accepting new pending events from database
3. Wait for in-flight publishes to complete (or timeout after 30s)
4. Events that were published but not marked as Published will be re-published after restart (idempotent due to stable Message ID)

## Test Plan

### Unit Tests
- Transactional outbox: business fact + OutboxEvent in same transaction
- Transaction rollback: neither business fact nor OutboxEvent persisted
- Stable Message ID: same message_id survives relay restart
- Publish retry: exponential backoff with jitter
- Max attempts: transition to Failed state

### Integration Tests
- Outbox Relay crash after publish, before mark: no duplicate business effect
- Concurrent relay instances: no double-publish (SKIP LOCKED)
- Database connection loss: retry with backoff