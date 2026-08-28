# SMS Provider Manifest and SPI Evidence

## Created Schema
- `contracts/schema/alert/v1/sms-provider-manifest.schema.json` — T2 SMS Provider capability declaration manifest.

### Manifest Fields
| Field | Description |
|-------|-------------|
| `metadata.name` | Provider name |
| `metadata.version` | Semver |
| `metadata.vendor` | Vendor name |
| `spec.regions` | ISO 3166-1 alpha-2 region codes |
| `spec.phoneFormats` | Per-region phone number patterns and examples |
| `spec.supportsTemplates` | Template support flag + allowed fields |
| `spec.supportsSignatures` | Signature support with maxLength, approval, allowedChars |
| `spec.supportsReceipts` | Delivery receipt support with format (callback/webhook/polling), signed flag, fields |
| `spec.supportsQuota` | Quota model with maxPerDay, maxPerMonth, resetPeriod |
| `spec.supportsBudget` | Budget model with currency, costPerMessage, monthlyBudget, alertThresholds |
| `spec.dataResidency` | Processing regions, location, data retention days |
| `spec.rateLimit` | maxPerSecond, maxPerMinute |

### SPI Contract
The SMS Provider SPI consists of:
1. **Send(SmsRequest) → SmsResponse** — Send a message; returns providerMessageId on success
2. **GetStatus(providerMessageId) → SmsStatus** — Query delivery status (for polling receipt model)
3. **GetQuota() → QuotaInfo** — Current quota usage
4. **GetBudget() → BudgetInfo** — Current billing period spend
5. **Health() → HealthStatus** — Provider health check

### Key Design Decisions
- Provider is fully optional (T2) — no SMS code path in T1.
- Phone numbers are masked (`+86****1234`) in all logs, delivery records, and Portal.
- Credentials use SecretReference only; never inline in manifest.
- Receipts are verified by channel, tenant, providerMessageId, signature, and time window.
- Budget alerts fire at configurable percentage thresholds (e.g., 50%, 80%, 100%).

## Verification
- SMS Provider manifest schema enforces all required capability fields.
- `additionalProperties: false` ensures no undocumented fields.
- Manifest can be validated at install time to determine provider capabilities.