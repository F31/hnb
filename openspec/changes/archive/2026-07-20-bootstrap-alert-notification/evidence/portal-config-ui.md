# Policy, Contact, Channel Configuration UI Design Evidence

## Configuration Pages

### Policy Configuration
- List of NotificationPolicies with tenant scope, matchers, channels
- Create/Edit form with matcher builder (name, value, isRegex)
- Channel selector (filtered by enabled channels for tenant)
- Escalation step editor (timing, contact group, channels, stop on ack)
- Schedule editor (timezone, day of week, start/end time)

### Contact Group Configuration
- List of ContactGroups with member count
- Create/Edit form with member editor (name, email, phone)
- Schedule assignment (optional, pick from existing schedules)

### User Preferences
- Language selector (en, zh-CN, etc.)
- Timezone selector
- Channel preferences (checkboxes, Critical always enforced)
- Severity filter (checkboxes, Critical always enabled)

### Channel Configuration
- List of NotificationChannels with type, enabled status
- Email: SMTP host, port, username, from address (password via SecretReference)
- Webhook: URL, allowed domains, timeout (secret via SecretReference)
- SMS: only shown if SMS Provider is installed and authenticated

### Test Notification
- Button on each channel detail page
- Sends test notification to the current user
- Shows success/failure result
- Fully audited

## Sensitive Field Masking

- Email addresses: `u***@example.com`
- Phone numbers: `+86****1234`
- Secret values: `********` (never displayed)
- API keys: `****...****` (last 4 chars shown for identification)

## Permissions

| Page | Read | Write |
|------|------|-------|
| Policy | `policy:read` | `policy:write` |
| Contact Group | `contact:read` | `contact:write` |
| User Preferences | `preference:read` (self) | `preference:write` (self) |
| Channel Config | `channel:read` | `channel:write` |
| Test Notification | `channel:test` | `channel:test` |

## Test Plan
- Form validation: all required fields, correct formats
- Permission: read-only user sees form fields disabled
- Masking: sensitive fields are masked in read mode
- Channel visibility: SMS hidden when no SMS Provider installed
- Test notification: test button sends correctly and is audited
- Escalation editor: add/remove/reorder escalation steps