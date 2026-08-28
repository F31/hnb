# SMS Provider and Conformance Harness Evidence

## SMS Provider Interface

```go
type SmsProvider interface {
    // Send sends an SMS message.
    Send(ctx context.Context, req *SmsRequest) (*SmsResponse, error)

    // GetStatus queries delivery status for a previously sent message.
    GetStatus(ctx context.Context, providerMessageID string) (*SmsStatus, error)

    // GetQuota returns current quota usage.
    GetQuota(ctx context.Context) (*QuotaInfo, error)

    // GetBudget returns current billing period spend.
    GetBudget(ctx context.Context) (*BudgetInfo, error)

    // Health returns provider health status.
    Health(ctx context.Context) (*HealthStatus, error)
}

type SmsRequest struct {
    To           string   // phone number (E.164 format)
    From         string   // sender ID or signature
    TemplateID   string   // provider-specific template
    TemplateData map[string]string
    Region       string   // ISO 3166-1 alpha-2
    TenantID     string
    IdempotencyKey string
}

type SmsResponse struct {
    ProviderMessageID string
    Accepted          bool
    Cost              float64
    Currency          string
}

type SmsStatus struct {
    State       string // delivered, failed, pending, unknown
    DeliveredAt *time.Time
    ErrorClass  string
}
```

## Conformance Harness

The Conformance Harness validates an SMS Provider implementation against:

| Test | Description | Pass Criteria |
|------|-------------|---------------|
| Contract | Provider implements all 5 interface methods | Compilation OK |
| Send | Send a test message to a valid number | Returns providerMessageID |
| Receipt | Provider returns valid delivery receipt | Status delivered, signed |
| Region | Provider supports at least one declared region | Region test passes |
| Template | Template rendering works correctly | Rendered output matches expected |
| Cost | Provider returns valid cost per message | Cost > 0, currency valid |
| Quota | Provider returns quota information | Quota fields populated |
| Budget | Provider returns budget information | Budget fields populated |
| Masking | Phone numbers are masked in logs | Pattern: +CC****NNNN |
| Health | Provider health check returns valid status | Status healthy/unhealthy |
| Failure | Provider handles invalid numbers gracefully | Error class returned |
| Timeout | Provider handles slow responses | Timeout after configured duration |

## Example SMS Provider (Twilio-like)

```go
type ExampleSmsProvider struct {
    accountSID string
    authToken  string
    fromNumber string
    client     *http.Client
}

func (p *ExampleSmsProvider) Send(ctx context.Context, req *SmsRequest) (*SmsResponse, error) {
    // POST to SMS API endpoint
    // Parse response
    // Return providerMessageID
}
```

## SMS Provider Registration

1. Provider manifest is validated against `sms-provider-manifest.schema.json`
2. Provider code is loaded as a plugin or compiled-in
3. Provider is registered in the Channel Registry
4. Provider capabilities are exposed to Portal for configuration UI

## Test Plan
- Provider contract: mock provider implements all methods
- Send: success case returns providerMessageID
- Receipt: poll for status, verify delivered
- Budget threshold: budget exceeded -> suppress SMS, use alternate channel
- Quota limit: quota exceeded -> rate limit
- Region support: unsupported region -> error
- Masking: logs show +86****1234, not full number
- Encryption: credentials stored via SecretReference only