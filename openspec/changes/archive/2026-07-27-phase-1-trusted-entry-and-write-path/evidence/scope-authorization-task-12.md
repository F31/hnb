# Task 12 Scope Authorization Evidence

Date: 2026-07-26
Task: P1-ING-003, OpenSpec task 12

## Architecture

- The apiserver IAM issuer resolves the active policy version and canonical
  permissions from `tenant_memberships`, `authorization_policy_versions`,
  `scoped_role_bindings`, and `scoped_roles` before signing an access token.
- Access tokens are ES256 signed, have a maximum 60-second lifetime, and carry
  `policyVersion` plus at most 64 `scopedPermissions`. Empty permission arrays
  are valid, but protected operations deny by default.
- Platform API and App Market use verifier-only IAM components and do not read
  IAM persistence. All three entry points evaluate the signed snapshot using
  exact method and Go route-pattern metadata.
- Authorization uses exact tenant matching and optional project,
  environment, namespace, resource, and action selectors. Missing or invalid
  policy data, invalid hierarchy, unknown routes, and missing grants deny.

## Route Coverage

- apiserver metadata covers health/readiness/OpenAPI, login/refresh/logout,
  users, roles, bindings, permission checks, workspaces, nested projects and
  environments, clusters, extensions, proxy methods, agents, and audit logs.
  AuthZ runs before tenant enrichment, so denied routes cannot trigger its
  workspace lookup.
- platform-api metadata covers every operation and runtime-target route.
  Approve, reject, and cancel map to distinct authorization actions. Handler
  tests prove every protected route denies before any store call when no grant
  exists, and unknown routes deny by default.
- app-market metadata covers every `/api` route. Handler tests prove every
  protected route denies before handler execution when no grant exists, and
  unknown routes deny by default.

## Repository Coverage

- platform-api operation and runtime-target reads and mutations include the
  verified tenant predicate. Concrete-resource handlers use tenant-bounded
  lookup before applying resource scope, and foreign target UUIDs return 404.
- app-market publisher, product, release, and application Get/List/Create,
  Publish, UpdateStatus, and Delete paths carry tenant predicates. Product and
  release ownership is established through publisher joins; association
  creation uses tenant-bounded `INSERT ... SELECT`; foreign UUIDs return not
  found. Release publish does not reference a nonexistent `updated_at` column.
- apiserver workspace/project/environment, cluster, extension, proxy/agent,
  and audit queries use the trusted tenant predicate. A Postgres integration
  test covers foreign cluster GET/DELETE and foreign nested workspace/project
  queries.

## Security Tests

- Issuance and verification cover signed policy snapshots, tampering, expiry,
  missing policy, wildcard tenant rejection, snapshot bounds, and an empty
  permission snapshot that signs successfully but cannot authorize a protected
  operation.
- Evaluator tests cover subject, tenant, project, environment, namespace,
  resource kind, resource ID, and action matches and mismatches, invalid scope
  hierarchy, reason codes, policy version evidence, and deny-by-default.
- Entry-point tests cover wrong audience, spoofed identity headers/body fields,
  exact route methods/patterns, no-permission 403 without store/handler calls,
  foreign UUID 404, and unknown-route denial.

## Verification

- `npm run contracts:generate`: passed; generated Go and TypeScript artifacts
  regenerated.
- `npm run contracts:check`: passed; 16 contract tests plus schema,
  compatibility, OpenAPI, Buf, TypeScript, and generated-drift checks.
- `go test -race -count=1 ./...` in `pkg/iam`: passed.
- `go test -race -count=1 ./...` in `cmd/apiserver`: passed.
- `go test -race -count=1 ./...` in `cmd/platform-api`: passed.
- `go test -race -count=1 ./...` in `cmd/app-market`: passed.
- `openspec validate phase-1-trusted-entry-and-write-path --strict`: passed.
- `git diff --check`: passed.

PostgreSQL 16 integration was then run against three isolated databases cloned
from a database migrated through `027`:

- apiserver cross-tenant permission and nested-resource tests: passed;
- platform-api Operation and RuntimeTarget store tests: passed;
- app-market tenant-predicate repository tests: passed.

The first platform-api run exposed that RuntimeTarget creation did not satisfy
the reconciled non-null workspace hierarchy. Creation now atomically resolves
or creates the authenticated tenant's `default` workspace, and integration
fixtures create real tenants instead of relying on unconstrained tenant text.
The complete platform-api PostgreSQL suite passed after that correction.

## Scope Exclusions

- No RuntimeIntent implementation was added.
- No Console code was changed.
- No service identity or signing-key rotation behavior was added.
- Tasks 13, 14, 20, and 23 remain unchecked.
