# Webhook Worker Design Evidence

## Architecture

```
Notification Dispatcher
    |
    v
Webhook Worker (JetStream Consumer)
    |
    ├── Resolve webhook config from SecretReference
    ├── Compute HMAC signature
    ├── Add timestamp, idempotency key, nonce
    ├── Send HTTPS POST to webhook URL
    ├── Validate response
    ├── Record attempt (DeliveryAttempt)
    └── Update DeliveryRecord state
```

## Webhook Configuration

```go
type WebhookConfig struct {
    URL             string          // target URL (validated for SSRF)
    SecretRef       SecretReference // HMAC signing key
    Timeout         time.Duration   // default 10s
    RetryMax        int             // default 5
    AllowedDomains  []string        // SSRF allowlist
    AllowedCIDRs    []string        // optional IP range allowlist
    Headers         map[string]string // static headers
}
```

## HMAC Signing

```
payload = timestamp + "." + idempotencyKey + "." + requestBody
signature = HMAC-SHA256(payload, secret)
Header: X-HNB-Signature: t={timestamp},s={signature},k={idempotencyKey}
```

## Replay Protection

- Each request includes a unique `idempotencyKey` and `timestamp`
- Receiver SHOULD reject requests with timestamps outside ±5min window
- Receiver SHOULD use `idempotencyKey` for dedup within 24h
- HNB Worker tracks which keys have been delivered (for retry dedup)

## Response Handling

| HTTP Status | Result | Delivery State |
|-------------|--------|----------------|
| 2xx | Success | Accepted |
| 4xx | Client error | Failed (permanent_failure) |
| 5xx | Server error | Failed (transient_failure) |
| Timeout | Timeout | Failed (timeout) |

## Test Plan
- HMAC: verify signature is computed correctly and receiver can validate
- Retry: 5xx response -> retry with backoff
- Idempotency: retry sends same idempotencyKey, new timestamp
- Timeout: 10s timeout -> timeout result class
- Response validation: 2xx -> Accepted, 4xx -> permanent_failure
- Custom headers: static headers are included in request