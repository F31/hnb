# Disaster Recovery and Backup Evidence

## Backup Scope

The Alert Store backup includes:
- All alert_instances (active and resolved)
- All notification_policies
- All contact_groups and schedules
- All silences and maintenance_windows
- All notification_channels
- All delivery_records (including pending)
- All outbox_events (pending deliveries)
- All user_notification_preferences

## RPO and RTO

| Tier | RPO | RTO |
|------|-----|-----|
| Minimal | 24 hours | 4 hours |
| Lite HA | 1 hour | 30 minutes |
| Standard HA | 5 minutes | 5 minutes |
| Enterprise | 1 minute | 1 minute |

## Recovery Procedure

1. Restore Alert Store database from backup
2. Verify data integrity (alert count, policy count, delivery count)
3. Reconcile active alerts with Source Adapters:
   - Re-fetch active alerts from all source systems
   - Compare with restored alert_instances
   - Recover any alerts created between backup and restore
4. Verify Notification Dispatcher can access restored data
5. Verify Portal unread count is rebuilt correctly
6. Verify external channels can resume delivery

## Recovery Verification

| Check | Expected |
|-------|----------|
| Active alert count | Matches source systems |
| Policy count | Matches backup manifest |
| Delivery records | Pending deliveries are re-queued |
| Outbox events | Pending outbox events are re-dispatched |
| Unread count | Recalculated from server-side state |
| Channel config | All channels functional |
| SSE connections | Clients reconnect and re-sync |