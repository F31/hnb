# Tenant Context, RBAC, and SecretReference Schema Evidence

## Created Schema Files

### JSON Schema (5 files)
- `contracts/schema/identity/v1/tenant-context.schema.json` — TenantContext with tenantId, projectId, environmentId, namespaceId, actorId, roles, correlationId
- `contracts/schema/identity/v1/namespace.schema.json` — Namespace mapping to K8s namespace with DNS-1123 name validation
- `contracts/schema/identity/v1/rbac-role.schema.json` — RbacRole with 6 roles, permissions array, inheritance
- `contracts/schema/identity/v1/approval-policy.schema.json` — ApprovalPolicy binding operation types to required roles
- `contracts/schema/identity/v1/secret-reference.schema.json` — SecretReference with encrypted value, AES-256-GCM, rotation policy, versioning

### Registry
- `contracts/schema/identity/v1/identity-registry.json` — Index of all 5 identity schemas

### Protobuf Messages (7 messages in `contracts.proto`)
- `RequestContext` — extended with `namespace_id = 4`, `project_id`/`environment_id` made required
- `TenantContext` — full tenant context with `namespace_id = 4`, `project_id`/`environment_id` made required
- `AuthorizationRequest` — extended with `project_id`, `environment_id`, `namespace_id`
- `AuthorizationResponse` — authorization result with reason
- `SecretReferenceMsg` — secret reference message for events
- `NamespaceRef` — namespace reference with id, tenant_id, project_id, environment_id, name, status, labels

### API Endpoints Schema
- `GET/POST /tenants` — Tenant CRUD
- `GET/POST /tenants/{id}/projects` — Project management
- `GET/POST /projects/{id}/environments` — Environment management (scoped to project)
- `GET/POST /environments/{id}/namespaces` — Namespace CRUD (scoped to environment)
- `GET /namespaces/{id}` — Namespace detail
- `PUT /namespaces/{id}` — Update namespace (labels, status)
- `DELETE /namespaces/{id}` — Soft delete namespace
- `GET/POST /tenants/{id}/roles` — Role management
- `POST /tenants/{id}/users/{userId}:grant` — Role assignment
- `GET/POST /approval-policies` — Approval policy management
- `GET/POST /secrets` — SecretReference management

## Verification
- All 5 JSON Schema files follow Draft 2020-12 with `additionalProperties: false`
- Protobuf messages use non-conflicting field numbers (1-9 within each message)
- Namespace `name` pattern enforces DNS-1123 compliance: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` with max 63 chars
- `secret-ref` pattern enforces `secret://tenant/{tenant_id}/{secret_name}` format
- Role enum enforces exactly 6 roles matching the spec
- TenantContext: `projectId` and `environmentId` are now required fields