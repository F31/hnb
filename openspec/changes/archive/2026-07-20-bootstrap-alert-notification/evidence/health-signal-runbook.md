# Independent Health Signal and Runbook Evidence

## Design Principle

The Notification Dispatcher must NOT be the sole mechanism for reporting its own health. A failure that prevents the Dispatcher from sending notifications must be visible through an independent path.

## Health Signals

### Layer 1: Kubernetes Health Checks
- Liveness probe: `/healthz` — basic process health
- Readiness probe: `/readyz` — can accept work
- Both endpoints are served on a separate management port (not the public API port)

### Layer 2: Platform Component Status
- Alert/Notification service registers as a platform component
- Component status is reported via platform's internal health API
- If Dispatcher is unhealthy, platform status shows "degraded"

### Layer 3: External Monitoring
- Prometheus alerting rule: `notification_dispatcher_up == 0` → critical alert
- This alert is sent via a fallback channel (e.g., separate webhook or email server)
- The fallback must not depend on the same Dispatcher being monitored

## Runbook: Notification Dispatcher Full Outage

### Symptoms
- Dashboard shows `notification_jobs_pending` increasing
- No delivery attempts recorded for > 5 minutes
- Platform status shows "alert-notification: unavailable"

### Immediate Actions
1. Check Dispatcher pod status: `kubectl get pods -n hnb-system | grep notification-dispatcher`
2. Check logs: `kubectl logs -n hnb-system deployment/notification-dispatcher --tail=100`
3. Check JetStream consumer lag: `nats consumer ls --stream hnb.command.notification.dispatch.v1`
4. If pod is stuck: `kubectl delete pod -n hnb-system <notification-dispatcher-pod>`

### Recovery
1. Verify Dispatcher restarts and reconnects to JetStream
2. Monitor consumer lag decreasing
3. Check delivery records being processed
4. Verify Portal notifications are working
5. Verify external channels (email, webhook) resume delivery

### Escalation
- If Dispatcher cannot recover within 5 minutes: escalate to platform team
- If data loss is suspected: restore from backup
- If the issue is caused by a bug: rollback to previous version

## Test Plan
1. Stop all notification-dispatcher pods
2. **Expected**: Platform status shows "alert-notification: unavailable"
3. **Expected**: Prometheus alert fires via fallback channel
4. **Expected**: No new external notifications are sent
5. **Expected**: Alert data in PostgreSQL is not affected
6. Restore pods
7. **Expected**: Dispatcher resumes, backlog clears