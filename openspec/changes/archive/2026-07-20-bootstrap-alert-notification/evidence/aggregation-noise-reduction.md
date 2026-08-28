# Alert Aggregation and Noise Reduction Design Evidence

## Mechanisms

### 1. Fingerprint-based Dedup
- Same fingerprint → same alert instance
- Occurrence count increments with each event
- Prevents duplicate alerts for the same condition

### 2. Time Window Aggregation
- Configurable aggregation window (default: 5 minutes)
- Events within the same window for the same alert are merged
- `last_seen_at` updated, `occurrence_count` incremented
- Notification timer resets on each new event within window

### 3. Parent-Child Inhibition
```go
type InhibitionRule struct {
    SourceMatchers   map[string]string  // parent alert matchers
    TargetMatchers   map[string]string  // child alert matchers
    EqualLabels      []string           // labels that must match for inhibition
}
```
- If a parent alert is firing, child alerts matching the inhibition rule are suppressed
- Example: NodeDown inhibits all Pod alerts on that node
- Suppressed alerts are stored with state=firing but notification suppressed

### 4. Debounce (Jitter Protection)
- Configurable debounce window (default: 60 seconds)
- Rapid state oscillations within the window are collapsed
- Only the first event triggers notification evaluation
- `last_seen_at` is updated but notification timer is not reset

### 5. Repeat Notification Interval
- Configurable per severity (e.g., critical: 5m, warning: 15m, info: 1h)
- Controls how often a notification is re-sent for a still-firing alert
- Resets when the alert state changes (e.g., acknowledged)
- Does NOT reset on `last_seen_at` update alone

### 6. Maintenance Window
- Time-bound suppression of all alerts matching matchers
- During maintenance: alerts are still created and tracked, but notifications suppressed
- After window ends: alerts are re-evaluated for notification
- Maintenance windows are logged and auditable

### 7. Time-bound Silence
- Similar to maintenance window but created by users
- Supports matchers for flexible suppression
- Can be created, extended, and removed
- Expired silences are automatically detected and deactivated

## Notification Suppression Evaluation Order

1. Is alert silenced by active silence? → Suppress
2. Is alert in active maintenance window? → Suppress  
3. Is alert inhibited by parent alert? → Suppress
4. Is alert within debounce window? → Skip notification
5. Is alert within repeat interval? → Skip notification
6. → Allow notification

## Test Plan
- Aggregation: 10 events in 5 min window → 1 alert, 1 notification
- Inhibition: NodeDown suppresses PodCrashLooping on same node
- Debounce: 5 oscillations in 60s → 1 state change
- Repeat interval: 3 notifications in 15 min for critical (5m interval)
- Maintenance window: alert during window is suppressed, post-window re-evaluated
- Silence: manual silence suppresses notifications, auto-expire resumes
- Explain output: each suppression reason is recorded and visible