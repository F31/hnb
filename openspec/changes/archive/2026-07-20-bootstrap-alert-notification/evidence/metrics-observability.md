# Notification Metrics and Dashboard Evidence

## Metrics

| Metric Name | Type | Labels | Description |
|-------------|------|--------|-------------|
| alert_source_to_firing_seconds | Histogram | source, tenant | Time from source event to alert instance creation |
| alert_firing_to_portal_seconds | Histogram | tenant | Time from firing to Portal SSE update |
| alert_firing_to_first_attempt_seconds | Histogram | channel_type, tenant | Time from firing to first delivery attempt |
| notification_jobs_pending | Gauge | channel_type, tenant | Current number of pending notification jobs |
| notification_jobs_oldest_age_seconds | Gauge | channel_type, tenant | Age of the oldest pending job |
| notification_delivery_success_total | Counter | channel_type, tenant | Successful deliveries |
| notification_delivery_retry_total | Counter | channel_type, tenant | Retry attempts |
| notification_delivery_failed_total | Counter | channel_type, tenant | Permanent failures |
| notification_suppressed_total | Counter | reason | Suppressed notifications (silence, inhibition, etc.) |
| notification_circuit_breaker_state | Gauge | channel_type, tenant | 0=closed, 1=open, 2=half_open |
| provider_availability | Gauge | provider | 1=healthy, 0=unhealthy |

## Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| NotificationJobsPilingUp | `notification_jobs_pending > 1000` | warning |
| DeliveryFailureRateHigh | `rate(notification_delivery_failed_total[5m]) > 0.1` | warning |
| OldestJobTooOld | `notification_jobs_oldest_age_seconds > 300` | critical |
| CircuitBreakerOpen | `notification_circuit_breaker_state == 1` | critical |
| ProviderDown | `provider_availability == 0` | critical |

## Dashboard Panels

1. **Alert Pipeline**: Source-to-Firing latency, Firing-to-Portal latency, Firing-to-First-Attempt latency
2. **Queue Depth**: Pending jobs, oldest job age, by channel type
3. **Delivery Success Rate**: Success/failure rate, retry rate, by channel
4. **Circuit Breakers**: State of each channel's circuit breaker
5. **Provider Health**: Availability of each provider

## Trace Context

All operations carry:
- `correlation_id` — ties source event → alert → delivery → attempt
- `causation_id` — parent event chain
- `operation_id` — if alert is linked to an operation
- `trace_id` — W3C traceparent for distributed tracing

## Logging Policy

Logs include:
- Alert ID, tenant ID, channel type, delivery ID, attempt ID
- Result class, duration, HTTP status code
- Trace ID

Logs NEVER include:
- Raw email addresses, phone numbers, or names
- Secret values, tokens, passwords, API keys
- Full message body (only summary and masked destination)

## Test Plan
- Metric emission: all metrics are emitted correctly
- Alert firing: alerting rules fire at correct thresholds
- Dashboard: panels render with correct data
- Trace context: correlation_id flows through the entire pipeline
- Log safety: logs contain no secrets or PII