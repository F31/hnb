# Database Tenant Isolation and Security Evidence

## Tenant Isolation Design

All alert/notification tables include a `tenant_id` column (or `tenant_scope` for global rules). Queries MUST always filter by tenant:

| Table | Isolation Column | Type |
|-------|-----------------|------|
| alert_rules | tenant_scope | global/tenant |
| alert_instances | tenant_id | TEXT NOT NULL |
| silences | tenant_id | TEXT NOT NULL |
| maintenance_windows | tenant_id | TEXT NOT NULL |
| notification_policies | tenant_scope | global/tenant/project |
| contact_groups | tenant_id | TEXT NOT NULL |
| schedules | tenant_id | TEXT NOT NULL |
| notification_channels | tenant_id | TEXT NOT NULL |
| user_notification_preferences | tenant_id | TEXT NOT NULL |

### Enforcement Points
1. **API layer**: All alert/notification queries include a `WHERE tenant_id = ?` clause.
2. **SSE/WebSocket**: Event streams are filtered by tenant before transmission.
3. **Worker layer**: Notification workers verify tenant context before processing jobs.
4. **Database**: Row-Level Security (RLS) MAY be enabled for defense-in-depth; application-level enforcement is the primary mechanism.

## Contact Encryption and Masking

### Storage
- Contact email addresses and phone numbers are stored encrypted at rest using PostgreSQL `pgcrypto` extension with tenant-scoped encryption keys.
- The `destination_masked` column in `delivery_records` stores only masked values (e.g., `+86****1234`, `u***@example.com`).
- Audit logs never contain raw contact information — only masked values and references.

### Access
- Only the Notification Dispatcher and Channel Workers have access to decrypted contact data.
- Portal API returns only masked contact info.
- API responses exclude raw contact data unless the caller has explicit `contact:read` permission.

### Masking Rules
| Field Type | Masking Pattern | Example |
|-----------|----------------|---------|
| Email | First 3 chars of local part + ***@domain | `use***@example.com` |
| Phone | Country code + first 2 digits + **** + last 4 digits | `+86****1234` |
| Name | First character + *** | `张***` |

## Minimum Privilege Database Accounts

| Role | Tables | Permissions |
|------|--------|-------------|
| alert_api | All alert/policy/contact/channel tables | SELECT, INSERT, UPDATE (by tenant_id) |
| alert_normalizer | alert_instances, alert_rules, alert_state_audits, silences, maintenance_windows | SELECT, INSERT, UPDATE |
| notification_dispatcher | notification_jobs, delivery_records, delivery_attempts, outbox_events | SELECT, INSERT, UPDATE |
| email_worker | notification_channels (email), delivery_records, delivery_attempts | SELECT, UPDATE |
| webhook_worker | notification_channels (webhook), delivery_records, delivery_attempts | SELECT, UPDATE |
| portal_read | alert_instances, notification_policies, contact_groups, schedules, delivery_records | SELECT (masked) |

### Authentication
- Database accounts use individual credentials, not shared passwords.
- SecretReference is used for all channel credentials (SMTP password, API keys, etc.).
- No inline secrets in configuration files or database functions.

## Verification
- All 10 alert/notification tables include tenant_id or tenant_scope for isolation.
- Contact data is encrypted at rest and masked in API responses.
- 6 database roles with minimum required permissions per component.
- Cross-tenant queries are prevented at the application layer.