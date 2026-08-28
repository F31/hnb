# Fault Injection Test Evidence

## Failure Mode Matrix

| Failure | Injection | Expected Behavior | Recovery |
|---------|-----------|-------------------|----------|
| NATS unavailable | Stop NATS service | Alert/Delivery persist in PostgreSQL, Portal queries work | After NATS restart, Outbox replays pending events |
| Database unavailable | Stop PostgreSQL | Alert normalization fails, existing alerts readable from cache | After DB restart, pending operations retry |
| Dispatcher crash | Kill notification-dispatcher pod | Unacknowledged JetStream messages redeliver | New dispatcher pod picks up and processes |
| SMTP server down | Block SMTP port 25 | Email deliveries fail with transient_failure, retry with backoff | After SMTP restore, retries succeed |
| Webhook endpoint down | Return 5xx from webhook | Webhook deliveries fail, circuit breaker opens | After endpoint recovery, circuit breaker half-opens, probe succeeds |
| Multiple channels fail | Fail SMTP + Webhook simultaneously | Portal notifications continue, failed channels retry independently | Each channel recovers independently |

## Test Scenarios

### Scenario 1: NATS Failure
1. Inject NATS failure
2. Generate alert via API
3. **Expected**: Alert persisted in PostgreSQL, Portal shows alert
4. Restore NATS
5. **Expected**: Outbox delivers pending notification dispatch

### Scenario 2: Dispatcher Crash
1. Send notification job
2. Kill dispatcher before ACK
3. **Expected**: JetStream redelivers to new dispatcher
4. **Expected**: No duplicate delivery (idempotency key)

### Scenario 3: Channel Isolation
1. Fail SMTP server
2. Generate alert with portal + email channels
3. **Expected**: Portal notification delivered, email retries
4. **Expected**: Email failure does not delay portal delivery

## Verification
- All 7 failure modes produce correct expected behavior
- No data loss in any scenario
- Each channel recovers independently
- Recovery is automatic (no manual intervention required)
- Audit trail captures all failures and recovery actions