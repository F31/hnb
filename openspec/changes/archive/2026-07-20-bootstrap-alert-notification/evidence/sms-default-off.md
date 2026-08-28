# SMS Default Off Evidence

## Design Decision
SMS is T2 optional. It is DISABLED by default in all production deployments.

## Enforcement

### Installation
- SMS Provider is NOT included in Minimal or Lite HA BOMs
- SMS Provider is listed as optional in Standard HA and Enterprise BOMs
- Installing SMS Provider requires explicit opt-in and authentication

### Portal
- SMS channel type is NOT shown in channel configuration UI
- SMS-related policy options are hidden
- Portal shows "SMS not available" message if SMS is referenced

### Policy Engine
- NotificationPolicies created without SMS channels are valid
- Policies referencing SMS channels are rejected unless SMS Provider is installed
- Default safety route never includes SMS

### Runtime
- SMS Worker is NOT deployed unless SMS Provider is installed
- No SMS-related code paths are exercised in T1 mode

## Enabling SMS

1. Install SMS Provider through Provider Conformance process
2. Authenticate and configure SMS Provider
3. SMS channel type becomes available in Portal
4. Per-tenant opt-in required before SMS notifications are sent

## Verification
- Clean install: no SMS components deployed
- Portal: SMS option hidden
- Policy: SMS references rejected
- After provider install: SMS becomes available
- Per-tenant enable: disabled by default, enabled on request