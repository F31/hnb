# Tenant Management Portal UI Design Evidence

## Vue Component Architecture

```
TenantManagement (page component)
  ├── TenantList (table with search, pagination)
  │   └── TenantListItem (name, status, project count, action buttons)
  ├── TenantDetail (form with tabs)
  │   ├── GeneralTab (name, display name, status)
  │   ├── ProjectsTab (list + create/edit)
  │   │   └── ProjectDetail
  │   │       ├── EnvironmentsTab (list + create/edit)
  │   │       │   └── EnvironmentDetail
  │   │       │       └── NamespacesTab (list + create/edit)
  │   │       │           └── NamespaceDetail (name, status, labels, description)
  │   ├── RolesTab (list + create/edit permissions)
  │   ├── UsersTab (role assignment, grant/revoke)
  │   └── ApprovalPoliciesTab (list + create/edit)
  └── SecretReferenceManagement (page component)
      ├── SecretList (table with name, version, expires)
      └── SecretDetail (create, view, rotate, delete)
```

## API Endpoints

| Method | Path | Permission |
|--------|------|-----------|
| GET | `/tenants` | tenant:manage |
| POST | `/tenants` | tenant:manage |
| GET | `/tenants/{id}` | tenant:read |
| PUT | `/tenants/{id}` | tenant:manage |
| DELETE | `/tenants/{id}` | tenant:manage (soft delete) |
| GET | `/tenants/{id}/projects` | tenant:read |
| POST | `/tenants/{id}/projects` | project:manage |
| GET | `/projects/{id}/environments` | tenant:read |
| POST | `/projects/{id}/environments` | project:manage |
| GET | `/environments/{id}/namespaces` | tenant:read |
| POST | `/environments/{id}/namespaces` | project:manage |
| GET | `/namespaces/{id}` | tenant:read |
| PUT | `/namespaces/{id}` | project:manage |
| DELETE | `/namespaces/{id}` | project:manage (soft delete) |
| GET | `/tenants/{id}/roles` | tenant:read |
| POST | `/tenants/{id}/roles` | role:manage |
| POST | `/tenants/{id}/users/{userId}:grant` | user:assign |
| POST | `/tenants/{id}/users/{userId}:revoke` | user:assign |
| GET | `/approval-policies` | tenant:read |
| POST | `/approval-policies` | role:manage |
| GET | `/secrets` | secret:manage |
| POST | `/secrets` | secret:manage |
| POST | `/secrets/{id}:rotate` | secret:manage |

## RBAC in UI

- Platform admin: sees all tenants, can manage everything
- Tenant admin: sees only their tenant, can manage projects/roles/users within
- Project admin: sees only their project, can assign users within
- Operator: sees operational views, no tenant/role management

## Test Plan
- Tenant CRUD: create, read, update, soft delete tenant
- Project management: create project within tenant
- Environment management: create environment under project
- Namespace CRUD: create, read, update, soft delete namespace under environment
- Multi-namespace: create multiple namespaces under one environment
- Auto-naming: namespace name follows `{tenant}-{project}-{env}` format
- Role assignment: grant/revoke user role within tenant
- Cross-tenant isolation: tenant admin cannot see other tenants
- Secret management: create, view, rotate, delete secret
- Permission enforcement: operator cannot access tenant management