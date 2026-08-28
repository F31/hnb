# Channel Rate Limiting, Circuit Breaker, and Failure Isolation Design Evidence

## Per-Channel Rate Limiting

Each channel type has independent rate limits:

| Channel | Rate Limit | Scope |
|---------|-----------|-------|
| Email | 60/min, 1000/hour | Per tenant |
| Webhook | 120/min, 3000/hour | Per tenant, per endpoint |
| SMS | 10/min, 100/hour | Per tenant, per region |
| Portal | Unlimited | — |

### Token Bucket Algorithm

```go
type TokenBucket struct {
    Capacity    int
    RefillRate  int      // tokens per second
    Tokens      float64
    LastRefill  time.Time
    Mu          sync.Mutex
}

func (tb *TokenBucket) Allow() bool {
    tb.Mu.Lock()
    defer tb.Mu.Unlock()
    now := time.Now()
    elapsed := now.Sub(tb.LastRefill).Seconds()
    tb.Tokens = math.Min(float64(tb.Capacity), tb.Tokens + elapsed * float64(tb.RefillRate))
    tb.LastRefill = now
    if tb.Tokens >= 1 {
        tb.Tokens--
        return true
    }
    return false
}
```

## Circuit Breaker

Per-channel circuit breaker with three states: CLOSED → OPEN → HALF_OPEN

| State | Behavior | Transition |
|-------|----------|------------|
| CLOSED | Normal operation | → OPEN after N consecutive failures |
| OPEN | Requests fail fast | → HALF_OPEN after cooldown period |
| HALF_OPEN | Single probe request | → CLOSED on success, → OPEN on failure |

### Configuration
- Failure threshold: 5 consecutive failures
- Cooldown: 30 seconds (email), 10 seconds (webhook), 60 seconds (SMS)
- Half-open max probes: 1

## Failure Isolation

- Each channel type has its own worker pool
- One channel's rate limit or circuit breaker does NOT affect other channels
- Portal notifications are always delivered regardless of external channel status
- Channel failures are tracked in DeliveryAttempt records

## Backoff Strategy

Exponential backoff per delivery attempt:
```
Attempt 1: immediate
Attempt 2: 5s
Attempt 3: 30s
Attempt 4: 2m
Attempt 5: 5m
Attempt 6+: 5m (cap)
```

Maximum retries: 5 (configurable per channel type)

## Manual Redrive

API endpoint `POST /admin/notification/redrive`:
- Accepts delivery_id
- Retries a failed delivery regardless of backoff state
- Resets circuit breaker for the channel
- Requires admin permissions
- Audited

## Test Plan
- Rate limit: 61st email in a minute is rate-limited
- Circuit breaker: 5 consecutive failures → OPEN, subsequent requests fail fast
- Half-open: after cooldown, probe succeeds → CLOSED
- Channel isolation: email failure doesn't block webhook
- Backoff: attempts follow exponential schedule
- Manual redrive: failed delivery can be manually retried
- Portal independence: external channel failures don't affect Portal notifications