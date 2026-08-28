package gateway

type GatewayType string

const (
	GwStandard      GatewayType = "standard"
	GwAPIManagement GatewayType = "api_management"
	GwMesh          GatewayType = "mesh"
	GwAI            GatewayType = "ai"
)

type GatewayStatus string

const (
	GwPending      GatewayStatus = "pending"
	GwReady        GatewayStatus = "ready"
	GwError        GatewayStatus = "error"
	GwDecommissioned GatewayStatus = "decommissioned"
)

type RouteStatus string

const (
	RoutePending  RouteStatus = "pending"
	RouteAccepted RouteStatus = "accepted"
	RouteRejected RouteStatus = "rejected"
	RouteOrphaned RouteStatus = "orphaned"
)

type GatewayClass struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	ControllerName string         `json:"controller_name"`
	Description    string         `json:"description,omitempty"`
	ParametersRef  map[string]any `json:"parameters_ref,omitempty"`
	IsDefault      bool           `json:"is_default"`
	IsActive       bool           `json:"is_active"`
}

type Listener struct {
	Name        string `json:"name"`
	Hostname    string `json:"hostname,omitempty"`
	Port        int32  `json:"port"`
	Protocol    string `json:"protocol"` // HTTP, HTTPS, TLS, TCP, UDP
	AllowRoute  string `json:"allow_route,omitempty"` // SameNamespace, All, Selector
	TLS         *TLSConfig `json:"tls,omitempty"`
}

type Gateway struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	Name           string            `json:"name"`
	GatewayClassID string            `json:"gateway_class_id"`
	Type           GatewayType       `json:"type"`
	Listeners      []Listener        `json:"listeners"`
	Namespace      string            `json:"namespace"`
	Status         GatewayStatus     `json:"status"`
	Labels         map[string]string `json:"labels"`
}

type TLSConfig struct {
	Mode           string   `json:"mode"` // Terminate, Passthrough
	CertificateRef string   `json:"certificate_ref"`
	MinVersion     string   `json:"min_version,omitempty"`
}

type HTTPRoute struct {
	ID               string            `json:"id"`
	GatewayProfileID string            `json:"gateway_profile_id"`
	Name             string            `json:"name"`
	Hostnames        []string          `json:"hostnames"`
	Rules            []HTTPRouteRule   `json:"rules"`
	Status           RouteStatus       `json:"status"`
	StatusReason     string            `json:"status_reason,omitempty"`
}

type HTTPRouteRule struct {
	Matches  []MatchCriteria   `json:"matches,omitempty"`
	Backends []WeightedBackend `json:"backends,omitempty"`
	Filters  []HTTPFilter      `json:"filters,omitempty"`
}

type MatchCriteria struct {
	Path    *PathMatch    `json:"path,omitempty"`
	Headers []HeaderMatch `json:"headers,omitempty"`
	Query   []QueryMatch  `json:"query,omitempty"`
	Method  string        `json:"method,omitempty"`
}

type PathMatch struct {
	Type  string `json:"type"` // PathPrefix, Exact, RegularExpression
	Value string `json:"value"`
}

type HeaderMatch struct {
	Type  string `json:"type"` // Exact, RegularExpression, Presence
	Name  string `json:"name"`
	Value string `json:"value"`
}

type QueryMatch struct {
	Type  string `json:"type"` // Exact, RegularExpression
	Name  string `json:"name"`
	Value string `json:"value"`
}

type WeightedBackend struct {
	Name   string `json:"name"`
	Port   int32  `json:"port"`
	Weight int32  `json:"weight"`
	Group  string `json:"group,omitempty"`
}

type HTTPFilter struct {
	Type       string           `json:"type"`
	RequestMirror  *MirrorTarget  `json:"request_mirror,omitempty"`
	RequestRewrite *RewriteRule   `json:"request_rewrite,omitempty"`
	Redirect    *RedirectRule    `json:"redirect,omitempty"`
	HeaderMod   *HeaderModifier  `json:"header_modifier,omitempty"`
}

type MirrorTarget struct {
	Name string `json:"name"`
	Port int32  `json:"port"`
}

type RewriteRule struct {
	Hostname        string `json:"hostname,omitempty"`
	PathPrefix      string `json:"path_prefix,omitempty"`
}

type RedirectRule struct {
	Scheme   string `json:"scheme,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Path     string `json:"path,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Code     int    `json:"code,omitempty"` // 301, 302, 307, 308
}

type HeaderModifier struct {
	Set    map[string]string `json:"set,omitempty"`
	Add    map[string]string `json:"add,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

type ReferenceGrant struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	FromNamespace string `json:"from_namespace"`
	ToNamespace   string `json:"to_namespace"`
	FromGroup     string `json:"from_group"`
	FromKind      string `json:"from_kind"`
	ToGroup       string `json:"to_group"`
	ToKind        string `json:"to_kind"`
	ToName        string `json:"to_name,omitempty"`
	IsActive      bool   `json:"is_active"`
}
