package gateway

type MultiTenantValidator struct {
	grants map[string]*ReferenceGrant
}

func NewMultiTenantValidator() *MultiTenantValidator {
	return &MultiTenantValidator{
		grants: make(map[string]*ReferenceGrant),
	}
}

func (mv *MultiTenantValidator) AddGrant(g *ReferenceGrant) {
	mv.grants[g.ID] = g
}

type RouteNamespace struct {
	RouteNamespace string `json:"route_namespace"`
	RouteKind      string `json:"route_kind"`
	BackendNamespace string `json:"backend_namespace"`
	BackendKind    string `json:"backend_kind"`
	BackendName    string `json:"backend_name"`
}

func (mv *MultiTenantValidator) CheckCrossNamespaceRef(ref *RouteNamespace) *AuthResult {
	if ref.RouteNamespace == ref.BackendNamespace {
		return &AuthResult{Allowed: true}
	}

	for _, grant := range mv.grants {
		if !grant.IsActive {
			continue
		}
		if grant.FromNamespace != ref.RouteNamespace {
			continue
		}
		if grant.ToNamespace != ref.BackendNamespace {
			continue
		}
		if grant.FromKind != ref.RouteKind {
			continue
		}
		if grant.ToKind != ref.BackendKind {
			continue
		}
		if grant.ToName != "" && grant.ToName != ref.BackendName {
			continue
		}
		return &AuthResult{Allowed: true}
	}

	return &AuthResult{
		Allowed: false,
		Reason:  "no matching ReferenceGrant from " + ref.RouteNamespace + " to " + ref.BackendNamespace,
	}
}

func (mv *MultiTenantValidator) CheckAllowedRoutes(gw *Gateway, routeNamespace string) *AuthResult {
	for _, l := range gw.Listeners {
		switch l.AllowRoute {
		case "SameNamespace":
			if routeNamespace != gw.Namespace {
				return &AuthResult{
					Allowed: false,
					Reason:  "listener " + l.Name + " only allows routes in namespace " + gw.Namespace,
				}
			}
		case "Selector":
			return &AuthResult{
				Allowed: false,
				Reason:  "listener " + l.Name + " uses Selector mode (not implemented: namespace selector labels not configured)",
			}
		case "All":
			return &AuthResult{Allowed: true}
		case "":
			if routeNamespace != gw.Namespace {
				return &AuthResult{
					Allowed: false,
					Reason:  "default SameNamespace: route namespace " + routeNamespace + " != gateway namespace " + gw.Namespace,
				}
			}
		}
	}
	return &AuthResult{Allowed: true}
}

type AuthResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}
