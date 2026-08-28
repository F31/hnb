# Tenant Context Middleware Design Evidence

## Architecture

```
API Request
    |
    v
Tenant Context Middleware
    |
    ├── Extract JWT from Authorization header
    ├── Parse JWT claims (tenant_id, project_id, environment_id, actor_id, roles)
    ├── Validate token signature and expiry
    ├── Inject TenantContext into Go context
    └── Call next handler
    |
    v
Authorization Middleware
    |
    ├── Extract action and resource from request
    ├── Call AuthorizationService.Allowed()
    ├── If denied: return 403 Forbidden + audit
    └── If allowed: call next handler
    |
    v
Handler (with context.TenantContext available)
```

## JWT Claims Structure

```json
{
  "sub": "user-123",
  "tenant_id": "t1",
  "project_id": "p1",
  "environment_id": "e1",
  "namespace_id": "ns-1",
  "roles": ["tenant_admin", "operator"],
  "iat": 1516239022,
  "exp": 1516242622
}
```

## Middleware Implementation

```go
type TenantContext struct {
    TenantID      string
    ProjectID     string
    EnvironmentID string
    NamespaceID   string
    ActorID       string
    CorrelationID string
    Roles         []string
}

type contextKey string
const tenantContextKey contextKey = "tenant_context"

func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractToken(r)
        claims, err := validateAndParseJWT(token)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        ctx := context.WithValue(r.Context(), tenantContextKey, TenantContext{
            TenantID:      claims.TenantID,
            ProjectID:     claims.ProjectID,
            EnvironmentID: claims.EnvironmentID,
            NamespaceID:   claims.NamespaceID,
            ActorID:       claims.Subject,
            CorrelationID: r.Header.Get("X-Correlation-ID"),
            Roles:         claims.Roles,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func GetTenantContext(ctx context.Context) (TenantContext, bool) {
    tc, ok := ctx.Value(tenantContextKey).(TenantContext)
    return tc, ok
}
```

## Cross-Tenant Access Control

```go
func AuthorizationMiddleware(authService *AuthorizationService) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tc, ok := GetTenantContext(r.Context())
            if !ok {
                http.Error(w, "no tenant context", http.StatusUnauthorized)
                return
            }

            // Extract target tenant from request path or body
            targetTenant := extractTargetTenant(r)

            // Cross-tenant check
            if targetTenant != "" && targetTenant != tc.TenantID {
                if !isExplicitlyAuthorized(tc, targetTenant) {
                    audit.Log(r.Context(), AuditEvent{
                        Action:   "cross_tenant_denied",
                        Actor:    tc.ActorID,
                        Tenant:   tc.TenantID,
                        Target:   targetTenant,
                        Resource: r.URL.Path,
                        Result:   "denied",
                    })
                    http.Error(w, "cross-tenant access denied", http.StatusForbidden)
                    return
                }
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

## Test Plan
- JWT extraction: valid JWT produces correct TenantContext
- Missing JWT: returns 401
- Expired JWT: returns 401
- Cross-tenant: Tenant A request to Tenant B resource → 403
- Explicit authorization: shared resource → allowed
- Context propagation: TenantContext available in all downstream handlers
- Correlation ID: generated if missing, propagated if present