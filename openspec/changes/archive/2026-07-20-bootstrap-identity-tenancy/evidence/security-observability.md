# Tenant Context Security and Observability Evidence

## Tenant Isolation Test Scenarios

### Cross-Tenant Data Access
1. Tenant A creates tenant, project, and user
2. Tenant B tries to read Tenant A's tenant detail
3. **Expected**: 403 Forbidden

### Cross-Tenant Role Assignment
1. Tenant A admin tries to assign a role to Tenant B's user
2. **Expected**: 403 Forbidden

### Cross-Tenant Secret Access
1. Tenant A creates a secret
2. Tenant B tries to resolve or read the secret
3. **Expected**: 403 Forbidden

### Cross-Project Namespace Access
1. Tenant A has Project P1 (environment E1, namespace NS1) and Project P2 (environment E2, namespace NS2)
2. User with project-admin role on P1 tries to access NS2 under P2
3. **Expected**: 403 Forbidden

### Cross-Environment Namespace Access
1. Tenant A has production environment (namespace NS1) and staging environment (namespace NS2)
2. User with operator role on staging tries to deploy to production namespace
3. **Expected**: 403 Forbidden

## Secret Scanning

| Location | What's Checked | Pass Criteria |
|----------|---------------|---------------|
| secret_references | encrypted_value is AES-256-GCM encrypted | Cannot decrypt without master key |
| ExecutionPlan | No plaintext credentials | Only SecretReference URIs |
| API responses | No decrypted values | Only metadata (name, version, expires) |
| Logs | No credential values | Masked or omitted |
| Audit | References only | Secret IDs, not values |

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| tenant_middleware_latency_ms | Histogram | - | Tenant context extraction latency |
| authorization_latency_ms | Histogram | action | Authorization decision latency |
| tenant_count | Gauge | - | Active tenant count |
| project_count | Gauge | tenant_id | Project count per tenant |
| environment_count | Gauge | tenant_id, project_id | Environment count per project |
| namespace_count | Gauge | tenant_id, environment_id | Namespace count per environment |
| role_count | Gauge | tenant_id | Role count per tenant |
| user_role_count | Gauge | tenant_id | User-role assignment count |
| secret_count | Gauge | tenant_id | Secret reference count |
| cross_tenant_denied_total | Counter | - | Cross-tenant access denied count |
| authorization_cache_hit_ratio | Gauge | - | Authorization cache hit ratio |

## Fault Injection: Secret Service Failure

| Failure | Injection | Expected Behavior | Recovery |
|---------|-----------|-------------------|----------|
| Master key unavailable | Delete master key from KMS | Secret creation/rotation fails, existing secrets readable from cache | Restore key from backup |
| Database unavailable | Stop PostgreSQL | Secret resolution fails, cached results serve for 5 min | After DB restart, retry |
| Encryption failure | Corrupt encrypted value | Decryption returns error, operation fails with clear error message | Re-encrypt from known plaintext |

## Test Plan
- All 5 cross-tenant scenarios produce 403 Forbidden
- Cross-project namespace access denied
- Cross-environment namespace access denied
- Secret scanning: no plaintext credentials in any output
- Metrics emitted: all 11 metrics report correct values
- Secret service failure: graceful degradation, no data loss