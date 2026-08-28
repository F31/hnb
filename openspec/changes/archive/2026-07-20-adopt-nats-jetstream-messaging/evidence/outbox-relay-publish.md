# Outbox Relay: Publish Confirmation, Backoff, Backpressure, and Graceful Shutdown

## Publish Confirmation Flow

1. Outbox Relay reads Pending events from database (FOR UPDATE SKIP LOCKED)
2. For each event:
   a. Build NATS message with Envelope fields
   b. Set `MsgId` header to `message_id` for JetStream dedup
   c. Publish to JetStream with `PublishAsync` (non-blocking)
   d. Wait for `PubAck` future:
      - Success: update OutboxEvent status to `published`
      - Failure: increment attempt, set `next_attempt_at`, keep `pending`
3. After batch processing, commit database transaction

## Backoff Strategy

```go
func nextAttempt(attempt int) time.Duration {
    base := 5 * time.Second
    max := 300 * time.Second
    delay := time.Duration(math.Min(
        float64(base)*math.Pow(2, float64(attempt)),
        float64(max),
    ))
    // Add jitter: ±25%
    jitter := time.Duration(float64(delay) * (0.75 + rand.Float64()*0.5))
    return jitter
}
```

| Attempt | Backoff (approx) |
|---------|------------------|
| 0 | 5s |
| 1 | 10s |
| 2 | 20s |
| 3 | 40s |
| 4 | 80s |
| 5 | 160s |
| 6+ | 300s (capped) |

## Backpressure Throttling

| Condition | Action |
|-----------|--------|
| Pending events > 1000 | Log warning, increase poll interval |
| Pending events > 5000 | Halt new API writes (503), alert |
| Publish error rate > 10% | Backoff poll interval, log errors |
| Outbox age > 5 min | Alert (potential broker issue) |

## Graceful Shutdown Sequence

1. Receive SIGTERM/SIGINT
2. Stop polling database for new pending events
3. Wait for in-flight publish operations to complete (max 30s)
4. Flush any pending JetStream async publishes
5. Complete the current database transaction
6. Exit with code 0 if all in-flight events were published, code 1 if some remain pending

## Database Schema for Publish Tracking

```sql
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 10;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending'
  CHECK (status IN ('pending', 'published', 'failed'));
```

## Test Plan

### Integration Tests
- Publish succeeds: OutboxEvent transitions to `published`
- Publish fails transiently: OutboxEvent stays `pending`, attempt incremented
- Max attempts exceeded: OutboxEvent transitions to `failed`
- Relay crash after publish, before mark: re-publish is idempotent (stable MsgId)
- Backpressure: pending event count > 1000 triggers slower polling
- Graceful shutdown: in-flight publishes complete; pending events remain pending for next relay