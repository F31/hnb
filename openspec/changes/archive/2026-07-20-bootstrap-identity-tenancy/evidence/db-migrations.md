# Identity Database Migration Evidence

## Migration Files Created

| File | Tables | Description |
|------|--------|-------------|
| 005_identity_core.sql | tenants, projects, environments | Core tenant hierarchy |
| 006_identity_rbac.sql | roles, user_roles, approval_policies | RBAC and approval |
| 007_identity_secrets.sql | secret_references, secret_versions | Encrypted credential storage |

## Tables Created

### tenants (005)
- `id`, `name`, `display_name`, `status` (active/suspended/deleted), timestamps

### projects (005)
- `id`, `tenant_id` (FK CASCADE), `name`, unique per tenant

### environments (005)
- `id`, `tenant_id` (FK CASCADE), `project_id` (FK CASCADE), `name`, `env_type` (production/staging/development)
- Unique per project + name

### namespaces (005)
- `id` (auto-generated UUID), `tenant_id` (FK CASCADE), `environment_id` (FK CASCADE), `project_id` (FK CASCADE), `name` (DNS-1123 compliant), `description`, `status` (active/suspended/deleted), `labels` (JSONB)
- Unique per tenant + name (global uniqueness across cluster)

### roles (006)
- `id`, `tenant_id` (FK CASCADE), `name` (6 enum values), `permissions` (JSONB), `inherits_from`

### user_roles (006)
- `user_id`, `tenant_id`, `project_id`, `role_id` (FK CASCADE), `granted_by`, `revoked_at`
- Partial unique index on active (non-revoked) assignments

### approval_policies (006)
- `tenant_id` (FK CASCADE), `operation_type`, `required_roles` (JSONB), `max_pending_duration`
- Unique per tenant + operation_type

### secret_references (007)
- `tenant_id` (FK CASCADE), `name`, `secret_ref`, `encrypted_value`, `algorithm` (AES-256-GCM), `version`, `rotation_policy`, `expires_at`

### secret_versions (007)
- `secret_id` (FK CASCADE), `version`, `encrypted_value`, `created_by`, timestamps

## Key Design Decisions
- Tenant IDs are TEXT (natural key from IdP), not UUID
- `user_roles` uses partial unique index on active records only
- `secret_references` stores encrypted blob, `secret_versions` tracks version history
- `approval_policies` unique per tenant+operation_type ensures one policy per operation type
- `environments` belong to `projects` (not directly to tenants) — forming the hierarchy tenant → project → environment → namespace
- `namespaces` denormalize `tenant_id` and `project_id` alongside `environment_id` for query efficiency
- Namespace `name` follows DNS-1123 label format, auto-generated as `{tenant_id}-{project_id}-{env_type}[-{suffix}]`
- Multi-namespace environments: a single environment can have multiple namespaces (e.g., production → api + worker + cache)

## Verification
- All 3 migrations use `CREATE TABLE IF NOT EXISTS` for idempotency
- Rollback scripts drop all tables in reverse dependency order
- Foreign keys cascade on delete
- CHECK constraints enforce enum values