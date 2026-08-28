package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

var apiserverRoutes = []iam.RouteMetadata{
	{Method: http.MethodGet, Pattern: "/health", Public: true},
	{Method: http.MethodGet, Pattern: "/ready", Public: true},
	{Method: http.MethodGet, Pattern: "/openapi.json", Public: true},
	{Method: http.MethodPost, Pattern: "/api/v1/auth/login", Public: true},
	{Method: http.MethodPost, Pattern: "/api/v1/auth/refresh", Public: true},
	{Method: http.MethodPost, Pattern: "/api/v1/auth/logout", ResourceKind: "session", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/session/bootstrap", Public: true},
	{Method: http.MethodGet, Pattern: "/api/v1/users", ResourceKind: "user", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/users", ResourceKind: "user", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/users/{id}", ResourceKind: "user", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/users/{id}", ResourceKind: "user", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/users/{id}", ResourceKind: "user", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/users/{id}/reset-password", ResourceKind: "user", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/roles", ResourceKind: "role", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/roles/{id}", ResourceKind: "role", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/roles", ResourceKind: "role", Action: iam.ActionCreate},
	{Method: http.MethodDelete, Pattern: "/api/v1/roles/{id}", ResourceKind: "role", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/role-bindings", ResourceKind: "roleBinding", Action: iam.ActionCreate},
	{Method: http.MethodDelete, Pattern: "/api/v1/role-bindings/{user_id}/{scope}/{scope_id}", ResourceKind: "roleBinding", ResourceIDParam: "user_id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/role-bindings", ResourceKind: "roleBinding", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/check-permission", ResourceKind: "authorization", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/tenants", ResourceKind: "tenant", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/tenants", ResourceKind: "tenant", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/tenants/{id}", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/tenants/{id}", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/tenants/{id}", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/tenants/{id}/workspaces", ResourceKind: "workspace", ResourceIDParam: "id", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/tenants/{id}/workspaces", ResourceKind: "workspace", ResourceIDParam: "id", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/tenants/{id}/quota", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/tenants/{id}/quota", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/tenants/{id}/cluster-allocations", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/tenants/{id}/cluster-allocations/{cluster_id}", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/tenants/{id}/cluster-allocations/{cluster_id}", ResourceKind: "tenant", ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces", ResourceKind: "workspace", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/workspaces", ResourceKind: "workspace", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/namespaces", ResourceKind: "namespace", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/namespaces", ResourceKind: "namespace", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/quota", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/workspaces/{workspace_id}/quota", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/workspaces/{workspace_id}/bind-cluster", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/workspaces/{workspace_id}/clusters/{cluster_id}", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/clusters", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodPut, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/quota-remaining", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/workspaces/{workspace_id}/namespaces/{namespace_id}/members/{subject_id}", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/workspaces/{workspace_id}/users", ResourceKind: "workspace", ResourceIDParam: "workspace_id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/clusters", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/clusters", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/clusters/{id}", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodDelete, Pattern: "/api/v1/clusters/{id}", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters/{id}", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters/{id}/nodes", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters/{id}/plugins", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters/{id}/monitoring/summary", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/resources/clusters/{id}/monitoring/metrics", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPatch, Pattern: "/api/v1/resources/clusters/{id}/description", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/resources/clusters/{id}/kubeconfig:download", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/resources/clusters/{id}/agent-onboarding", ResourceKind: string(iam.ResourceCluster), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/dictionaries/cluster.status", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/overview", ResourceKind: string(iam.ResourceStorageOverview), Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/backends", ResourceKind: string(iam.ResourceStorageBackend), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/backends", ResourceKind: string(iam.ResourceStorageBackend), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/backends/{backendId}", ResourceKind: string(iam.ResourceStorageBackend), ResourceIDParam: "backendId", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/storage/backends/{backendId}", ResourceKind: string(iam.ResourceStorageBackend), ResourceIDParam: "backendId", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/storage/backends/{backendId}", ResourceKind: string(iam.ResourceStorageBackend), ResourceIDParam: "backendId", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/provider-schemas", ResourceKind: string(iam.ResourceStorageBackend), Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/offerings", ResourceKind: string(iam.ResourceWorkloadStorageOffering), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/offerings", ResourceKind: string(iam.ResourceWorkloadStorageOffering), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/offerings/{offeringId}", ResourceKind: string(iam.ResourceWorkloadStorageOffering), ResourceIDParam: "offeringId", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/storage/offerings/{offeringId}", ResourceKind: string(iam.ResourceWorkloadStorageOffering), ResourceIDParam: "offeringId", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/storage/offerings/{offeringId}", ResourceKind: string(iam.ResourceWorkloadStorageOffering), ResourceIDParam: "offeringId", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/driver-installations", ResourceKind: string(iam.ResourceStorageDriverInstallation), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/driver-installations/{installationId}/intents/install", ResourceKind: string(iam.ResourceStorageDriverInstallation), ResourceIDParam: "installationId", Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/driver-installations/{installationId}/intents/upgrade", ResourceKind: string(iam.ResourceStorageDriverInstallation), ResourceIDParam: "installationId", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/driver-installations/{installationId}/intents/uninstall", ResourceKind: string(iam.ResourceStorageDriverInstallation), ResourceIDParam: "installationId", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/targets/{targetId}/inventory", ResourceKind: string(iam.ResourceStorageInventory), ResourceIDParam: "targetId", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/targets/{targetId}/metrics", ResourceKind: string(iam.ResourceStorageInventory), ResourceIDParam: "targetId", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/offerings/{offeringId}/bindings", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "offeringId", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/offerings/{offeringId}/bindings", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "offeringId", Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/bindings/{bindingId}", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "bindingId", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/storage/bindings/{bindingId}", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "bindingId", Action: iam.ActionUpdate},
	{Method: http.MethodDelete, Pattern: "/api/v1/storage/bindings/{bindingId}", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "bindingId", Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/offerings/{offeringId}/bindings/intents/import", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "offeringId", Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/bindings/{bindingId}/intents/reconcile", ResourceKind: string(iam.ResourceStorageClassBinding), ResourceIDParam: "bindingId", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/retained-volumes/{volumeId}/intents/release", ResourceKind: string(iam.ResourceRetainedVolume), ResourceIDParam: "volumeId", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/retained-volumes/{volumeId}/intents/sanitize", ResourceKind: string(iam.ResourceRetainedVolume), ResourceIDParam: "volumeId", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/storage/alert-rules", ResourceKind: string(iam.ResourceStorageAlertRule), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/storage/alert-rules", ResourceKind: string(iam.ResourceStorageAlertRule), Action: iam.ActionCreate},
	{Method: http.MethodPost, Pattern: "/api/v1/runtime-intents", ResourceKind: string(iam.ResourceCluster), IntentKindAction: true},
	{Method: http.MethodPost, Pattern: "/api/v1/runtime-intent-batches", ResourceKind: string(iam.ResourceCluster), Action: iam.ActionDelete},
	{Method: http.MethodPost, Pattern: "/api/v1/secrets:register", ResourceKind: string(iam.ResourceSecret), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/extensions", ResourceKind: "extension", Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/extensions", ResourceKind: "extension", Action: iam.ActionCreate},
	{Method: http.MethodDelete, Pattern: "/api/v1/extensions/{id}", ResourceKind: "extension", ResourceIDParam: "id", Action: iam.ActionDelete},
	{Method: http.MethodGet, Pattern: "/api/v1/navigation/menus", ResourceKind: string(iam.ResourceNavigation), Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/schema/page/{id}", ResourceKind: string(iam.ResourceSchema), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/ui/pages/{id}/publish", ResourceKind: string(iam.ResourceSchema), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/ui/pages/{id}/rollback", ResourceKind: string(iam.ResourceSchema), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/gslb/services", ResourceKind: string(iam.ResourceGSLB), Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/gslb/services/{id}", ResourceKind: string(iam.ResourceGSLB), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/gslb/services/{id}/drills", ResourceKind: string(iam.ResourceGSLB), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/gslb/services/{id}/intents", ResourceKind: string(iam.ResourceGSLB), ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/gslb/switch-requests/{id}/approve", ResourceKind: string(iam.ResourceGSLB), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/gslb/switch-requests/{id}/reject", ResourceKind: string(iam.ResourceGSLB), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/operations", ResourceKind: string(iam.ResourceOperation), Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/operations/{id}", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/operations/{id}/actions/approve", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/operations/{id}/actions/reject", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodPost, Pattern: "/api/v1/operations/{id}/actions/cancel", ResourceKind: string(iam.ResourceOperation), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/dr/groups", ResourceKind: string(iam.ResourceDR), Action: iam.ActionList},
	{Method: http.MethodPost, Pattern: "/api/v1/dr/groups", ResourceKind: string(iam.ResourceDR), Action: iam.ActionCreate},
	{Method: http.MethodGet, Pattern: "/api/v1/dr/groups/{id}", ResourceKind: string(iam.ResourceDR), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/dr/groups/{id}/members", ResourceKind: string(iam.ResourceDR), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/dr/groups/{id}/runs", ResourceKind: string(iam.ResourceDR), ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/dr/groups/{id}/switch", ResourceKind: string(iam.ResourceDR), ResourceIDParam: "id", Action: iam.ActionExecute},
	{Method: http.MethodPost, Pattern: "/api/v1/dr/runs/{id}/confirm-data-layer", ResourceKind: string(iam.ResourceDR), ResourceIDParam: "id", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/proxy/{cluster_id}/{path...}", ResourceKind: "proxy", ResourceIDParam: "cluster_id", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/proxy/{cluster_id}/{path...}", ResourceKind: "proxy", ResourceIDParam: "cluster_id", Action: iam.ActionExecute},
	{Method: http.MethodPut, Pattern: "/api/v1/proxy/{cluster_id}/{path...}", ResourceKind: "proxy", ResourceIDParam: "cluster_id", Action: iam.ActionExecute},
	{Method: http.MethodPatch, Pattern: "/api/v1/proxy/{cluster_id}/{path...}", ResourceKind: "proxy", ResourceIDParam: "cluster_id", Action: iam.ActionExecute},
	{Method: http.MethodDelete, Pattern: "/api/v1/proxy/{cluster_id}/{path...}", ResourceKind: "proxy", ResourceIDParam: "cluster_id", Action: iam.ActionExecute},
	{Method: http.MethodGet, Pattern: "/api/v1/agents", ResourceKind: "agent", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/agents/{cluster_id}", ResourceKind: "agent", ResourceIDParam: "cluster_id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/audit-logs", ResourceKind: "auditLog", Action: iam.ActionList},
	{Method: http.MethodGet, Pattern: "/api/v1/audit-logs/{id}", ResourceKind: "auditLog", ResourceIDParam: "id", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/settings", ResourceKind: "settings", Action: iam.ActionRead},
	{Method: http.MethodPut, Pattern: "/api/v1/settings", ResourceKind: "settings", Action: iam.ActionUpdate},
	{Method: http.MethodGet, Pattern: "/api/v1/capabilities", ResourceKind: string(iam.ResourceNavigation), Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/capabilities/{name}", ResourceKind: string(iam.ResourceNavigation), Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/market", ResourceKind: "market", Action: iam.ActionRead},
	{Method: http.MethodGet, Pattern: "/api/v1/market/{path...}", ResourceKind: "market", Action: iam.ActionRead},
	{Method: http.MethodPost, Pattern: "/api/v1/market/{path...}", ResourceKind: "market", Action: iam.ActionExecute},
	{Method: http.MethodPut, Pattern: "/api/v1/market/{path...}", ResourceKind: "market", Action: iam.ActionExecute},
	{Method: http.MethodPatch, Pattern: "/api/v1/market/{path...}", ResourceKind: "market", Action: iam.ActionExecute},
	{Method: http.MethodDelete, Pattern: "/api/v1/market/{path...}", ResourceKind: "market", Action: iam.ActionExecute},
}

type AuthzMW struct {
	evaluator *iam.Evaluator
}

func NewAuthz() *AuthzMW { return &AuthzMW{evaluator: iam.NewEvaluator()} }

func (m *AuthzMW) Name() string { return "authorization" }

func (m *AuthzMW) Handle(ctx *Context, next func()) {
	matched, ok := iam.MatchRoute(apiserverRoutes, ctx.Request.Method, ctx.Request.URL.Path)
	if !ok {
		ctx.Abort(http.StatusForbidden, []byte(`{"code":40300,"message":"route is not authorized"}`))
		return
	}
	if matched.Metadata.Public {
		next()
		return
	}
	trusted, ok := iam.TrustedContextFrom(ctx.Request.Context())
	if !ok {
		if isStorageAuthorizationResource(matched.Metadata.ResourceKind) {
			abortStorageAuthorization(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.")
			return
		}
		ctx.Abort(http.StatusUnauthorized, []byte(`{"code":40100,"message":"trusted context required"}`))
		return
	}
	if matched.Metadata.IntentKindAction {
		action, ok := runtimeIntentAction(ctx.Request)
		if !ok {
			ctx.Abort(http.StatusBadRequest, []byte(`{"code":40000,"message":"invalid cluster intent kind"}`))
			return
		}
		matched.Metadata.Action = action
	}
	if decision := m.evaluator.EvaluateRoute(trusted, matched); !decision.Allowed {
		if isStorageAuthorizationResource(matched.Metadata.ResourceKind) {
			abortStorageAuthorization(ctx, http.StatusForbidden, "FORBIDDEN", "The requested storage scope is not authorized.")
			return
		}
		ctx.Abort(http.StatusForbidden, []byte(`{"code":40300,"message":"forbidden"}`))
		return
	}
	next()
}

func isStorageAuthorizationResource(resource string) bool {
	switch iam.AuthorizationResource(resource) {
	case iam.ResourceStorageOverview, iam.ResourceStorageBackend, iam.ResourceWorkloadStorageOffering,
		iam.ResourceStorageDriverInstallation, iam.ResourceStorageInventory, iam.ResourceStorageClassBinding, iam.ResourceRetainedVolume,
		iam.ResourceStorageAlertRule:
		return true
	default:
		return false
	}
}

func abortStorageAuthorization(ctx *Context, status int, code, detail string) {
	correlationID := strings.ToLower(ctx.Request.Header.Get("X-Correlation-ID"))
	if parsed, err := uuid.Parse(correlationID); err != nil || parsed.String() != correlationID {
		correlationID = uuid.NewString()
	}
	body, _ := json.Marshal(map[string]any{
		"type":          "https://hnb.cloud/problems/" + strings.ToLower(code),
		"title":         http.StatusText(status),
		"status":        status,
		"detail":        detail,
		"code":          code,
		"correlationId": correlationID,
		"traceId":       strings.ReplaceAll(correlationID, "-", ""),
	})
	ctx.Aborted, ctx.abortCode, ctx.abortBody = true, status, body
	ctx.Response.Header().Set("Content-Type", "application/problem+json")
	ctx.Response.Header().Set("X-Correlation-ID", correlationID)
	ctx.Response.Header().Set("X-Trace-Id", strings.ReplaceAll(correlationID, "-", ""))
	ctx.Response.WriteHeader(status)
	_, _ = ctx.Response.Write(body)
}

func runtimeIntentAction(r *http.Request) (iam.AuthorizationAction, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return "", false
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	var envelope struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return "", false
	}
	return iam.ClusterActionForIntentKind(envelope.Kind)
}
