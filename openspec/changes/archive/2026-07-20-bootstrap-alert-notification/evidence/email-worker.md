# Email/SMTP Worker Design Evidence

## Architecture

```
Notification Dispatcher
    |
    v
Email Worker (JetStream Consumer)
    |
    ├── Resolve SMTP config from SecretReference
    ├── Render template (language, timezone)
    ├── Establish TLS connection
    ├── Send email via SMTP
    ├── Record attempt (DeliveryAttempt)
    └── Update DeliveryRecord state
```

## SMTP Configuration

```go
type SmtpConfig struct {
    Host         string          // SMTP server hostname
    Port         int             // 25, 465, or 587
    TLSRequired  bool            // always true for production
    Username     string          // from SecretReference
    PasswordRef  SecretReference // never inline
    FromAddress  string
    FromName     string
}
```

## Delivery State Semantics

| State | Condition | Notes |
|-------|-----------|-------|
| Accepted | SMTP server accepted the message | No evidence of delivery |
| Delivered | SMTP server returned DSN with delivery confirmation | Rare, most SMTP servers don't provide this |
| Read | N/A for email | Email workers never mark Read |

## Template Rendering

1. Load template by channel type, language, and version
2. Parse with restricted field access (only Alert.Summary, Alert.Severity, etc.)
3. Reject templates referencing Secret, Token, Kubeconfig, or Password
4. Render with user's timezone for timestamps
5. Apply language-specific formatting

## Test Notification

1. API endpoint: `POST /notification-channels/{id}:test`
2. Sends a test email to the authenticated user's address
3. Includes `X-HNB-Test: true` header
4. Does NOT create a permanent DeliveryRecord
5. Test attempts are fully audited

## Test Plan
- Mock SMTP: send to mock SMTP server, verify Accepted state
- TLS: verify TLS connection is established
- Timeout: SMTP timeout -> transient_failure, retry with backoff
- Rejected: SMTP rejects recipient -> permanent_failure
- No receipt: no DSN -> stays Accepted
- Template: test email renders correctly in zh-CN and en
- Secret check: template referencing {{.Secret}} is rejected at save time