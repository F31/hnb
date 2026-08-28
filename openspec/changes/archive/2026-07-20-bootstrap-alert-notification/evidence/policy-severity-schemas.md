# Alert Policy and Severity Schema Evidence

## Created Schema Files

### Severity and Policy Configuration (5 additional JSON Schema files)
- `contracts/schema/alert/v1/severity-levels.schema.json` — Defines 3-tier severity (critical/warning/info) with order, color, default repeat intervals, escalation timing, and mandatory route flag. Includes the `defaultSafetyRoute` object with contact group and channels.
- `contracts/schema/alert/v1/contact-group.schema.json` — Contact group with tenant ID, member list (userId, name, email, phone), and optional schedule reference.
- `contracts/schema/alert/v1/schedule.schema.json` — On-call schedule with timezone, shift definitions (days of week, start/end time), and exception overrides.
- `contracts/schema/alert/v1/escalation-step.schema.json` — Escalation step with `after` duration, target contact group, channels, stop-on-ack flag, and alternate channel fallback definitions.
- `contracts/schema/alert/v1/channel-capability.schema.json` — Channel capability with tier (T1/T2), supported delivery states, max achievable state, receipt requirements, and rate limits.

### Registry
- `contracts/schema/alert/v1/policy-config-registry.json` — Index of all 5 policy configuration schemas.

## Key Design Decisions
- Critical severity has `mandatoryRoute: true` — cannot be disabled by user preferences.
- Default safety route is embedded in the severity-levels schema, ensuring platform-approved emergency routing always exists.
- Escalation steps use ISO 8601 duration strings (e.g., `5m`, `1h`, `1d`) for timing.
- Channel capability explicitly defines max state (Accepted/Delivered/Read) to prevent false delivery semantics.
- Rate limits are per-channel-type configurable with per-minute, per-hour, and per-day buckets.

## Verification
- 5 new JSON Schema files, all Draft 2020-12, `additionalProperties: false`.
- `severity-levels.schema.json` enforces exactly 3 levels with enum validation.
- `escalation-step.schema.json` uses `pattern` for duration validation.
- `contact-group.schema.json` uses `format: email` and `pattern` for phone number validation.
- Registry index file references all schemas.