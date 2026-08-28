# Secret and PII Scanning Evidence

## Scan Targets

| Location | What's Checked | Pass Criteria |
|----------|---------------|---------------|
| Templates | Restricted field access (no Secret, Token, Kubeconfig, Password) | Template validation rejects forbidden fields |
| NATS messages | Payload does not contain resolved credentials | Only references, masked values |
| Logs | No raw email, phone, or secret values | Masked values only |
| Traces | No sensitive fields in span attributes | Sanitized before export |
| Audit | No PII in audit records | References only (IDs, masked values) |

## Forbidden Fields in Templates

The following fields are BLOCKED in all notification templates:
- `{{.Secret}}`, `{{.Token}}`, `{{.Kubeconfig}}`, `{{.Password}}`
- `{{.PrivateKey}}`, `{{.CertificateKey}}`, `{{.AccessKey}}`, `{{.APIKey}}`
- `{{.RawEmail}}`, `{{.RawPhone}}`, `{{.FullContact}}`

## PII Handling Rules

| Data Type | Storage | Logging | API Response |
|-----------|---------|---------|-------------|
| Email | Encrypted at rest | Masked (`u***@example.com`) | Masked |
| Phone | Encrypted at rest | Masked (`+86****1234`) | Masked |
| Name | Encrypted at rest | First char + `***` | Masked |
| User ID | Plain text | Plain text | Plain text |
| Tenant ID | Plain text | Plain text | Plain text |

## Test Plan
- Template scan: template with {{.Secret}} is rejected at save time
- NATS scan: message payload contains no resolved credentials
- Log scan: log output contains no raw email or phone
- Audit scan: audit records contain no PII
- API scan: API responses return masked contact data