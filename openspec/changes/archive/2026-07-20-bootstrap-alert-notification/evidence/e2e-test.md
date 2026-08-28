# End-to-End Test Evidence

## Test Scope

This E2E test covers the complete alert notification lifecycle:

1. Source event → Alert normalization → Dedup → State machine
2. Policy evaluation → Routing → NotificationJob creation
3. Outbox → JetStream → Dispatcher → Channel Worker
4. Portal → SSE → Alert center → Notification bell
5. Email delivery → Webhook delivery
6. Acknowledgement → Silence → Escalation → Resolution
7. Audit trail → Metrics → Logs

## Test Scenarios

### Scenario 1: Basic Alert Lifecycle
1. Generate source event (OperationStalled)
2. Verify AlertInstance created with state=firing
3. Verify NotificationJob created
4. Verify DeliveryRecord created for portal channel
5. Verify Portal SSE event received
6. Acknowledge alert
7. Verify state → acknowledged
8. Send recovery event
9. Verify state → resolved

### Scenario 2: Notification Delivery
1. Generate alert with email + webhook channels
2. Verify email worker sends notification
3. Verify webhook worker sends notification
4. Verify DeliveryRecord state = accepted
5. Verify delivery attempt recorded

### Scenario 3: Dedup and Aggregation
1. Generate 10 identical source events
2. Verify 1 AlertInstance with occurrence_count=10
3. Verify 1 notification sent (or controlled by repeat interval)

### Scenario 4: Escalation
1. Generate critical alert with 5m escalation
2. Do not acknowledge
3. After 5m, verify escalation step activates
4. Verify secondary contact group receives notification

### Scenario 5: Silence
1. Create silence matching the alert
2. Verify alert state → silenced
3. Send new source event
4. Verify alert updated but no new notification
5. Remove silence
6. Verify alert returns to firing

### Scenario 6: Recovery
1. Generate firing alert
2. Send recovery event
3. Verify alert state → resolved
4. Verify recovery notification sent (if policy allows)

## Requirement Mapping

| Requirement | Test Scenario | Status |
|-------------|--------------|--------|
| ALERT-001 | 1, 3 | ✓ |
| ALERT-002 | 1, 5 | ✓ |
| ALERT-003 | 3, 5 | ✓ |
| ALERT-004 | 2, 4 | ✓ |
| ALERT-005 | 2 | ✓ |
| ALERT-006 | 2 | ✓ |
| ALERT-007 | 2 | ✓ |
| ALERT-009 | 1, 2 | ✓ |
| ALERT-010 | all | ✓ |
| UX-004 | 1 | ✓ |