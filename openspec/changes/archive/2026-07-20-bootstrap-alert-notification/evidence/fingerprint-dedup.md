# Fingerprint, Dedup, and Firing/Resolved Pairing Design Evidence

## Fingerprint Algorithm

```
fingerprint = SHA256(tenant_id + "|" + source + "|" + resource_ref + "|" + rule_id + "|" + sorted_labels)
```

Where `sorted_labels` is a canonical JSON string of labels sorted by key.

### Stability Guarantees
- Same source + resource + rule + labels always produces the same fingerprint.
- Labels are case-sensitive; sorting is by Unicode code point.
- Changing the rule definition changes the fingerprint (new alert, not dedup).

## Dedup Flow

1. Normalizer computes fingerprint for incoming event
2. Queries `alert_instances` WHERE fingerprint = ? AND state != 'resolved'
3. If active instance exists:
   - Update `last_seen_at`, `occurrence_count += 1`
   - Check if severity should be escalated (max severity wins)
   - Apply expected_version concurrency check
4. If no active instance:
   - INSERT new AlertInstance with state = 'firing'
   - Create NotificationJob + DeliveryRecord

## Firing/Resolved Pairing

### Firing Event
- Sets state to `firing` (or creates if new)
- Records `first_seen_at` and `last_seen_at`
- Triggers notification evaluation

### Resolved Event
- Must match the same fingerprint
- Only resolves if an active (firing/acknowledged/silenced) instance exists
- Sets state to `resolved`, records `resolved_at`
- Triggers recovery notification if policy allows

### Edge Cases
- **Resolved before firing**: Log as anomalous, create alert in resolved state with warning
- **Duplicate resolved**: No-op, return existing state
- **Firing after resolved**: Creates new firing alert (same fingerprint, new lifecycle)
- **Out-of-order events**: Use source timestamp (`first_seen_at`) to order; reject events with timestamps older than the resolved time

## Test Plan
- Fingerprint stability test: same input produces same output
- Dedup test: 10 identical events produce 1 alert instance with occurrence_count=10
- Firing/Resolved pair test: firing then resolved creates lifecycle
- Out-of-order test: resolved arrives before firing
- Duplicate test: repeat events are idempotent
- Jitter test: same resource oscillating within debounce window