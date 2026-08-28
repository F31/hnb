# Source Adapter and Normalizer Design Evidence

## Architecture

```
Source Event (external)
    |
    v
Source Adapter Interface
    - Validates tenant/resource context
    - Normalizes severity, source, resource_ref
    - Generates fingerprint
    v
Alert Normalizer
    - Checks fingerprint dedup
    - Creates or updates AlertInstance
    - Applies silence/inhibition/maint window
    - Evaluates NotificationPolicy
    - Creates NotificationJob + Delivery + Outbox
    v
Alert Store (PostgreSQL)
```

## Source Adapter Interface

```go
type SourceAdapter interface {
    // Normalize converts a source-specific event into a normalized alert fact.
    Normalize(ctx context.Context, sourceEvent *SourceEvent) (*NormalizedAlert, error)

    // ValidateTenant ensures the source event belongs to a known tenant.
    ValidateTenant(ctx context.Context, tenantID string) error

    // ValidateResource ensures the referenced resource exists within the tenant.
    ValidateResource(ctx context.Context, tenantID, resourceRef string) error
}
```

## Normalizer Flow

1. Receive source event via Source Adapter
2. Validate tenant and resource context
3. Generate canonical fingerprint: `SHA256(tenant + source + resource + rule + sorted_labels)`
4. Look up existing active AlertInstance by fingerprint
5. If found: update last_seen_at, increment occurrence_count, apply new severity if escalated
6. If not found: create new AlertInstance with state=Pending → Firing
7. Apply silence/inhibition checks
8. Evaluate NotificationPolicy for routing
9. Create NotificationJob + DeliveryRecord + Outbox event in same transaction
10. Publish `hnb.event.alert.firing.v1` via JetStream

## Supported Source Types

| Source Type | Adapter | Example Event |
|-------------|---------|---------------|
| Operation | OperationAdapter | OperationStalled, OperationFailed |
| Provider | ProviderAdapter | ProviderTimeout, ProviderError |
| Security | SecurityAdapter | VulnerabilityDetected, PolicyViolation |
| Gateway | GatewayAdapter | GatewayLatency, GatewayError |
| Resource | ResourceAdapter | ResourceQuotaExceeded, ResourceDegraded |
| External | ExternalAdapter | Prometheus Alertmanager webhook, CloudWatch SNS |

## Test Plan
- Adapter contract test: verify each source type produces correct normalized output
- Normalizer unit test: dedup, state transition, silence application
- Negative test: invalid tenant, unknown resource, missing fingerprint fields
- Edge case: recovery event arriving before firing (should be logged as anomalous, not create resolved alert)