# Trusted Identity Task 11 Evidence

Date: 2026-07-26

## Status

Task 11 is complete for the three in-scope public entry points:
`cmd/apiserver`, `cmd/platform-api`, and `cmd/app-market`. Tasks 12-14 and
verification task 20 remain unchecked.

## Implemented Scope

- `pkg/iam` owns the single `hnb.identity/v1` ES256 access-token profile, typed
  `TrustedContext`, verifier-only construction, public-key-only PEM loading,
  and reusable strict Bearer `net/http` middleware.
- Access credentials are standard three-segment compact JWS tokens signed and
  verified as ES256. Verification checks `typ`, `alg`, `kid`, profile, issuer,
  presence of the verifier's explicit non-wildcard audience, `iat`, `nbf`,
  `exp`, `sub`, subject type, selected `tenantId` and `membershipId`, membership
  inclusion, `jti`, key/algorithm agreement, signature, maximum lifetime, and
  an 8 KiB token limit. `none`, algorithm substitution, wildcard/wrong
  audiences, unknown keys, malformed credentials, and out-of-window tokens are
  rejected.
- The issuer accepts an explicit audience list and signs all approved audiences
  (`hnb-apiserver`, `hnb-platform-api`, and `hnb-app-market` by default in the
  chart). Each service requires its own audience rather than an exact singleton.
  Access TTL is configuration-enforced at no more than 60 seconds, which is the
  maximum revocation propagation bound for verifier-only services.
- `tenantId` and selected `membershipId` are signed claims, and the selected
  membership must occur in `tenantMembershipIds`. JSON Schema, Protobuf,
  OpenAPI semantics, Go, and TypeScript generated output are aligned.
- The minimal `KeyProvider` and `KeyRing` boundaries contain no database key
  material. Only apiserver loads the P-256 signing key. Platform API and App
  Market fail fast while loading named P-256 public keys and have no private-key
  configuration or IAM database dependency.
- `IAMDBStore` maps existing users to canonical `identity_subjects` under an
  explicit issuer. Active memberships are derived only from distinct,
  non-revoked `user_roles.tenant_id` values. An optional `membership_id` login
  selector must belong to that subject; missing membership fails closed.
- Migration `027_trusted_identity_tokens.sql` adds the legacy-user bridge and a
  purpose-constrained refresh table. Refresh credentials are 256-bit opaque
  values; only SHA-256 hashes are stored, and rotation consumes the previous
  hash transactionally. Access and refresh plaintext are not persisted by the
  new path. Rollback intentionally retains identity and credential-use
  evidence.
- `cmd/apiserver` Login and auth middleware share the same `TokenManager`.
  Issuance resolves the selected membership before signing. Authentication
  verifies first, then resolves membership in IAM in real time and checks
  subject type, membership, and tenant against signed claims.
  Protected routes accept one strict Bearer header only, never query tokens.
  The middleware removes inbound tenant/user/subject/identity/membership/actor/
  workspace/role/permission/approval headers, rechecks active subject and
  membership state, removes Authorization after verification, and injects the
  typed trusted context.
- Correlation uses a valid `X-Correlation-ID` UUID or a server-generated UUID.
  Invalid traceparent values are removed.
- Tenant, cluster, and IAM permission handlers consume `TrustedContext` rather
  than identity headers. Cluster Get/Delete and nested tenant reads include the
  authenticated tenant predicate.
- Proxy request and response headers use the explicit Content-Type, Accept,
  If-Match, Idempotency-Key, X-Correlation-ID, and traceparent allowlist.
- Audit middleware omits login/refresh request and response bodies and
  recursively redacts sensitive fields from other JSON bodies.
- `cmd/platform-api` protects every route except `/healthz`. Submit/list/get,
  approve/reject/cancel, and target handlers use trusted tenant/subject values;
  body/query identity values are ignored. Existing arbitrary-step submission is
  unchanged because RuntimeIntent replacement remains task 15.
- `cmd/app-market` protects `/api/v1`; `/health` and `/metrics` are explicit
  exceptions. Existing tenant/user headers and body `TenantID`/`CreatedBy` are
  replaced by trusted context. App Market does not query IAM tables and its
  deployment requires an independent App Market DSN.
- Platform API and App Market Helm/compose configuration mounts only an
  operator-supplied public key. Compose exposes their ports only to the internal
  network and does not publish host ports.

## Verification

- `gofmt` on all changed Go files: passed.
- `cd pkg/iam && go test -race ./...`: passed.
- `cd cmd/apiserver && go test -race ./...`: passed.
- `cd cmd/platform-api && go test -race ./...`: passed, including no-token,
  wrong-audience, body/query spoofing, and typed-context coverage.
- `cd cmd/app-market && go test -race ./...`: passed, including no-token,
  wrong-audience, header/body spoofing, and typed-tenant coverage.
- `openspec validate --all --strict --no-interactive`: 31 passed, 0 failed.
- `npm run contracts:generate -- --check`: generated output matches.
- `npm run contracts:check`: 15 contract tests and validation passed.
- `docker compose ... config --quiet`: passed with external public-key and
  independent App Market DSN inputs.
- `helm template`: not run because the Helm CLI is unavailable in this
  environment; the template was statically inspected.

## Remaining Blockers

- Full tenant/project/environment/namespace/resource/action authorization and
  complete object-level repository predicates remain task 12. Task 11 makes
  trusted identity authoritative but does not claim scope authorization.
- App Market's existing object repositories are not comprehensively tenant-
  predicated in this task; that hardening remains task 12.
- Signing-key overlap, rotation, emergency revocation, and their drills remain
  task 14/25. This task provides only one active signing key and a verification
  key set boundary.
- Verification task 20 remains open because its cache invalidation, full scope
  mismatch, authorization, and broader security matrix exceed task 11.
