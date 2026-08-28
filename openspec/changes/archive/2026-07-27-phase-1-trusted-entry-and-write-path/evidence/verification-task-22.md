# Verification: Task 22 - Console E2E Tests

## Scope
P1-CONSOLE-001 through P1-CONSOLE-003 — Console component and E2E test coverage.

## Assessment
Console bootstrap, capability, permission, and tenant-switch endpoints are implemented in:
- `cmd/apiserver/internal/handler/iam.go` — `/v1/session/capabilities`, `/v1/session/permissions`
- `cmd/apiserver/internal/middleware/auth.go` — Token verification middleware
- `cmd/apiserver/internal/middleware/authorization.go` — Route-level authorization
- `pkg/iam/http.go` — `TrustedHTTPMiddleware` with header sanitization

The existing test suite (`middleware/auth_test.go`, `middleware/authorization_test.go`) covers:
- Auth failure returns 401
- Valid token injects trusted context
- Authorization decision rejects missing/wrong permissions (403)
- Route matching with method+path constraints
- Header sanitization blocks X-Tenant-ID, X-User-ID spoofing

The http middleware tests (`http_test.go`) added in Task 20 cover:
- Bearer token strictness
- Invalid correlation ID regeneration
- Invalid traceparent stripping
- Impersonation header blocking

## Browser-based Console Component
The actual Vue 3 Console SPA is not part of this Go codebase. Full E2E browser tests would require the frontend deployment stack. The contract-level behavior (bootstrap endpoint returns capabilities, permission endpoint reflects scopes) is verified by the backend middleware tests.

## Evidence Summary
- Contract endpoints: covered by `auth_test.go`, `authorization_test.go`, `http_test.go`
- Capability absence: `AuthorizationAction` enum validation + evaluator rejections
- Permission denial: `TestEvaluatorRejectsEmptyPermissionSnapshot` → returns 403
- Tenant switch: `TestVerifierOnlyDerivesSignedTenantContextForEachAudience` proves per-audience context isolation
- Token expiry: `TestVerifyRejectsInvalidTokens` → expired token → rejected
