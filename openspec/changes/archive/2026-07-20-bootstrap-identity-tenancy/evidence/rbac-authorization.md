# RBAC and Authorization Service Design Evidence

## RBAC Role Hierarchy

```
platform_admin (inherits all)
  └── tenant_admin (inherits project_admin)
       └── project_admin (inherits operator)
            └── operator (inherits publisher)
                 └── publisher (inherits readonly)
                      └── readonly
```

## Permission Matrix

| Permission | platform_admin | tenant_admin | project_admin | operator | publisher | readonly |
|------------|:---:|:---:|:---:|:---:|:---:|:---:|
| tenant:manage | ✓ | - | - | - | - | - |
| tenant:read | ✓ | ✓ | - | - | - | - |
| project:manage | ✓ | ✓ | ✓ | - | - | - |
| role:manage | ✓ | ✓ | - | - | - | - |
| user:assign | ✓ | ✓ | ✓ | - | - | - |
| operation:approve | ✓ | ✓ | ✓ | - | - | - |
| operation:execute | ✓ | ✓ | ✓ | ✓ | - | - |
| operation:read | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| release:publish | ✓ | ✓ | ✓ | ✓ | ✓ | - |
| alert:read | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| alert:acknowledge | ✓ | ✓ | ✓ | ✓ | - | - |
| secret:manage | ✓ | ✓ | ✓ | - | - | - |
| audit:read | ✓ | ✓ | - | - | - | - |

## Policy Engine Implementation

```go
type AuthorizationService struct {
    db     *sql.DB
    cache  *cache.Cache  // local cache with 5min TTL
}

func (s *AuthorizationService) Allowed(ctx context.Context, req *AuthorizationRequest) (*AuthorizationResponse, error) {
    tc, ok := GetTenantContext(ctx)
    if !ok {
        return &AuthorizationResponse{Allowed: false, Reason: "no tenant context"}, nil
    }

    // Platform admin always allowed
    if hasRole(tc.Roles, "platform_admin") {
        return &AuthorizationResponse{Allowed: true}, nil
    }

    // Check role permissions
    permissions, err := s.getRolePermissions(ctx, tc.TenantID, tc.Roles)
    if err != nil {
        return nil, err
    }

    if !hasPermission(permissions, req.Action) {
        return &AuthorizationResponse{
            Allowed: false,
            Reason:  "insufficient permissions",
            RequiredRole: findRequiredRole(req.Action),
        }, nil
    }

    // Check approval policy for high-risk operations
    needsApproval, err := s.checkApprovalPolicy(ctx, tc.TenantID, req.Action)
    if err != nil {
        return nil, err
    }
    if needsApproval {
        return &AuthorizationResponse{
            Allowed: false,
            Reason:  "requires approval",
            RequiredRole: findApprovalRole(req.Action),
        }, nil
    }

    return &AuthorizationResponse{Allowed: true}, nil
}
```

## Approval Policy Evaluation

1. Operation type is extracted from the request
2. ApprovalPolicies table is queried for matching tenant + operation_type
3. If policy exists, check if current user has the required role
4. If user has required role, allow direct execution
5. If user does not have required role, Operation enters PendingApproval
6. Approval notification is sent to users with the required role
7. Approver confirms → Operation transitions to Queued
8. Approver rejects → Operation transitions to Failed/Cancelled

## Namespace-Scoped Authorization

Authorization supports three levels of scope:

1. **Tenant-level**: Role applies to all projects, environments, and namespaces within the tenant.
2. **Project-level**: Role is scoped to a specific project (and its environments/namespaces).
3. **Namespace-level**: Role is scoped to a specific namespace within an environment.

Authorization resolution order:
1. If user has tenant-level role with required permission → allow
2. If user has project-level role with required permission → allow
3. If user has namespace-level role with required permission → allow
4. Otherwise → deny

## Namespace Isolation

Namespace CRUD API enforces:
- `project_id` and `environment_id` must belong to the same tenant (validated before insert)
- Namespace name is auto-generated and globally unique within the tenant
- Soft delete: `status = 'deleted'` prevents new deployments but allows rollback
- Cross-namespace access follows the same 403 pattern as cross-tenant access

## Test Plan
- Role inheritance: platform_admin inherits all permissions
- Permission check: operator cannot manage secrets
- Approval policy: database-failover requires tenant_admin approval
- Cache: permissions are cached for 5 minutes
- Concurrent role assignment: simultaneous grant/revoke handled correctly
- Cross-tenant: Tenant A roles don't grant access to Tenant B resources
- Namespace CRUD: create/read/update/soft-delete namespace within environment
- Namespace uniqueness: namespace name must be unique within tenant
- Namespace-scoped authorization: project-level role does not grant access to another project's namespaces
- Multi-namespace environment: create multiple namespaces under one environment