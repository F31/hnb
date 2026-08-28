# Tenant Isolation and Authorization Audit Evidence

## Isolation Points

| Layer | Mechanism | Scope |
|-------|-----------|-------|
| Database | RLS + WHERE tenant_id = ? | All alert/notification tables |
| API | JWT tenant context validation | All endpoints |
| SSE | Filter events by tenant before push | Alert events stream |
| Worker | Verify tenant context before processing | All channel workers |
| Portal | Tenant-scoped UI components | Alert center, config pages |

## Test Scenarios

### Cross-Tenant Alert Access
1. Tenant A creates alert
2. Tenant B user tries to read alert detail
3. **Expected**: 403 Forbidden

### Cross-Tenant SSE Events
1. Tenant A alert fires
2. Tenant B has SSE connection open
3. **Expected**: Tenant B receives no event

### Cross-Tenant Policy Modification
1. Tenant A admin tries to modify Tenant B's policy
2. **Expected**: 403 Forbidden

### Audit Trail
1. All cross-tenant access attempts are logged
2. Audit records include: actor_id, target_tenant, action, result, timestamp

## Verification
- All 4 test scenarios pass
- Audit logs capture all denied cross-tenant access attempts
- No tenant can read, modify, or delete another tenant's data