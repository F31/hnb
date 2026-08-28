# Retry, Failed Subject, Manual Redrive, and Approval Check

## Finite Retry with Backoff

Each consumer has a configured `MaxDeliver` and `backoff` array:
- `operation-worker`: 10 execution attempts plus one delivery reserved for persisting terminal failure, backoff up to 2 hours
- `projector`: max 5 deliveries, backoff up to 5 minutes
- `audit`: max 3 deliveries, backoff up to 30 seconds
- `notification-dispatcher`: max 5 deliveries, backoff up to 2 minutes

When all deliveries are exhausted, the message moves to the failed subject.

## Failed Subject

Messages that exceed MaxDeliver are published to `hnb.failed.<original-subject>`:

```
Original: hnb.command.operation.step-requested.v1
Failed:    hnb.failed.command.operation.step-requested.v1
```

### Failed Message Envelope

```json
{
  "originalMessageId": "uuid",
  "originalSubject": "hnb.command.operation.step-requested.v1",
  "failedAt": "2026-07-20T12:00:00Z",
  "failureReason": "max_deliveries_exceeded",
  "deliveryAttempts": 10,
  "lastError": "Provider API timeout after 30s",
  "message": { ... original message body ... }
}
```

### Failed Subject Stream

| Parameter | Value |
|-----------|-------|
| Stream Name | `failed-messages` |
| Subjects | `hnb.failed.>` |
| Retention | `limits` |
| Max Age | 30 days |
| Max Bytes | 1 GB |
| Storage | `file` |

## Manual Redrive Entry

A REST API endpoint allows operators to manually redrive failed messages:

```
POST /api/v1/admin/messaging/redrive
{
  "messageId": "uuid",
  "targetSubject": "hnb.command.operation.step-requested.v1",
  "reason": "Provider issue resolved"
}
```

The redrive operation:
1. Reads the original message from the failed subject
2. Verifies the operator has admin permissions
3. Publishes the message to the original subject with a new message ID
4. Records the redrive action in the audit log
5. Returns success/failure

## Approval Check

Before redriving a message to certain subjects, an approval check is required:

| Subject Pattern | Approval Required | Approver |
|----------------|-------------------|----------|
| `hnb.command.>` | Yes | Platform Admin |
| `hnb.event.>` | No | Auto-approved |
| `hnb.notification.>` | No | Auto-approved |

The approval request:
```
POST /api/v1/admin/messaging/redrive/approve
{
  "messageId": "uuid",
  "reason": "Resource contention resolved",
  "approvedBy": "admin@example.com"
}
```

## Test Plan

### Unit Tests
- Message exceeds MaxDeliver: moves to failed subject
- Failed message contains original metadata and error details
- Redrive API: republishes to original subject

### Integration Tests
- Poison message: retry until exhausted, lands in failed subject
- Redrive: operator republishes failed message, worker processes it
- Security: unauthorized user cannot redrive messages
- Approval: command subject requires approval, event subject does not
