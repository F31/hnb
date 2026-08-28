# Grayscale Migration Plan Evidence

## Migration Phases

### Phase 1: Portal-only (1 week)
- Enable Portal alert center with read-only access
- Default policy: Portal-only for all alerts
- No external notifications (email, webhook)
- Verify tenant isolation, filtering, and real-time updates
- **Rollback**: Disable Alert Center feature flag, revert to old monitoring

### Phase 2: Alert Actions (1 week)
- Enable acknowledge, assign, silence actions
- Verify RBAC and concurrency control
- **Rollback**: Disable write actions, Portal returns to read-only

### Phase 3: Email (2 weeks)
- Enable Email channel for a subset of tenants
- Start with test notifications, then production traffic
- Monitor delivery success rate and latency
- **Rollback**: Disable Email channel, queue pending emails for manual review

### Phase 4: Webhook (2 weeks)
- Enable Webhook channel for a subset of tenants
- Verify SSRF protection, HMAC signing, and retry behavior
- **Rollback**: Disable Webhook channel

### Phase 5: Full Rollout (1 week)
- Enable all channels for all tenants
- Monitor end-to-end metrics
- Disable old monitoring alerting pipeline

## Rollback Procedure

### Immediate Rollback (< 1 hour)
1. Disable all external channels (email, webhook)
2. Portal returns to read-only mode
3. Old monitoring alerting pipeline is reactivated
4. No data loss — Alert Store data is preserved

### Extended Rollback (> 1 hour)
1. Perform Phase 1-4 rollback steps
2. Verify old monitoring pipeline is working
3. Schedule data migration if needed
4. Communicate to affected tenants

## Success Criteria
- Each phase passes with < 0.1% error rate
- No tenant reports missed notifications
- All rollback procedures tested and documented