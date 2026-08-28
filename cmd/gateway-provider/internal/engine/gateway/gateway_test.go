package gateway

import (
	"testing"
)

func TestGatewayProfile_Validate(t *testing.T) {
	pv := NewProfileValidator()
	profile := &GatewayProfile{
		Name: "test-profile",
		Type: GwStandard,
		Rules: []ProfileRule{
			{Name: "rule-1", Backends: []WeightedBackend{{Name: "svc-a", Port: 8080, Weight: 100}}},
		},
	}
	errs := pv.Validate(profile)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestGatewayProfile_Validate_EmptyName(t *testing.T) {
	pv := NewProfileValidator()
	errs := pv.Validate(&GatewayProfile{Name: ""})
	if len(errs) == 0 {
		t.Error("expected error for empty name")
	}
}

func TestGatewayProfile_Validate_NoRulesNoListeners(t *testing.T) {
	pv := NewProfileValidator()
	errs := pv.Validate(&GatewayProfile{Name: "p", Rules: nil, Listeners: nil})
	if len(errs) == 0 {
		t.Error("expected error for empty profile")
	}
}

func TestGatewayProfile_Validate_RuleNoBackendNoRedirect(t *testing.T) {
	pv := NewProfileValidator()
	errs := pv.Validate(&GatewayProfile{Name: "p", Rules: []ProfileRule{{Name: "bad"}}})
	if len(errs) == 0 {
		t.Error("expected error for rule with no backends and no redirect")
	}
}

func TestGatewayProfile_Validate_MultiBackendNoWeights(t *testing.T) {
	pv := NewProfileValidator()
	errs := pv.Validate(&GatewayProfile{
		Name: "p",
		Rules: []ProfileRule{{
			Name:     "r1",
			Backends: []WeightedBackend{{Name: "a", Port: 80, Weight: 0}, {Name: "b", Port: 80, Weight: 0}},
		}},
	})
	if len(errs) == 0 {
		t.Error("expected error for multi-backend with no weights")
	}
}

func TestGatewayProfile_Validate_AIGatewayRequireTLS(t *testing.T) {
	pv := NewProfileValidator()
	errs := pv.Validate(&GatewayProfile{
		Name: "ai-gw",
		Type: GwAI,
		Rules: []ProfileRule{
			{Name: "r1", Backends: []WeightedBackend{{Name: "svc", Port: 443, Weight: 100}}},
		},
		TLS: nil,
	})
	found := false
	for _, e := range errs {
		if e.Field == "tls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TLS validation error for AI gateway")
	}
}

func TestCapabilityChecker_Pass(t *testing.T) {
	cc := NewCapabilityChecker()
	result := cc.Check(
		&GatewayRequirements{RequiredRoutes: []string{"HTTPRoute"}},
		&GatewayCapabilitySnapshot{
			ProviderName:    "istio",
			SupportedRoutes: []string{"HTTPRoute", "GRPCRoute"},
			CoreFeatures:    []string{"HTTPRoute.HostRewrite"},
		},
	)
	if !result.Passed {
		t.Errorf("expected pass, got: %v", result.Issues)
	}
}

func TestCapabilityChecker_RouteNotSupported(t *testing.T) {
	cc := NewCapabilityChecker()
	result := cc.Check(
		&GatewayRequirements{RequiredRoutes: []string{"GRPCRoute"}},
		&GatewayCapabilitySnapshot{ProviderName: "envoy", SupportedRoutes: []string{"HTTPRoute"}},
	)
	if result.Passed {
		t.Error("expected fail when route type unsupported")
	}
}

func TestCapabilityChecker_FeatureNotSupported(t *testing.T) {
	cc := NewCapabilityChecker()
	result := cc.Check(
		&GatewayRequirements{RequiredFeatures: []string{"HTTPRoute.RequestMirror"}},
		&GatewayCapabilitySnapshot{
			ProviderName:    "simple-gw",
			SupportedRoutes: []string{"HTTPRoute"},
			CoreFeatures:    []string{},
		},
	)
	if result.Passed {
		t.Error("expected fail when feature unsupported")
	}
}

func TestCapabilityChecker_ExtendedFeature(t *testing.T) {
	cc := NewCapabilityChecker()
	result := cc.Check(
		&GatewayRequirements{RequiredFeatures: []string{"HTTPRoute.WeightedSplit"}},
		&GatewayCapabilitySnapshot{
			ProviderName:     "istio",
			SupportedRoutes:  []string{"HTTPRoute"},
			ExtendedFeatures: []string{"HTTPRoute.WeightedSplit"},
		},
	)
	if !result.Passed {
		t.Errorf("expected pass for extended feature, got: %v", result.Issues)
	}
}

func TestMultiTenantValidator_SameNamespace(t *testing.T) {
	mv := NewMultiTenantValidator()
	result := mv.CheckCrossNamespaceRef(&RouteNamespace{
		RouteNamespace:   "ns-a",
		BackendNamespace: "ns-a",
		RouteKind:        "HTTPRoute",
	})
	if !result.Allowed {
		t.Error("same namespace should always be allowed")
	}
}

func TestMultiTenantValidator_CrossNamespaceNoGrant(t *testing.T) {
	mv := NewMultiTenantValidator()
	result := mv.CheckCrossNamespaceRef(&RouteNamespace{
		RouteNamespace:   "ns-a",
		BackendNamespace: "ns-b",
		RouteKind:        "HTTPRoute",
		BackendKind:      "Service",
		BackendName:      "svc-b",
	})
	if result.Allowed {
		t.Error("cross-namespace without grant should be denied")
	}
}

func TestMultiTenantValidator_CrossNamespaceWithGrant(t *testing.T) {
	mv := NewMultiTenantValidator()
	mv.AddGrant(&ReferenceGrant{
		ID:            "grant-1",
		FromNamespace: "ns-a",
		ToNamespace:   "ns-b",
		FromKind:      "HTTPRoute",
		ToKind:        "Service",
		IsActive:      true,
	})
	result := mv.CheckCrossNamespaceRef(&RouteNamespace{
		RouteNamespace:   "ns-a",
		BackendNamespace: "ns-b",
		RouteKind:        "HTTPRoute",
		BackendKind:      "Service",
		BackendName:      "svc-b",
	})
	if !result.Allowed {
		t.Errorf("cross-namespace with grant should be allowed, got: %s", result.Reason)
	}
}

func TestMultiTenantValidator_CrossNamespaceInactiveGrant(t *testing.T) {
	mv := NewMultiTenantValidator()
	mv.AddGrant(&ReferenceGrant{
		ID:            "grant-1",
		FromNamespace: "ns-a",
		ToNamespace:   "ns-b",
		FromKind:      "HTTPRoute",
		ToKind:        "Service",
		IsActive:      false,
	})
	result := mv.CheckCrossNamespaceRef(&RouteNamespace{
		RouteNamespace:   "ns-a",
		BackendNamespace: "ns-b",
		RouteKind:        "HTTPRoute",
		BackendKind:      "Service",
	})
	if result.Allowed {
		t.Error("inactive grant should not allow cross-namespace")
	}
}

func TestMultiTenantValidator_AllowedRoutes_SameNamespace(t *testing.T) {
	mv := NewMultiTenantValidator()
	gw := &Gateway{Namespace: "ns-a", Listeners: []Listener{{Name: "http", AllowRoute: "SameNamespace"}}}
	result := mv.CheckAllowedRoutes(gw, "ns-a")
	if !result.Allowed {
		t.Errorf("same namespace should be allowed, got: %s", result.Reason)
	}
	result = mv.CheckAllowedRoutes(gw, "ns-b")
	if result.Allowed {
		t.Error("different namespace with SameNamespace should be denied")
	}
}

func TestMultiTenantValidator_AllowedRoutes_All(t *testing.T) {
	mv := NewMultiTenantValidator()
	gw := &Gateway{Namespace: "ns-a", Listeners: []Listener{{Name: "http", AllowRoute: "All"}}}
	result := mv.CheckAllowedRoutes(gw, "ns-other")
	if !result.Allowed {
		t.Error("All should allow any namespace")
	}
}

func TestMultiTenantValidator_AllowedRoutes_Default(t *testing.T) {
	mv := NewMultiTenantValidator()
	gw := &Gateway{Namespace: "ns-a", Listeners: []Listener{{Name: "http", AllowRoute: ""}}}
	result := mv.CheckAllowedRoutes(gw, "ns-b")
	if result.Allowed {
		t.Error("default SameNamespace should deny different namespace")
	}
}

func TestTrafficTierValidator_Allowed(t *testing.T) {
	tv := NewTrafficTierValidator()

	if result := tv.Check("application", GwStandard); !result.Allowed {
		t.Errorf("app→standard should be allowed, got: %s", result.Reason)
	}
	if result := tv.Check("mesh", GwMesh); !result.Allowed {
		t.Errorf("mesh→mesh should be allowed")
	}
	if result := tv.Check("ai", GwAI); !result.Allowed {
		t.Errorf("ai→ai should be allowed")
	}
}

func TestTrafficTierValidator_AppNotAI(t *testing.T) {
	tv := NewTrafficTierValidator()
	result := tv.Check("application", GwAI)
	if result.Allowed {
		t.Error("app→AI should be denied")
	}
	if result.Reason == "" {
		t.Error("should include reason")
	}
}

func TestTrafficTierValidator_MeshNotStandard(t *testing.T) {
	tv := NewTrafficTierValidator()
	result := tv.Check("mesh", GwStandard)
	if result.Allowed {
		t.Error("mesh→standard should be denied")
	}
}

func TestGatewayExecutor_ToStepSpec(t *testing.T) {
	gb := NewGatewayExecutor()
	profile := &GatewayProfile{
		Name:     "prod-gw",
		Type:     GwStandard,
		ProfileDigest: "abc123def4567890123456",
		Rules:    []ProfileRule{
			{Name: "route-1", Backends: []WeightedBackend{{Name: "svc-a", Port: 80, Weight: 100}}},
		},
		Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
	}
	task := gb.ToTask(profile)
	if task.StepType != "configure_gateway" {
		t.Errorf("step_type = %s, want configure_gateway", task.StepType)
	}
	if task.Name != "configure-gateway-prod-gw" {
		t.Errorf("name = %s, want configure-gateway-prod-gw", task.Name)
	}
}

func TestGatewayExecutor_ValidateAndPrepare_Pass(t *testing.T) {
	gb := NewGatewayExecutor()
	err := gb.ValidateAndPrepare(
		&GatewayProfile{Name: "gw", Type: GwStandard, Rules: []ProfileRule{
			{Name: "r1", Backends: []WeightedBackend{{Name: "svc", Port: 80, Weight: 100}}},
		}},
		&GatewayRequirements{RequiredRoutes: []string{"HTTPRoute"}},
		&GatewayCapabilitySnapshot{
			ProviderName: "istio", SupportedRoutes: []string{"HTTPRoute"},
		},
		&Gateway{Namespace: "default", Listeners: []Listener{{Name: "http", AllowRoute: "All"}}},
	)
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestGatewayExecutor_ValidateAndPrepare_Fail(t *testing.T) {
	gb := NewGatewayExecutor()
	err := gb.ValidateAndPrepare(
		&GatewayProfile{Name: "bad-gw", Type: GwStandard},
		&GatewayRequirements{RequiredRoutes: []string{"GRPCRoute"}},
		&GatewayCapabilitySnapshot{
			ProviderName: "simple", SupportedRoutes: []string{"HTTPRoute"},
		},
		&Gateway{Namespace: "default", Listeners: []Listener{{Name: "http"}}},
	)
	if err == nil {
		t.Error("expected validation failure")
	}
}

func TestGatewayProfile_FromWeights(t *testing.T) {
	backends := []WeightedBackend{
		{Name: "v1", Port: 80, Weight: 90},
		{Name: "v2", Port: 80, Weight: 10},
	}
	total := int32(0)
	for _, b := range backends {
		total += b.Weight
	}
	if total != 100 {
		t.Errorf("total weight = %d, want 100", total)
	}
}
