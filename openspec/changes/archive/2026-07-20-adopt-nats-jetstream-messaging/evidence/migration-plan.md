# Migration Plan: Gray Release and Rollback

## Phase 1: Shadow Mode (no real side effects)

1. Deploy NATS JetStream cluster (Minimal or Lite HA per tier)
2. Create Streams, Consumers, and ACLs per JetStream config
3. Deploy Outbox Relay in shadow mode (publish events but don't switch consumers)
4. Deploy shadow consumers for Projector, Audit, Notification
5. Compare shadow consumer output with existing PostgreSQL polling output
6. Fix discrepancies until output matches

**Duration:** 1-2 weeks
**Rollback:** Stop shadow consumers, no user impact

## Phase 2: Read-Only Consumer Migration

1. Migrate Audit consumer to JetStream (read-only, no write side effects)
2. Verify audit trail completeness
3. Migrate Projector consumer (read model updates only)
4. Verify read model consistency

**Duration:** 1 week
**Rollback:** Switch Projector/Audit back to PostgreSQL polling

## Phase 3: Low-Risk Command Migration

1. Migrate notification dispatcher to JetStream
2. Verify notifications are delivered correctly
3. Migrate low-risk commands (e.g., status checks, read-only operations)
4. Verify command execution and idempotency

**Duration:** 1 week
**Rollback:** Switch low-risk commands back to PostgreSQL polling

## Phase 4: Operation Step Migration

1. Migrate Operation Step commands to JetStream
2. Run both PostgreSQL polling and JetStream consumer in parallel
3. Verify Operation execution is identical
4. After verification, disable PostgreSQL polling

**Duration:** 2 weeks
**Rollback:** Enable PostgreSQL polling, disable JetStream consumer

## Phase 5: Old Scheduler Removal

1. Remove PostgreSQL polling code
2. Remove old scheduler database tables
3. Update documentation
4. Run full E2E test suite

**Duration:** 1 week
**Rollback:** Revert code changes, restore database tables

## Rollback Procedure

### At any phase:

1. Stop new JetStream consumer processing
2. Wait for in-flight operations to complete (or timeout after 5 min)
3. Enable PostgreSQL polling scheduler
4. Verify Queued operations are picked up by old scheduler
5. Monitor for any duplicate operations

### Rollback Verification:

1. No new operations are stuck in Queued state
2. All existing operations complete normally
3. No duplicate business effects
4. Audit trail is continuous
5. Portal shows correct progress

## Safety Points

| Phase | Safety Point | Rollback Window |
|-------|-------------|-----------------|
| Shadow | No real side effects | Any time |
| Read-only consumers | No writes | Any time |
| Low-risk commands | Idempotent, observable | 1 hour |
| Operation steps | Dual path, verified | 1 day |
| Old scheduler removal | Code revert | 1 week |