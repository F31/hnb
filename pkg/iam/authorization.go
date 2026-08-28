package iam

import (
	"errors"
	"net/http"
	"strings"
)

const (
	MaxScopedPermissions = 64
	MaxPolicyVersionLen  = 128
)

type AuthorizationAction string

type AuthorizationResource string

const (
	ResourceNavigation                AuthorizationResource = "navigation"
	ResourceSchema                    AuthorizationResource = "schema"
	ResourceGSLB                      AuthorizationResource = "gslb"
	ResourceDR                        AuthorizationResource = "dr"
	ResourceCluster                   AuthorizationResource = "cluster"
	ResourceClusterMetadata           AuthorizationResource = "cluster-metadata"
	ResourceRuntimeTarget             AuthorizationResource = "runtimeTarget"
	ResourceOperation                 AuthorizationResource = "operation"
	ResourceProvider                  AuthorizationResource = "provider"
	ResourceIntent                    AuthorizationResource = "intent"
	ResourceStorageOverview           AuthorizationResource = "storageOverview"
	ResourceStorageBackend            AuthorizationResource = "storageBackend"
	ResourceWorkloadStorageOffering   AuthorizationResource = "workloadStorageOffering"
	ResourceStorageDriverInstallation AuthorizationResource = "storageDriverInstallation"
	ResourceStorageInventory          AuthorizationResource = "storageInventory"
	ResourceStorageClassBinding       AuthorizationResource = "storageClassBinding"
	ResourceRetainedVolume            AuthorizationResource = "retainedVolume"
	ResourceStorageAlertRule          AuthorizationResource = "storageAlertRule"
	ResourceSecret                    AuthorizationResource = "secret"
)

const (
	ActionRead         AuthorizationAction = "read"
	ActionList         AuthorizationAction = "list"
	ActionCreate       AuthorizationAction = "create"
	ActionUpdate       AuthorizationAction = "update"
	ActionDelete       AuthorizationAction = "delete"
	ActionExecute      AuthorizationAction = "execute"
	ActionApprove      AuthorizationAction = "approve"
	ActionReject       AuthorizationAction = "reject"
	ActionCancel       AuthorizationAction = "cancel"
	ActionSwitchTenant AuthorizationAction = "switchTenant"
)

const (
	PermissionClusterList                        = "cluster:list"
	PermissionClusterRead                        = "cluster:read"
	PermissionClusterCreate                      = "cluster:create"
	PermissionClusterUpdate                      = "cluster:update"
	PermissionClusterDelete                      = "cluster:delete"
	PermissionStorageOverviewRead                = "storageOverview:read"
	PermissionStorageBackendList                 = "storageBackend:list"
	PermissionStorageBackendCreate               = "storageBackend:create"
	PermissionStorageBackendRead                 = "storageBackend:read"
	PermissionStorageBackendUpdate               = "storageBackend:update"
	PermissionStorageBackendDelete               = "storageBackend:delete"
	PermissionWorkloadStorageOfferingList        = "workloadStorageOffering:list"
	PermissionWorkloadStorageOfferingCreate      = "workloadStorageOffering:create"
	PermissionWorkloadStorageOfferingRead        = "workloadStorageOffering:read"
	PermissionWorkloadStorageOfferingUpdate      = "workloadStorageOffering:update"
	PermissionWorkloadStorageOfferingDelete      = "workloadStorageOffering:delete"
	PermissionStorageDriverInstallationList      = "storageDriverInstallation:list"
	PermissionStorageDriverInstallationInstall   = "storageDriverInstallation:install"
	PermissionStorageDriverInstallationUpgrade   = "storageDriverInstallation:upgrade"
	PermissionStorageDriverInstallationUninstall = "storageDriverInstallation:uninstall"
	PermissionStorageInventoryRead               = "storageInventory:read"
	PermissionStorageClassBindingList            = "storageClassBinding:list"
	PermissionStorageClassBindingCreate          = "storageClassBinding:create"
	PermissionStorageClassBindingRead            = "storageClassBinding:read"
	PermissionStorageClassBindingUpdate          = "storageClassBinding:update"
	PermissionStorageClassBindingDelete          = "storageClassBinding:delete"
	PermissionStorageClassBindingImport          = "storageClassBinding:import"
	PermissionStorageClassBindingReconcile       = "storageClassBinding:reconcile"
	PermissionRetainedVolumeRelease              = "retainedVolume:release"
	PermissionRetainedVolumeSanitize             = "retainedVolume:sanitize"
	PermissionStorageAlertRuleList               = "storageAlertRule:list"
	PermissionStorageAlertRuleCreate             = "storageAlertRule:create"
)

func StorageBindingActionForIntentKind(kind string) (AuthorizationAction, bool) {
	switch kind {
	case "ImportStorageClassBinding":
		return ActionCreate, true
	case "ReconcileStorageClassBinding":
		return ActionUpdate, true
	default:
		return "", false
	}
}

func RetainedVolumeActionForIntentKind(kind string) (AuthorizationAction, bool) {
	switch kind {
	case "ReleaseRetainedVolume", "SanitizeRetainedVolume":
		return ActionExecute, true
	default:
		return "", false
	}
}

func StorageDriverActionForIntentKind(kind string) (AuthorizationAction, bool) {
	switch kind {
	case "InstallStorageDriver":
		return ActionCreate, true
	case "UpgradeStorageDriver":
		return ActionUpdate, true
	case "UninstallStorageDriver":
		return ActionDelete, true
	default:
		return "", false
	}
}

func ClusterActionForIntentKind(kind string) (AuthorizationAction, bool) {
	switch kind {
	case "CreateKubernetesTarget", "ImportRuntimeTarget":
		return ActionCreate, true
	case "UpgradeRuntimeTarget":
		return ActionUpdate, true
	case "DeleteRuntimeTarget":
		return ActionDelete, true
	default:
		return "", false
	}
}

func ClusterActionForOperationAction(action, intentKind string) (AuthorizationAction, bool) {
	switch action {
	case "approve", "reject", "cancel":
		return ClusterActionForIntentKind(intentKind)
	default:
		return "", false
	}
}

type ScopedPermission struct {
	ResourceKind  string              `json:"resourceKind"`
	ResourceID    string              `json:"resourceId,omitempty"`
	Action        AuthorizationAction `json:"action"`
	TenantID      string              `json:"tenantId"`
	ProjectID     string              `json:"projectId,omitempty"`
	EnvironmentID string              `json:"environmentId,omitempty"`
	NamespaceID   string              `json:"namespaceId,omitempty"`
}

type AuthorizationRequest struct {
	SubjectID     string
	TenantID      string
	ResourceKind  string
	ResourceID    string
	Action        AuthorizationAction
	ProjectID     string
	EnvironmentID string
	NamespaceID   string
}

type AuthorizationDecision struct {
	Allowed       bool                `json:"allowed"`
	ReasonCode    string              `json:"reasonCode"`
	PolicyVersion string              `json:"policyVersion"`
	ResourceKind  string              `json:"resourceKind"`
	ResourceID    string              `json:"resourceId,omitempty"`
	Action        AuthorizationAction `json:"action"`
}

type Evaluator struct{}

func NewEvaluator() *Evaluator { return &Evaluator{} }

func (e *Evaluator) Evaluate(trusted TrustedContext, request AuthorizationRequest) AuthorizationDecision {
	decision := AuthorizationDecision{
		ReasonCode: "permission_denied", PolicyVersion: trusted.PolicyVersion,
		ResourceKind: request.ResourceKind, ResourceID: request.ResourceID, Action: request.Action,
	}
	if request.SubjectID == "" || request.SubjectID != trusted.SubjectID || request.TenantID == "" || request.TenantID != trusted.TenantID {
		decision.ReasonCode = "identity_scope_mismatch"
		return decision
	}
	if err := ValidatePermissionSnapshot(trusted.PolicyVersion, trusted.ScopedPermissions, trusted.TenantID); err != nil {
		decision.ReasonCode = "invalid_policy_snapshot"
		return decision
	}
	if !boundedOptional(request.ResourceKind, 128, true) || request.ResourceKind == "*" || !validAction(request.Action) ||
		!boundedOptional(request.ResourceID, 256, false) || !validRequestScope(request.ProjectID, request.EnvironmentID, request.NamespaceID) {
		decision.ReasonCode = "invalid_authorization_request"
		return decision
	}
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID != request.TenantID || permission.Action != request.Action ||
			(permission.ResourceKind != "*" && permission.ResourceKind != request.ResourceKind) ||
			(permission.ResourceID != "" && permission.ResourceID != request.ResourceID) ||
			(permission.ProjectID != "" && permission.ProjectID != request.ProjectID) ||
			(permission.EnvironmentID != "" && permission.EnvironmentID != request.EnvironmentID) ||
			(permission.NamespaceID != "" && permission.NamespaceID != request.NamespaceID) {
			continue
		}
		decision.Allowed = true
		decision.ReasonCode = "permission_granted"
		return decision
	}
	return decision
}

func ValidatePermissionSnapshot(policyVersion string, permissions []ScopedPermission, tenantID string) error {
	if !boundedClaim(policyVersion, MaxPolicyVersionLen) || !boundedClaim(tenantID, 128) || tenantID == "*" || len(permissions) > MaxScopedPermissions {
		return errors.New("invalid policy snapshot boundary")
	}
	seen := make(map[ScopedPermission]struct{}, len(permissions))
	for _, permission := range permissions {
		if permission.TenantID != tenantID || permission.TenantID == "*" ||
			!boundedOptional(permission.ResourceKind, 128, true) || !boundedOptional(permission.ResourceID, 256, false) ||
			!validAction(permission.Action) || !validScope(permission.ProjectID, permission.EnvironmentID, permission.NamespaceID) {
			return errors.New("invalid scoped permission")
		}
		if _, duplicate := seen[permission]; duplicate {
			return errors.New("duplicate scoped permission")
		}
		seen[permission] = struct{}{}
	}
	return nil
}

func validScope(projectID, environmentID, namespaceID string) bool {
	return validRequestScope(projectID, environmentID, namespaceID) &&
		(environmentID == "" || projectID != "") && (namespaceID == "" || environmentID != "")
}

func validRequestScope(projectID, environmentID, namespaceID string) bool {
	return boundedOptional(projectID, 128, false) && boundedOptional(environmentID, 128, false) && boundedOptional(namespaceID, 253, false)
}

func boundedOptional(value string, maximum int, required bool) bool {
	if value == "" {
		return !required
	}
	return len(value) <= maximum && !strings.ContainsAny(value, "\x00\r\n")
}

func validAction(action AuthorizationAction) bool {
	switch action {
	case ActionRead, ActionList, ActionCreate, ActionUpdate, ActionDelete, ActionExecute, ActionApprove, ActionReject, ActionCancel, ActionSwitchTenant:
		return true
	default:
		return false
	}
}

type RouteMetadata struct {
	Method           string
	Pattern          string
	Public           bool
	ResourceKind     string
	Action           AuthorizationAction
	ResourceIDParam  string
	ProjectIDParam   string
	EnvironmentParam string
	NamespaceParam   string
	IntentKindAction bool
}

type MatchedRoute struct {
	Metadata RouteMetadata
	Params   map[string]string
}

func MatchRoute(routes []RouteMetadata, method, path string) (MatchedRoute, bool) {
	for _, route := range routes {
		if route.Method != method {
			continue
		}
		if params, ok := matchRoutePattern(route.Pattern, path); ok {
			return MatchedRoute{Metadata: route, Params: params}, true
		}
	}
	return MatchedRoute{}, false
}

func (e *Evaluator) EvaluateRoute(trusted TrustedContext, matched MatchedRoute) AuthorizationDecision {
	metadata, params := matched.Metadata, matched.Params
	return e.Evaluate(trusted, AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID,
		ResourceKind: metadata.ResourceKind, ResourceID: params[metadata.ResourceIDParam], Action: metadata.Action,
		ProjectID: params[metadata.ProjectIDParam], EnvironmentID: params[metadata.EnvironmentParam], NamespaceID: params[metadata.NamespaceParam],
	})
}

func AuthorizeRoutes(evaluator *Evaluator, routes []RouteMetadata, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched, ok := MatchRoute(routes, r.Method, r.URL.Path)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if matched.Metadata.Public {
			next.ServeHTTP(w, r)
			return
		}
		trusted, ok := TrustedContextFrom(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !evaluator.EvaluateRoute(trusted, matched).Allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchRoutePattern(pattern, path string) (map[string]string, bool) {
	patternParts := splitRoutePath(pattern)
	pathParts := splitRoutePath(path)
	params := make(map[string]string)
	for index, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "...}") {
			if index >= len(pathParts) {
				return nil, false
			}
			params[strings.TrimSuffix(strings.TrimPrefix(part, "{"), "...}")] = strings.Join(pathParts[index:], "/")
			return params, true
		}
		if index >= len(pathParts) {
			return nil, false
		}
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params[strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")] = pathParts[index]
		} else if part != pathParts[index] {
			return nil, false
		}
	}
	return params, len(patternParts) == len(pathParts)
}

func splitRoutePath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
