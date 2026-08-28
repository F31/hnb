package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

func TestAuthorizationAllowsExplicitPublicPathWithoutTrustedContext(t *testing.T) {
	middleware := NewAuthz()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	handled := false
	middleware.Handle(ctx, func() { handled = true })
	if !handled || ctx.Aborted {
		t.Fatalf("login bypass handled = %v, aborted = %v", handled, ctx.Aborted)
	}
}

func TestAuthorizationRequiresTrustedContextForProtectedPath(t *testing.T) {
	middleware := NewAuthz()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	middleware.Handle(ctx, func() { t.Fatal("protected handler executed") })
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestAuthorizationDeniesUnknownRouteAndMissingPermission(t *testing.T) {
	for name, request := range map[string]*http.Request{
		"unknown": httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil),
		"missing permission": httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil).WithContext(iam.WithTrustedContext(
			httptest.NewRequest(http.MethodGet, "/", nil).Context(),
			iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1"},
		)),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx := &Context{Request: request, Response: recorder}
			NewAuthz().Handle(ctx, func() { t.Fatal("denied handler executed") })
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d", recorder.Code)
			}
		})
	}
}

func TestAuthorizationUsesDynamicResourceID(t *testing.T) {
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/cluster-a", nil)
	trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{ResourceKind: "cluster", ResourceID: "cluster-a", Action: iam.ActionDelete, TenantID: "tenant"}}}
	request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	handled := false
	NewAuthz().Handle(ctx, func() { handled = true })
	if !handled || ctx.Aborted {
		t.Fatalf("handled = %v, aborted = %v", handled, ctx.Aborted)
	}
}

func TestAuthorizationAllowsNavigationRead(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/navigation/menus", nil)
	trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{ResourceKind: "navigation", Action: iam.ActionRead, TenantID: "tenant"}}}
	request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	handled := false
	NewAuthz().Handle(ctx, func() { handled = true })
	if !handled || ctx.Aborted {
		t.Fatalf("handled = %v, aborted = %v", handled, ctx.Aborted)
	}
}

func TestClusterRoutePermissionMatrix(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		action iam.AuthorizationAction
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/resources/clusters", action: iam.ActionList},
		{name: "detail", method: http.MethodGet, path: "/api/v1/resources/clusters/cluster-a", action: iam.ActionRead},
		{name: "nodes", method: http.MethodGet, path: "/api/v1/resources/clusters/cluster-a/nodes", action: iam.ActionRead},
		{name: "kubeconfig-download", method: http.MethodPost, path: "/api/v1/resources/clusters/cluster-a/kubeconfig:download", action: iam.ActionRead},
		{name: "agent-onboarding", method: http.MethodPost, path: "/api/v1/resources/clusters/cluster-a/agent-onboarding", action: iam.ActionRead},
		{name: "create", method: http.MethodPost, path: "/api/v1/runtime-intents", body: `{"kind":"CreateKubernetesTarget"}`, action: iam.ActionCreate},
		{name: "import", method: http.MethodPost, path: "/api/v1/runtime-intents", body: `{"kind":"ImportRuntimeTarget"}`, action: iam.ActionCreate},
		{name: "upgrade", method: http.MethodPost, path: "/api/v1/runtime-intents", body: `{"kind":"UpgradeRuntimeTarget"}`, action: iam.ActionUpdate},
		{name: "delete-unmanage", method: http.MethodPost, path: "/api/v1/runtime-intents", body: `{"kind":"DeleteRuntimeTarget"}`, action: iam.ActionDelete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name    string
				action  iam.AuthorizationAction
				allowed bool
			}{
				{name: "allow", action: test.action, allowed: true},
				{name: "deny-other-cluster-action", action: differentClusterAction(test.action)},
				{name: "deny-execute", action: iam.ActionExecute},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
					trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{ResourceKind: "cluster", Action: scenario.action, TenantID: "tenant"}}}
					request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
					recorder := httptest.NewRecorder()
					ctx := &Context{Request: request, Response: recorder}
					handled := false
					NewAuthz().Handle(ctx, func() { handled = true })
					if handled != scenario.allowed {
						t.Fatalf("handled = %v, want %v (status %d)", handled, scenario.allowed, recorder.Code)
					}
					if test.body != "" && handled {
						body, err := io.ReadAll(request.Body)
						if err != nil || string(body) != test.body {
							t.Fatalf("restored body = %q, err = %v", body, err)
						}
					}
				})
			}
		})
	}
}

func TestRuntimeIntentAuthorizationRejectsInvalidKindBeforeHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime-intents", strings.NewReader(`{"kind":"InstallRelease"}`))
	trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{ResourceKind: "cluster", Action: iam.ActionCreate, TenantID: "tenant"}}}
	request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
	recorder := httptest.NewRecorder()
	ctx := &Context{Request: request, Response: recorder}
	NewAuthz().Handle(ctx, func() { t.Fatal("invalid intent reached handler") })
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestStorageRoutePermissionMatrix(t *testing.T) {
	tests := []struct {
		method, path, resource, resourceID string
		action                             iam.AuthorizationAction
	}{
		{http.MethodGet, "/api/v1/storage/overview", "storageOverview", "", iam.ActionRead},
		{http.MethodGet, "/api/v1/storage/backends", "storageBackend", "", iam.ActionList},
		{http.MethodGet, "/api/v1/storage/provider-schemas", "storageBackend", "", iam.ActionList},
		{http.MethodPost, "/api/v1/storage/backends", "storageBackend", "", iam.ActionCreate},
		{http.MethodPut, "/api/v1/storage/backends/backend-a", "storageBackend", "backend-a", iam.ActionUpdate},
		{http.MethodDelete, "/api/v1/storage/backends/backend-a", "storageBackend", "backend-a", iam.ActionDelete},
		{http.MethodGet, "/api/v1/storage/offerings", "workloadStorageOffering", "", iam.ActionList},
		{http.MethodPost, "/api/v1/storage/offerings", "workloadStorageOffering", "", iam.ActionCreate},
		{http.MethodGet, "/api/v1/storage/driver-installations", "storageDriverInstallation", "", iam.ActionList},
		{http.MethodPost, "/api/v1/storage/driver-installations/driver-a/intents/install", "storageDriverInstallation", "driver-a", iam.ActionCreate},
		{http.MethodPost, "/api/v1/storage/driver-installations/driver-a/intents/upgrade", "storageDriverInstallation", "driver-a", iam.ActionUpdate},
		{http.MethodPost, "/api/v1/storage/driver-installations/driver-a/intents/uninstall", "storageDriverInstallation", "driver-a", iam.ActionDelete},
		{http.MethodGet, "/api/v1/storage/targets/target-a/inventory", "storageInventory", "target-a", iam.ActionRead},
		{http.MethodGet, "/api/v1/storage/targets/target-a/metrics", "storageInventory", "target-a", iam.ActionRead},
		{http.MethodGet, "/api/v1/storage/offerings/offering-a/bindings", "storageClassBinding", "offering-a", iam.ActionList},
		{http.MethodPost, "/api/v1/storage/offerings/offering-a/bindings", "storageClassBinding", "offering-a", iam.ActionCreate},
		{http.MethodPut, "/api/v1/storage/bindings/binding-a", "storageClassBinding", "binding-a", iam.ActionUpdate},
		{http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/release", "retainedVolume", "volume-a", iam.ActionExecute},
		{http.MethodPost, "/api/v1/storage/retained-volumes/volume-a/intents/sanitize", "retainedVolume", "volume-a", iam.ActionExecute},
		{http.MethodGet, "/api/v1/storage/alert-rules", "storageAlertRule", "", iam.ActionList},
		{http.MethodPost, "/api/v1/storage/alert-rules", "storageAlertRule", "", iam.ActionCreate},
	}
	for _, test := range tests {
		t.Run(test.resource, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{
				ResourceKind: test.resource, ResourceID: test.resourceID, Action: test.action, TenantID: "tenant",
			}}}
			request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
			recorder := httptest.NewRecorder()
			handled := false
			NewAuthz().Handle(&Context{Request: request, Response: recorder}, func() { handled = true })
			if !handled {
				t.Fatalf("route denied: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestStorageAuthorizationDenialUsesStableProblem(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storage/targets/target-a/inventory", nil)
	request.Header.Set("X-Correlation-ID", "95ee3540-b162-4c05-b65c-7f909b29cb11")
	trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []iam.ScopedPermission{{
		ResourceKind: "storageInventory", ResourceID: "other-target", Action: iam.ActionRead, TenantID: "tenant",
	}}}
	request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
	recorder := httptest.NewRecorder()
	NewAuthz().Handle(&Context{Request: request, Response: recorder}, func() { t.Fatal("denied storage handler executed") })
	if recorder.Code != http.StatusForbidden || recorder.Header().Get("Content-Type") != "application/problem+json" ||
		!strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) ||
		!strings.Contains(recorder.Body.String(), `"correlationId":"95ee3540-b162-4c05-b65c-7f909b29cb11"`) {
		t.Fatalf("unexpected denial: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStorageAuthorizationRequiresAuthenticationForEveryReadRoute(t *testing.T) {
	paths := []string{
		"/api/v1/storage/overview",
		"/api/v1/storage/backends",
		"/api/v1/storage/provider-schemas",
		"/api/v1/storage/offerings",
		"/api/v1/storage/driver-installations",
		"/api/v1/storage/targets/target-a/inventory",
		"/api/v1/storage/targets/target-a/metrics",
		"/api/v1/storage/offerings/offering-a/bindings",
		"/api/v1/storage/alert-rules",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			NewAuthz().Handle(&Context{Request: request, Response: recorder}, func() { t.Fatal("unauthenticated storage handler executed") })
			if recorder.Code != http.StatusUnauthorized || recorder.Header().Get("Content-Type") != "application/problem+json" ||
				!strings.Contains(recorder.Body.String(), `"code":"UNAUTHORIZED"`) {
				t.Fatalf("unexpected denial: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func differentClusterAction(action iam.AuthorizationAction) iam.AuthorizationAction {
	if action == iam.ActionRead {
		return iam.ActionList
	}
	return iam.ActionRead
}

func TestEveryProtectedRouteDeniesBeforeHandlerWithoutPermission(t *testing.T) {
	trusted := iam.TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1"}
	for _, route := range apiserverRoutes {
		if route.Public {
			continue
		}
		path := strings.NewReplacer(
			"{workspace_id}", "workspace-a", "{project_id}", "project-a", "{cluster_id}", "cluster-a",
			"{user_id}", "user-a", "{scope}", "project", "{scope_id}", "scope-a", "{id}", "resource-a", "{path...}", "api/v1/pods",
		).Replace(route.Pattern)
		var body io.Reader
		if route.IntentKindAction {
			body = strings.NewReader(`{"kind":"CreateKubernetesTarget"}`)
		}
		request := httptest.NewRequest(route.Method, path, body)
		request = request.WithContext(iam.WithTrustedContext(request.Context(), trusted))
		recorder := httptest.NewRecorder()
		ctx := &Context{Request: request, Response: recorder}
		NewAuthz().Handle(ctx, func() { t.Errorf("%s %s handler executed", route.Method, path) })
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d", route.Method, path, recorder.Code)
		}
	}
}
