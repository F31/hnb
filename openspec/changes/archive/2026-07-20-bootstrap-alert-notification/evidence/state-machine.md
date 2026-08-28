# Alert Instance State Machine Design Evidence

## State Machine

```
                    +---> Silenced <---+
                    |                  |
Pending ---> Firing ---> Acknowledged -+
                    |                  |
                    +---> Resolved <---+
                         (from any non-resolved state)
```

### State Transitions

| From | To | Trigger | Notes |
|------|----|---------|-------|
| Pending | Firing | Source firing event | Automatic on first valid event |
| Firing | Acknowledged | User acknowledge action | Requires permissions, reason, expected_version |
| Firing | Silenced | Silence match active | Automatic, reversible |
| Firing | Resolved | Source recovery event | Only from authoritative source |
| Acknowledged | Silenced | Silence activated | Does not un-acknowledge |
| Acknowledged | Resolved | Source recovery event | Recovery always wins |
| Silenced | Firing | Silence expired/removed | Alert can still be firing |
| Silenced | Resolved | Source recovery event | Recovery during silence |
| Any | Resolved | Manual resolution | Requires admin permissions, audit trail |

### Illegal Transitions
- Resolved → any other state (terminal)
- Silenced → Acknowledged (must un-silence first, which returns to Firing)
- Pending → Resolved (only if recovery event received before firing; logged as anomalous)

## Concurrency Control

Every state transition uses `expected_version`:
```sql
UPDATE alert_instances
SET state = ?, version = version + 1, updated_at = now()
WHERE id = ? AND version = ?
```

If version mismatches, the transition is rejected with a 409 Conflict error. The caller must re-read the current state and retry.

## Audit Trail

Every state transition records an `alert_state_audits` row:
- alert_id, previous_state, new_state, actor_id, reason, version

## Test Plan
- Legal transition matrix: all 12 legal transitions produce correct state
- Illegal transition matrix: all illegal transitions rejected with error
- Concurrency test: two simultaneous transitions; one succeeds, one gets 409
- Actor audit: each transition creates correct audit row
- Recovery from acknowledged: acknowledged alert is resolved by recovery event
- Silence lifecycle: auto-expire, manual create, manual remove