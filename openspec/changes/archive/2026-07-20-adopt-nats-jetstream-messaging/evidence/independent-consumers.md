# Independent Durable Consumers: Projector, Audit, Notification

## Architecture

Each downstream consumer (Projector, Audit, Notification) has its own JetStream Durable Consumer on the same stream. This allows independent progress tracking, separate lifecycle management, and isolated fault domains.

## Consumer Definitions

### Projector Consumer

| Parameter | Value |
|-----------|-------|
| Consumer Name | `projector` |
| Stream | `domain-events` |
| Filter Subject | `hnb.event.>` |
| Deliver Policy | `new` |
| Ack Policy | `explicit` |
| Ack Wait | 60s |
| Max Deliver | 5 |
| Max Ack Pending | 32 |
| Replay Policy | `instant` |
| Backoff | [10s, 30s, 60s, 120s, 300s] |

**Purpose:** Update read models (projections) when domain events occur.

### Audit Consumer

| Parameter | Value |
|-----------|-------|
| Consumer Name | `audit` |
| Stream | `domain-events` |
| Filter Subject | `hnb.event.>` |
| Deliver Policy | `new` |
| Ack Policy | `explicit` |
| Ack Wait | 30s |
| Max Deliver | 3 |
| Max Ack Pending | 64 |
| Replay Policy | `instant` |
| Backoff | [5s, 15s, 30s] |

**Purpose:** Record immutable audit trail of all domain events.

### Notification Dispatcher Consumer

| Parameter | Value |
|-----------|-------|
| Consumer Name | `notification-dispatcher` |
| Stream | `notifications` |
| Filter Subject | `hnb.notification.>` |
| Deliver Policy | `new` |
| Ack Policy | `explicit` |
| Ack Wait | 30s |
| Max Deliver | 5 |
| Max Ack Pending | 32 |
| Replay Policy | `instant` |
| Backoff | [5s, 15s, 30s, 60s, 120s] |

**Purpose:** Dispatch notifications to Portal/Email/Webhook channels.

## Independent Progress

Each consumer maintains its own delivery cursor in JetStream:
- Consumer A (Projector) may be at sequence 100
- Consumer B (Audit) may be at sequence 50 (slower)
- Consumer C (Notification) may be at sequence 80

This isolation means:
- A slow Projector doesn't block Audit
- A failed Notification dispatcher doesn't affect Projector
- Each consumer can be reset/replayed independently

## Consumer Restart Recovery

When a consumer restarts:
1. JetStream resumes delivery from the last ACKed sequence
2. ConsumerCheckpoint database table logs the last processed sequence
3. On restart, consumer compares JetStream cursor with database checkpoint
4. If database checkpoint is ahead (possible race), skip already-processed events
5. If database checkpoint is behind (possible crash before checkpoint), reprocess (idempotent)

## Event Fan-Out

Domain events are published once to the `domain-events` stream. JetStream's Durable Consumer model handles fan-out automatically:
- Multiple consumers on the same stream each receive their own copy of matching messages
- Filter subjects allow consumers to receive only relevant event types
- No need for explicit routing or topic exchange logic

## Test Plan

### Integration Tests
- All three consumers process events from the same stream independently
- Pause one consumer: other two continue unaffected
- Restart consumer: resumes from last ACKed position
- Slow consumer: backpressure applied, no impact on other consumers
- Consumer crash: redelivery with backoff, no data loss