# Delivery: Task 27 - Deployment Operator Runbook

## Scope
P1-ING-001 through P1-CONSOLE-003 — Deployment ordering, compatibility window, enforcement cutover.

## Component Startup Ordering

### Phase 1: Infrastructure Layer
1. **NATS JetStream** — Message backbone for outbox relay and command distribution
2. **PostgreSQL** — Primary data store for operations, intents, execution plans, identity/authorization state

Both must be healthy before application components start. Health check endpoints:
- NATS: `/streamz` or NATS client ping
- PostgreSQL: `SELECT 1` via connection pool test

### Phase 2: Core Services
3. **apiserver** — Edge proxy, auth middleware, Console bootstrap, route authorization
   - Requires: NATS upstream, PostgreSQL (for audit/metadata), IAM key manifest
   - Critical path: must start before any external traffic routing

4. **platform-api** — Intent submission, planning, operation lifecycle, outbox event emission
   - Requires: PostgreSQL (operations, intents, execution_plans, outbox_events)
   - Port: internal API (9090 by convention)

5. **app-market** — Application catalog, release tracking
   - Requires: PostgreSQL, OCI artifact registry
   - Optional: NATS for market event processing

### Phase 3: Provider & Worker Components
6. **Providers** — Kubernetes, Container, Edge runtime drivers
   - Started per-provider configuration
   - Require: operational cluster connectivity

7. **Operation Worker** (`cmd/operation-worker`) — Consumes step-requested outbox events
   - Requires: NATS subscription to `hnb.command.operation.step-requested.v1`
   - Start after platform-api is producing outbox events

## Compatibility Window

### Token Format Transition
- **Before cut-over**: apiserver accepts both legacy header-trusted context AND new JWT-based trusted context
- **After cut-over**: Only JWT-based trusted context accepted (legacy headers stripped)

### Compatibility Period
During the transition window (recommended: 48 hours minimum):
- Old tokens remain valid until expiry (max 60 seconds per token TTL)
- New services verify against existing signing keys
- Key manifest generation increases with each rotation

### Enforcement Cutover Plan
| Day | Action |
|-----|--------|
| D-2 | Deploy new binaries with dual-mode acceptance (header + JWT) |
| D-0 | Enable JWT enforcement on protected routes |
| D+1 | Monitor auth failure rate; if spike > 5%, rollback |
| D+2 | Disable legacy header mode entirely |
| D+3 | Verify no regression in client token flows |

## Failure Modes & Recovery
1. **NATS unreachable**: APIs return 503; operations cannot be submitted
2. **PostgreSQL partition**: All writes fail; reads may serve stale cached data
3. **Key rotation failure**: Stale keys continue serving until manifest reload succeeds
4. **apiserver crash**: platform-api remains functional but no new traffic routed
