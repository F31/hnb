# User Preferences and Critical Route Intersection Design Evidence

## User Preference Model

```json
{
  "userId": "user-123",
  "language": "zh-CN",
  "timezone": "Asia/Shanghai",
  "channels": ["portal", "email"],
  "severityFilters": ["critical", "warning"]
}
```

## Intersection Algorithm

When evaluating which channels and severity levels to send:

```
effective_channels = user_preferences.channels ∩ policy.channels
effective_severity = user_preferences.severity_filters ∩ policy.severity
```

### Critical Route Enforcement

Critical severity alerts ALWAYS use the platform's mandatory route:
- `effective_channels` for critical always includes at least `["portal", "email"]`
- User preference `severity_filters` cannot exclude `"critical"`
- Even if user only wants `["warning", "info"]`, critical notifications are sent

### Implementation

```go
func computeEffectiveChannels(prefs UserPreferences, policy NotificationPolicy, severity string) []string {
    if severity == "critical" {
        // Critical always uses mandatory route
        mandatory := []string{"portal", "email"}
        if policy.tenantScope != "global" && len(policy.channels) > 0 {
            return intersect(append(mandatory, prefs.Channels...), policy.channels)
        }
        return mandatory
    }
    return intersect(prefs.Channels, policy.channels)
}
```

## Test Plan
- Allow preference: user selects email-only → receives email for warning alerts
- Critical override: user has empty channels → still receives portal + email for critical
- Severity filter: user filters info → no info notifications, critical still goes through
- Partial intersection: user prefers email, policy includes webhook → only email delivered
- Admin override: tenant admin cannot disable mandatory critical route