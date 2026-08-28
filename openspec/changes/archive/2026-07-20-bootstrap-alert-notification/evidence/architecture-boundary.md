# Architecture Boundary Evidence

## Decision: Metrics/Logs/Traces Backend and Alertmanager — N/A

This change (`bootstrap-alert-notification`) defines the **Alert/Notification domain service** and its **Source Adapter** interface. It does NOT:

- **Choose a metrics backend** (Prometheus, Thanos, VictoriaMetrics, etc.) — these are existing infrastructure decisions, not part of this change.
- **Choose a logs backend** (Loki, Elasticsearch, etc.) — logs storage is out of scope.
- **Choose a traces backend** (Tempo, Jaeger, etc.) — distributed tracing is out of scope.
- **Choose a specific Alertmanager product** — the Source Adapter interface allows any monitoring system to feed normalized alerts into HNB's Alert Store.

### What This Change Defines

| Artifact | Scope | Out of Scope |
|----------|-------|-------------|
| Source Adapter | Tenant/resource validation, fingerprint generation, Firing/Resolved pairing | Metrics collection, log ingestion, tracing pipeline |
| Alert Normalizer | Event normalization, dedup, state machine, aggregation | Metric rule evaluation, log parsing, trace analysis |
| Alert Store | AlertInstance, Policy, Delivery persistence | Long-term metrics storage, log retention, trace storage |
| Notification Dispatcher | Job creation, JetStream dispatch, channel delivery | SMTP server, HTTP server, SMS gateway |

### Rationale

- HNB's architecture separates concerns: metrics/logs/traces are owned by the Observability plane, not the Alert/Notification domain.
- A Source Adapter can be implemented for any monitoring system (Prometheus Alertmanager, CloudWatch, Datadog, etc.) without changing the Core.
- This prevents vendor lock-in and keeps the alert domain focused on routing, notification, and lifecycle management.

## Verification
- No metrics, logs, traces, or Alertmanager backend schema is defined in this change.
- The Source Adapter pattern is documented as an SPI, not a concrete implementation.
- All T1 components (Normalizer, Dispatcher, Email/Webhook Workers) are domain services, not observability backends.