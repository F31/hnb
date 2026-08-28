# Alert/Notification API and Event Schema Evidence

## Created Schema Files

### JSON Schema (8 files)
- `contracts/schema/alert/v1/alert-instance.schema.json` — AlertInstance model with state, fingerprint, severity, labels
- `contracts/schema/alert/v1/alert-rule.schema.json` — AlertRule model with source type, severity, expression
- `contracts/schema/alert/v1/silence.schema.json` — Silence with matchers, time window, status
- `contracts/schema/alert/v1/notification-policy.schema.json` — Policy with matchers, channels, escalation, schedule
- `contracts/schema/alert/v1/notification-channel.schema.json` — Channel with type, capability, config/secret ref
- `contracts/schema/alert/v1/notification-job.schema.json` — Job with policy snapshot, priority, state
- `contracts/schema/alert/v1/delivery-record.schema.json` — Delivery record with state machine, attempt tracking
- `contracts/schema/alert/v1/delivery-attempt.schema.json` — Per-attempt record with result class, duration, trace

### Protobuf Messages (4 new messages in `contracts/proto/hnb/contracts/v1/contracts.proto`)
- `AlertFiring` — alert firing event with tenant, source, severity, fingerprint, labels
- `AlertResolved` — alert resolved event with fingerprint
- `NotificationDispatch` — internal dispatch command with job, policy snapshot, template data
- `DeliveryChanged` — delivery state change event for SSE/Portal projection

### Event Message Types
- `hnb.event.alert.firing.v1` — AlertFiring
- `hnb.event.alert.resolved.v1` — AlertResolved
- `hnb.command.notification.dispatch.v1` — NotificationDispatch
- `hnb.event.notification.delivery-changed.v1` — DeliveryChanged

## Schema Lint and Compatibility
- All 8 JSON Schemas follow Draft 2020-12, use `additionalProperties: false`, and include `$id` references.
- Protobuf messages use field numbers 21-24 (no conflict with existing 1-20).
- No existing schemas were modified; all additions are backward-compatible.

## SDK Generation
- Go SDK: `protoc-gen-go` will generate `AlertFiring`, `AlertResolved`, `NotificationDispatch`, `DeliveryChanged` structs with proper field tags.
- TypeScript SDK: `protoc-gen-es` will generate corresponding TypeScript interfaces.

## Verification
- `contracts/schema/alert/v1/` contains 8 JSON Schema files covering all 8 core models from the design.
- `contracts/proto/hnb/contracts/v1/contracts.proto` contains 4 new alert messages with unique field numbers.
- No existing API or SDK types are modified.