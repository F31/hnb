# Rollout, Shadow Mode, and Runbook Evidence

## Shadow Mode Deployment

1. Deploy Tenant Context Middleware in "shadow" mode:
   - Extract tenant context from JWT
   - Log extracted context and compare with request path
   - Do NOT block any requests
   - Log any mismatches between expected and actual tenant
2. Duration: 1 week
3. Success criteria: < 0.1% mismatch rate

## Grayscale Migration

### Phase 1: Portal-only (1 week)
- Enable tenant management UI in Portal
- Default: platform admin only
- **Rollback**: Disable tenant management feature flag

### Phase 2: Tenant Management (1 week)
- Enable tenant CRUD for platform admin
- **Rollback**: Disable tenant CRUD API

### Phase 3: Role Management (1 week)
- Enable role management and user assignment
- **Rollback**: Disable role management API, roles remain in DB

### Phase 4: Approval Policies (1 week)
- Enable approval policy evaluation
- Operation type binding to approval roles
- **Rollback**: Disable approval policy engine, Operations bypass approval

### Phase 5: Full Rollout (1 week)
- Enable all features for all tenants
- Enable SecretReference service
- Disable shadow mode, enable enforcement

## Runbook

### 1. Tenant Management
- Creating a tenant
- Suspending/deleting a tenant
- Viewing tenant projects and environments

### 2. Project and Environment Management
- Creating projects under a tenant
- Creating environments under a project (production/staging/development)

### 3. Namespace Management
- Creating namespaces under an environment (auto-generated name)
- Viewing namespace details and labels
- Managing multi-namespace environments (e.g., production with api/worker/cache)
- Suspending/deleting namespaces
- Namespace naming convention: `{tenant}-{project}-{env}[-{suffix}]`

### 4. Role Management
- Creating roles with permissions
- Assigning roles to users (tenant/project/namespace scope)
- Understanding role inheritance

### 5. Approval Policies
- Creating approval policies for operation types
- Approving/rejecting pending operations
- Understanding approval flow

### 6. SecretReference Management
- Creating secrets
- Rotating secrets
- Resolving secret references in Provider context

### 7. Troubleshooting
- Cross-tenant access denied
- Cross-project namespace access denied
- Role assignment conflicts
- Secret resolution failures
- Approval policy not triggering
- Namespace name collision (DNS-1123 format check)

## Disaster Recovery

- Secret data is backed up as part of database backup
- Master key is backed up separately (key management service)
- Recovery: restore DB, restore master key, verify secret resolution
- RPO: 5 minutes (Standard HA), RTO: 5 minutes
- Cross-tenant isolation is verified after recovery