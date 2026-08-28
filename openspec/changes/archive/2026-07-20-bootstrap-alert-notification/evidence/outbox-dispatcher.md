# Notification Job, Delivery, and Outbox Design Evidence

## Transactional Flow

```
1. Alert Normalizer creates/updates AlertInstance
2. Policy Engine evaluates and creates NotificationJob
3. DeliveryRecord created for each channel in the policy
4. Outbox event created with stable idempotency key
   -- All in ONE database transaction --
5. JetStream publishes `hnb.command.notification.dispatch.v1`
6. Notification Dispatcher receives and routes to channel workers
```

## Idempotency Key

```
idempotency_key = SHA256(delivery_id + "|" + attempt_count + "|" + channel_id)
```

This ensures:
- First attempt: creates delivery record, sends to channel
- Retry: same key → same delivery record, channel can use provider message ID for dedup
- Duplicate: blocked by unique index on `notification_jobs.idempotency_key`

## Notification Dispatcher Flow

1. Pull from `hnb.command.notification.dispatch.v1` JetStream consumer
2. Load DeliveryRecord, verify tenant context
3. Check if delivery is already in terminal state (idempotency)
4. Select appropriate Channel Worker based on channel type
5. Call Channel Worker
6. On success: update DeliveryRecord state → ACK JetStream message
7. On failure: update DeliveryRecord, schedule retry, NO ACK (or ACK with DLQ)

## JetStream Dispatcher

```yaml
Stream: hnb.command.notification.dispatch.v1
Consumer: notification-dispatcher
AckPolicy: explicit
MaxDeliver: 3
Backoff: [5s, 30s, 2m]
```

## Test Plan
- Transactional rollback: AlertInstance created but Outbox fails → all rolled back
- Idempotency: duplicate dispatch creates only one delivery record
- Dispatcher restart: in-flight deliveries resume after restart
- Outbox retry: Outbox failure -> retry with backoff -> eventual delivery
- Power loss: after commit, outbox event survives; after rollback, nothing persists