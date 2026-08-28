# Alert Actions and RBAC Design Evidence

## Available Actions

| Action | Permission | Effect |
|--------|-----------|--------|
| Acknowledge | `alert:acknowledge` | Sets state to Acknowledged, records actor + reason |
| Assign | `alert:assign` | Sets assigneeId, records actor |
| Silence | `alert:silence` | Creates Silence, sets alert state to Silenced |
| Unsilence | `alert:silence` | Removes active silence, returns to Firing |
| View | `alert:read` | View alert details and history |

## Permission Matrix

| Role | Read | Acknowledge | Assign | Silence | Unsilence |
|------|------|-------------|--------|---------|-----------|
| Viewer | ✓ | - | - | - | - |
| Operator | ✓ | ✓ | ✓ | ✓ | ✓ |
| Admin | ✓ | ✓ | ✓ | ✓ | ✓ |
| Audit | ✓ | - | - | - | - |

## Concurrency Control

All write operations use `expectedVersion`:
```
POST /alerts/{id}:acknowledge
{
    "expectedVersion": 5,
    "reason": "Investigating"
}
```

409 Conflict response:
```json
{
    "error": "version_conflict",
    "currentVersion": 6,
    "currentState": "silenced",
    "message": "Alert was modified by another user. Please refresh and retry."
}
```

## Audit Trail

Each action records:
- alert_id, action, actor_id, reason, timestamp, version
- Retrievable via `GET /alerts/{id}/audit`

## Test Plan
- RBAC: operator can acknowledge, viewer cannot
- Concurrency: two simultaneous acknowledges -> one succeeds, one gets 409
- Audit: each action creates correct audit entry
- UI: viewer sees no action buttons, operator sees enabled buttons
- Silenced state: unsilence returns to firing, acknowledged-silenced shows both states