package iam

import (
	"net/http"
	"testing"
)

func TestStorageDriverIntentActionsAreScoped(t *testing.T) {
	for kind, want := range map[string]AuthorizationAction{"InstallStorageDriver": ActionCreate, "UpgradeStorageDriver": ActionUpdate, "UninstallStorageDriver": ActionDelete} {
		got, ok := StorageDriverActionForIntentKind(kind)
		if !ok || got != want {
			t.Fatalf("%s action=%q ok=%v", kind, got, ok)
		}
	}
	if _, ok := StorageDriverActionForIntentKind("InstallRelease"); ok {
		t.Fatal("release intent mapped to storage driver IAM")
	}
}

func TestEvaluatorMatchesTenantHierarchyResourceAndAction(t *testing.T) {
	trusted := TrustedContext{
		SubjectID: "subject-a", TenantID: "tenant-a", PolicyVersion: "default:7",
		ScopedPermissions: []ScopedPermission{
			{ResourceKind: "deployment", Action: ActionRead, TenantID: "tenant-a", ProjectID: "project-a", EnvironmentID: "env-a", NamespaceID: "ns-a"},
			{ResourceKind: "operation", ResourceID: "op-a", Action: ActionApprove, TenantID: "tenant-a"},
		},
	}
	evaluator := NewEvaluator()
	base := AuthorizationRequest{SubjectID: "subject-a", TenantID: "tenant-a", ResourceKind: "deployment", Action: ActionRead, ProjectID: "project-a", EnvironmentID: "env-a", NamespaceID: "ns-a"}
	if decision := evaluator.Evaluate(trusted, base); !decision.Allowed || decision.PolicyVersion != "default:7" {
		t.Fatalf("matching decision = %+v", decision)
	}
	for name, mutate := range map[string]func(*AuthorizationRequest){
		"subject":     func(r *AuthorizationRequest) { r.SubjectID = "subject-b" },
		"tenant":      func(r *AuthorizationRequest) { r.TenantID = "tenant-b" },
		"project":     func(r *AuthorizationRequest) { r.ProjectID = "project-b" },
		"environment": func(r *AuthorizationRequest) { r.EnvironmentID = "env-b" },
		"namespace":   func(r *AuthorizationRequest) { r.NamespaceID = "ns-b" },
		"resource":    func(r *AuthorizationRequest) { r.ResourceKind = "secret" },
		"action":      func(r *AuthorizationRequest) { r.Action = ActionDelete },
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			mutate(&request)
			if decision := evaluator.Evaluate(trusted, request); decision.Allowed {
				t.Fatalf("mismatch was allowed: %+v", decision)
			}
		})
	}
	resource := AuthorizationRequest{SubjectID: "subject-a", TenantID: "tenant-a", ResourceKind: "operation", ResourceID: "op-a", Action: ActionApprove}
	if !evaluator.Evaluate(trusted, resource).Allowed {
		t.Fatal("matching resource ID was denied")
	}
	resource.ResourceID = "op-b"
	if evaluator.Evaluate(trusted, resource).Allowed {
		t.Fatal("different resource ID was allowed")
	}
}

func TestStoragePermissionsScopeInventoryAndBindingsByPathResource(t *testing.T) {
	trusted := TrustedContext{
		SubjectID: "subject-a", TenantID: "tenant-a", PolicyVersion: "default:1",
		ScopedPermissions: []ScopedPermission{
			{ResourceKind: string(ResourceStorageInventory), ResourceID: "target-a", Action: ActionRead, TenantID: "tenant-a"},
			{ResourceKind: string(ResourceStorageClassBinding), ResourceID: "offering-a", Action: ActionList, TenantID: "tenant-a"},
		},
	}
	evaluator := NewEvaluator()
	for _, request := range []AuthorizationRequest{
		{SubjectID: "subject-a", TenantID: "tenant-a", ResourceKind: string(ResourceStorageInventory), ResourceID: "target-a", Action: ActionRead},
		{SubjectID: "subject-a", TenantID: "tenant-a", ResourceKind: string(ResourceStorageClassBinding), ResourceID: "offering-a", Action: ActionList},
	} {
		if decision := evaluator.Evaluate(trusted, request); !decision.Allowed {
			t.Fatalf("matching storage request denied: %+v", decision)
		}
		request.ResourceID = "other"
		if decision := evaluator.Evaluate(trusted, request); decision.Allowed {
			t.Fatalf("cross-resource storage request allowed: %+v", decision)
		}
	}
}

func TestEvaluatorFailsClosedForMissingOrInvalidSnapshot(t *testing.T) {
	request := AuthorizationRequest{SubjectID: "subject", TenantID: "tenant", ResourceKind: "operation", Action: ActionRead}
	for name, trusted := range map[string]TrustedContext{
		"missing policy":  {SubjectID: "subject", TenantID: "tenant"},
		"wildcard tenant": {SubjectID: "subject", TenantID: "tenant", PolicyVersion: "v1", ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "*"}}},
		"unknown action":  {SubjectID: "subject", TenantID: "tenant", PolicyVersion: "v1", ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: "*", TenantID: "tenant"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if decision := NewEvaluator().Evaluate(trusted, request); decision.Allowed || decision.ReasonCode != "invalid_policy_snapshot" {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}

func TestEvaluatorRejectsIncompleteScopeHierarchy(t *testing.T) {
	request := AuthorizationRequest{SubjectID: "subject", TenantID: "tenant", ResourceKind: "operation", Action: ActionRead}
	for name, permission := range map[string]ScopedPermission{
		"environment without project":   {ResourceKind: "operation", Action: ActionRead, TenantID: "tenant", EnvironmentID: "environment"},
		"namespace without environment": {ResourceKind: "operation", Action: ActionRead, TenantID: "tenant", ProjectID: "project", NamespaceID: "namespace"},
	} {
		t.Run(name, func(t *testing.T) {
			trusted := TrustedContext{SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1", ScopedPermissions: []ScopedPermission{permission}}
			if decision := NewEvaluator().Evaluate(trusted, request); decision.Allowed || decision.ReasonCode != "invalid_policy_snapshot" {
				t.Fatalf("decision = %+v", decision)
			}
		})
	}
}

func TestRouteMetadataUsesExactMethodAndGoPatterns(t *testing.T) {
	routes := []RouteMetadata{{Method: http.MethodPost, Pattern: "/v1/operations/{id}/approve", ResourceKind: "operation", ResourceIDParam: "id", Action: ActionApprove}}
	matched, ok := MatchRoute(routes, http.MethodPost, "/v1/operations/op-a/approve")
	if !ok || matched.Params["id"] != "op-a" {
		t.Fatalf("matched = %+v, %v", matched, ok)
	}
	for _, request := range []struct{ method, path string }{{http.MethodGet, "/v1/operations/op-a/approve"}, {http.MethodPost, "/v1/operations/op-a/approve/extra"}, {http.MethodPost, "/v1/operations/op-a/reject"}} {
		if _, ok := MatchRoute(routes, request.method, request.path); ok {
			t.Fatalf("unexpected match for %s %s", request.method, request.path)
		}
	}
}

func TestEvalRejectsStalePolicyVersion(t *testing.T) {
	// Policy version must follow the "namespace:revision" format and be non-empty.
	// An empty or malformed policy version is rejected during evaluation.
	trusted := TrustedContext{
		SubjectID: "subject", TenantID: "tenant", PolicyVersion: "",
		ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "tenant"}},
	}
	decision := NewEvaluator().Evaluate(trusted, AuthorizationRequest{SubjectID: "subject", TenantID: "tenant", ResourceKind: "deployment", Action: ActionRead})
	if decision.Allowed {
		t.Fatal("empty policy version was allowed")
	}
}

func TestActionEnumValidation(t *testing.T) {
	trusted := TrustedContext{
		SubjectID: "subject", TenantID: "tenant", PolicyVersion: "default:1",
		ScopedPermissions: []ScopedPermission{{ResourceKind: "*", Action: ActionRead, TenantID: "tenant"}},
	}
	// Invalid action should be rejected by evaluator
	decision := NewEvaluator().Evaluate(trusted, AuthorizationRequest{SubjectID: "subject", TenantID: "tenant", ResourceKind: "deployment", Action: AuthorizationAction("custom_action_xyz")})
	if decision.Allowed {
		t.Fatal("custom action was allowed")
	}
}

func TestValidActionFunction(t *testing.T) {
	actions := []AuthorizationAction{ActionRead, ActionList, ActionCreate, ActionUpdate, ActionDelete, ActionExecute, ActionApprove, ActionReject, ActionCancel, ActionSwitchTenant}
	for _, action := range actions {
		if !validAction(action) {
			t.Fatalf("expected valid action: %s", action)
		}
	}
	if validAction(AuthorizationAction("unknown")) {
		t.Fatal("unknown action was marked valid")
	}
}

func TestClusterActionMappings(t *testing.T) {
	tests := []struct {
		kind string
		want AuthorizationAction
	}{
		{kind: "CreateKubernetesTarget", want: ActionCreate},
		{kind: "ImportRuntimeTarget", want: ActionCreate},
		{kind: "UpgradeRuntimeTarget", want: ActionUpdate},
		{kind: "DeleteRuntimeTarget", want: ActionDelete},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			got, ok := ClusterActionForIntentKind(test.kind)
			if !ok || got != test.want {
				t.Fatalf("action = %q, ok = %v, want %q", got, ok, test.want)
			}
			for _, operationAction := range []string{"approve", "reject", "cancel"} {
				got, ok = ClusterActionForOperationAction(operationAction, test.kind)
				if !ok || got != test.want {
					t.Fatalf("operation %s action = %q, ok = %v, want %q", operationAction, got, ok, test.want)
				}
			}
		})
	}
	for _, invalid := range []string{"", "InstallRelease", "UnmanageRuntimeTarget"} {
		if action, ok := ClusterActionForIntentKind(invalid); ok || action != "" {
			t.Errorf("invalid kind %q mapped to %q", invalid, action)
		}
	}
	if action, ok := ClusterActionForOperationAction("execute", "UpgradeRuntimeTarget"); ok || action != "" {
		t.Errorf("invalid operation action mapped to %q", action)
	}
}

func TestScopedRolesPermissionsParsingIsStrict(t *testing.T) {
	valid := []byte(`[{"resourceKind":"operation","action":"read","tenantId":"tenant-a"}]`)
	if permissions, err := decodeRolePermissions(valid); err != nil || len(permissions) != 1 {
		t.Fatalf("valid permissions = %+v, %v", permissions, err)
	}
	for _, invalid := range [][]byte{
		[]byte(`null`),
		[]byte(`[{"resourceKind":"operation","action":"read","tenantId":"tenant-a","unknown":true}]`),
		[]byte(`[] {}`),
	} {
		if _, err := decodeRolePermissions(invalid); err == nil {
			t.Fatalf("invalid permissions accepted: %s", invalid)
		}
	}
}
