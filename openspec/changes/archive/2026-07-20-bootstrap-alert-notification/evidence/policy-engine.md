# Policy Engine Design Evidence

## Policy Matching Algorithm

1. Collect all NotificationPolicies applicable to the alert's tenant/project/environment
2. Filter by tenant_scope: global → tenant → project (more specific takes priority)
3. For each policy, evaluate matchers against alert labels:
   - `name=value` exact match
   - `name=~regex` regex match
   - All matchers must match for policy to apply
4. If multiple policies match, apply priority order:
   - Most specific scope wins (project > tenant > global)
   - Within same scope, first match wins
   - Critical severity always has a default safety route fallback

## Policy Snapshot

When a notification is created, a snapshot of the matched policy is stored in `notification_jobs.policy_snapshot`:

```json
{
  "policyId": "uuid",
  "contactGroupId": "uuid",
  "channels": ["portal", "email"],
  "repeatInterval": "5m",
  "escalationLevel": 0
}
```

This ensures subsequent policy changes don't alter historical delivery behavior.

## Default Safety Route

If no custom policy matches a Critical alert:
1. Use the platform's `defaultSafetyRoute` from severity-levels configuration
2. This always includes at least Portal + Email channels
3. The default contact group is the platform admin group
4. This route cannot be disabled by tenant configuration

## Priority and Conflict Resolution

| Priority | Rule | Example |
|----------|------|---------|
| 1 (highest) | Project-level policy | `project=production, severity=critical` |
| 2 | Tenant-level policy | `severity=critical` |
| 3 | Global policy | `source=operation` |
| 4 | Default safety route | Always applies to unmatched Critical |

## Test Plan
- Tenant isolation: Tenant A's policies don't apply to Tenant B's alerts
- Scope priority: Project policy overrides tenant policy for project-scoped alerts
- Default safety route: Critical alert with no matching policy gets platform route
- Multiple matchers: All matchers must match for policy to apply
- Regex matcher: `resource=~^prod-.*` matches `prod-db-01`
- Snapshot immutability: Policy change after delivery doesn't affect existing jobs