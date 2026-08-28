# Conformance Harness: Failure Mode Matrix

## Test Matrix

| Fault | Scenario | Expected Behavior |
|-------|----------|-------------------|
| ACK lost | JetStream does not receive ACK for a delivered message | Message redelivered after AckWait; worker checks idempotency |
| Duplicate delivery | Same message delivered twice to worker | Worker checks idempotencyKey; second delivery is no-op |
| Out-of-order delivery | Older message arrives after newer one | Worker checks expectedVersion; stale version is rejected |
| Outbox Relay crash | Relay crashes after JetStream publish, before marking as Published | Relay restarts, re-reads Pending events, re-publishes (stable MsgId dedup) |
| Worker crash | Worker crashes after Provider call, before ACK | Message redelivered; worker reads checkpoint, resumes from last known state |
| Broker outage | NATS JetStream cluster unavailable | Operation Store accepts writes; Outbox events stay Pending; relay retries |
| Database outage | PostgreSQL unavailable | Worker stops processing; no new facts committed; messages remain unacked |
| Leader election | JetStream cluster leader fails | New leader elected; consumers reconnect; no message loss (R3) |
| Network partition | Some workers cannot reach database | Workers fail to acquire lease; messages remain unacked; redelivered when partition heals |

## Conformance Test Scenarios

### 1. ACK Loss
1. Subscribe to `hnb.command.operation.step-requested.v1`
2. Receive message, process step, write result, ACK
3. Simulate ACK loss (drop network packet)
4. Verify: message redelivered within AckWait (60s)
5. Verify: worker checks idempotency, does not re-execute Provider call

### 2. Duplicate Delivery
1. Publish same message twice (same MsgId)
2. Verify: JetStream deduplicates within duplicateWindow (2m)
3. Publish same message with different MsgId but same idempotencyKey
4. Verify: worker checks idempotencyKey, second delivery is no-op

### 3. Out-of-Order Delivery
1. Publish message with expectedVersion=2, then message with expectedVersion=1
2. Verify: worker rejects message with expectedVersion=1 (stale)
3. Verify: stale message is ACKed but not processed

### 4. Relay Crash
1. Write OutboxEvent with status=pending
2. Intercept at publish step: crash relay after JetStreamPubAck but before DB update
3. Restart relay
4. Verify: OutboxEvent is re-read, re-published, JetStream deduplicates by MsgId
5. Verify: only one OutboxEvent is marked as published

### 5. Worker Crash
1. Worker receives Step command, calls Provider API
2. Crash worker before ACK
3. Verify: message redelivered to another worker
4. Verify: new worker reads checkpoint, observes Provider state, resumes

### 6. Broker Outage
1. Stop NATS JetStream service
2. Submit new Operation via API
3. Verify: Operation is persisted as Queued, OutboxEvent is pending
4. Restart NATS JetStream
5. Verify: Outbox Relay publishes pending events, Operation continues

### 7. Database Outage
1. Stop PostgreSQL database
2. Worker receives message from JetStream
3. Verify: worker cannot acquire lease, does not process
4. Verify: message remains unacked, no false positive
5. Restore database
6. Verify: worker acquires lease, processes normally

## Pass/Fail Criteria

| Criterion | Threshold |
|-----------|-----------|
| Message loss after fault | 0 |
| Duplicate business effect | 0 |
| Stale message executed | 0 |
| Recovery without manual intervention | All redelivery scenarios |
| Max RTO for message processing | 5 minutes after fault clears |