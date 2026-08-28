package gateway

import (
	"fmt"
)

type IstioAdapter struct {
	*GenericAdapter
}

func NewIstioAdapter(gatewayClassName string) *IstioAdapter {
	return &IstioAdapter{
		GenericAdapter: NewGenericAdapter("istio", gatewayClassName, nil, nil),
	}
}

func (a *IstioAdapter) ToVirtualService(profile *GatewayProfile, tenantID string) map[string]any {
	ns := a.ToGatewayNamespace(profile, tenantID)
	http := make([]map[string]any, 0, len(profile.Rules))

	for _, rule := range profile.Rules {
		route := map[string]any{
			"name": rule.Name,
		}
		if len(rule.Matches) > 0 {
			route["match"] = a.toVirtualServiceMatches(rule.Matches)
		}
		if rule.Redirect != nil {
			route["redirect"] = a.toVirtualServiceRedirect(rule.Redirect)
		}
		if len(rule.Backends) > 0 {
			route["route"] = a.toVirtualServiceDestinations(rule.Backends, ns)
		}
		if rule.Mirror != nil {
			route["mirror"] = a.toVirtualServiceMirror(rule.Mirror, ns)
		}
		if rule.Rewrite != nil {
			route["rewrite"] = a.toVirtualServiceRewrite(rule.Rewrite)
		}
		if rule.Headers != nil {
			hdrs := map[string]any{}
			if len(rule.Headers.Set) > 0 {
				hdrs["set"] = rule.Headers.Set
			}
			if len(rule.Headers.Remove) > 0 {
				hdrs["remove"] = rule.Headers.Remove
			}
			route["headers"] = hdrs
		}
		if rule.Timeout != "" {
			route["timeout"] = rule.Timeout
		}
		http = append(http, route)
	}

	hostnames := make([]any, 0)
	for _, rule := range profile.Rules {
		if rule.Hostname != "" {
			hostnames = append(hostnames, rule.Hostname)
		}
	}

	return map[string]any{
		"apiVersion": "networking.istio.io/v1beta1",
		"kind":       "VirtualService",
		"metadata": map[string]any{
			"name":      profile.Name + "-vs",
			"namespace": ns,
			"labels": map[string]any{
				"hnb.cloud/managed-by": "hnb-gateway-provider",
				"hnb.cloud/tenant-id":  tenantID,
			},
		},
		"spec": map[string]any{
			"hosts":    hostnames,
			"gateways": []string{profile.Name},
			"http":     http,
		},
	}
}

func (a *IstioAdapter) toVirtualServiceMatches(matches []MatchCriteria) []map[string]any {
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		vsMatch := map[string]any{}
		if m.Path != nil {
			vsMatch["uri"] = map[string]any{
				a.toVirtualServicePathType(m.Path.Type): m.Path.Value,
			}
		}
		if m.Method != "" {
			vsMatch["method"] = map[string]any{"exact": m.Method}
		}
		if len(m.Headers) > 0 {
			h := map[string]any{}
			for _, hm := range m.Headers {
				h[hm.Name] = map[string]any{a.toVirtualServiceHeaderType(hm.Type): hm.Value}
			}
			vsMatch["headers"] = h
		}
		if len(m.Query) > 0 {
			q := map[string]any{}
			for _, qm := range m.Query {
				q[qm.Name] = map[string]any{qm.Type: qm.Value}
			}
			vsMatch["queryParams"] = q
		}
		out = append(out, vsMatch)
	}
	return out
}

func (a *IstioAdapter) toVirtualServiceDestinations(backends []WeightedBackend, namespace string) []map[string]any {
	out := make([]map[string]any, 0, len(backends))
	for _, b := range backends {
		dst := map[string]any{
			"destination": map[string]any{
				"host": fmt.Sprintf("%s.%s.svc.cluster.local", b.Name, namespace),
				"port": map[string]any{"number": float64(b.Port)},
			},
		}
		if b.Weight > 0 {
			dst["weight"] = float64(b.Weight)
		}
		out = append(out, dst)
	}
	return out
}

func (a *IstioAdapter) toVirtualServiceRedirect(r *RedirectRule) map[string]any {
	rd := map[string]any{}
	if r.Scheme != "" {
		rd["scheme"] = r.Scheme
	}
	if r.Hostname != "" {
		rd["authority"] = r.Hostname
	}
	if r.Path != "" {
		rd["uri"] = r.Path
	}
	if r.Port > 0 {
		rd["port"] = float64(r.Port)
	}
	if r.Code > 0 {
		rd["redirectCode"] = float64(r.Code)
	}
	return rd
}

func (a *IstioAdapter) toVirtualServiceMirror(m *MirrorTarget, namespace string) map[string]any {
	return map[string]any{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", m.Name, namespace),
		"port": map[string]any{"number": float64(m.Port)},
	}
}

func (a *IstioAdapter) toVirtualServiceRewrite(r *RewriteRule) map[string]any {
	rw := map[string]any{}
	if r.PathPrefix != "" {
		rw["uri"] = r.PathPrefix
	}
	if r.Hostname != "" {
		rw["authority"] = r.Hostname
	}
	return rw
}

func (a *IstioAdapter) toVirtualServicePathType(t string) string {
	switch t {
	case "PathPrefix":
		return "prefix"
	case "Exact":
		return "exact"
	case "RegularExpression":
		return "regex"
	default:
		return "prefix"
	}
}

func (a *IstioAdapter) toVirtualServiceHeaderType(t string) string {
	switch t {
	case "Exact":
		return "exact"
	case "RegularExpression":
		return "regex"
	case "Presence":
		return "presence"
	default:
		return "exact"
	}
}