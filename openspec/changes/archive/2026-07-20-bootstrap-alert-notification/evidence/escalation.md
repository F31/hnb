# Contact Group, Schedule, and Escalation Design Evidence

## Contact Group Resolution

1. Policy evaluation determines the target ContactGroup
2. If ContactGroup has a `schedule_ref`, resolve the on-call member:
   - Match current time against schedule shifts
   - If shift active, select on-call member
   - If no shift matches, use group's primary contact
3. ContactGroup members are resolved to delivery destinations:
   - Email → member.email (masked in logs)
   - Phone → member.phone (masked, encrypted)
   - Portal → member.userId

## Escalation Timer

1. Initial notification is sent to the primary ContactGroup
2. Escalation timer starts when the first notification is sent
3. If no acknowledgement within `escalation_steps[0].after` duration:
   - Route to `escalation_steps[0].contactGroupId`
   - Use `escalation_steps[0].channels`
4. Continue through escalation steps until:
   - Alert is acknowledged (`stopOnAck: true`)
   - Alert is resolved
   - All escalation steps are exhausted

## Virtual Clock Algorithm

```go
type EscalationState struct {
    CurrentLevel     int
    LevelStartedAt   time.Time
    StopOnAck        bool
    AlternateIndex   int  // tracks which alternate channel is active
}

func (e *EscalationState) Advance(now time.Time, steps []EscalationStep) {
    if e.CurrentLevel >= len(steps) {
        return // all levels exhausted
    }
    elapsed := now.Sub(e.LevelStartedAt)
    if elapsed >= steps[e.CurrentLevel].After {
        e.CurrentLevel++
        e.LevelStartedAt = now
    }
}
```

## Alternate Channel Fallback

1. Primary channel attempt fails
2. After `alternateChannels[0].afterFailures` failures, switch to alternate channel
3. Alternate channel is a different channel type (e.g., email → webhook)
4. Original channel is retried periodically; if it recovers, switch back

## Test Plan
- Virtual clock: 5m escalation fires at correct time
- Multiple levels: 3 escalation levels advance correctly
- Stop on ack: acknowledged alert stops escalation
- Alternate channel: after N failures, switch to alternate
- Schedule matching: shift matches correct timezone and day
- No schedule: contact group without schedule uses primary member