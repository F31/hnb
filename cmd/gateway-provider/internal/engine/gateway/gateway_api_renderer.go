package gateway

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type GatewayRenderer struct {
	gatewayClassName string
	extraLabels      map[string]string
	extraAnnotations map[string]string
}

func NewGatewayRenderer(gatewayClassName string, extraLabels, extraAnnotations map[string]string) *GatewayRenderer {
	return &GatewayRenderer{
		gatewayClassName: gatewayClassName,
		extraLabels:      extraLabels,
		extraAnnotations: extraAnnotations,
	}
}

func (r *GatewayRenderer) RenderGateway(profile *GatewayProfile, tenantID string) *unstructured.Unstructured {
	ns := "hnb-" + tenantID
	listeners := make([]any, 0, len(profile.Listeners))
	for _, l := range profile.Listeners {
		gl := map[string]any{
			"name":     l.Name,
			"port":     float64(l.Port),
			"protocol": toProtocol(l.Protocol),
		}
		if l.Hostname != "" {
			gl["hostname"] = l.Hostname
		}
		if l.TLS != nil {
			tls := map[string]any{}
			if l.TLS.Mode != "" {
				tls["mode"] = l.TLS.Mode
			}
			if l.TLS.CertificateRef != "" {
				tls["certificateRefs"] = []any{
					map[string]any{"name": l.TLS.CertificateRef},
				}
			}
			gl["tls"] = tls
		}
		if l.AllowRoute != "" {
			gl["allowedRoutes"] = map[string]any{
				"namespaces": map[string]any{"from": toAllowRoute(l.AllowRoute)},
			}
		}
		listeners = append(listeners, gl)
	}

	labels := map[string]string{
		"hnb.cloud/managed-by":   "hnb-gateway-provider",
		"hnb.cloud/tenant-id":    tenantID,
		"hnb.cloud/gateway-type": string(profile.Type),
	}
	for k, v := range r.extraLabels {
		labels[k] = v
	}

	labelsAny := make(map[string]any, len(labels))
	for k, v := range labels {
		labelsAny[k] = v
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":      profile.Name,
				"namespace": ns,
				"labels":    labelsAny,
			},
			"spec": map[string]any{
				"gatewayClassName": r.gatewayClassName,
				"listeners":        listeners,
			},
		},
	}

	if len(r.extraAnnotations) > 0 {
		anns := make(map[string]any, len(r.extraAnnotations))
		for k, v := range r.extraAnnotations {
			anns[k] = v
		}
		obj.Object["metadata"].(map[string]any)["annotations"] = anns
	}

	return obj
}

func (r *GatewayRenderer) RenderHTTPRoute(profile *GatewayProfile, tenantID string) *unstructured.Unstructured {
	ns := "hnb-" + tenantID
	rules := make([]any, 0, len(profile.Rules))
	hostnames := collectHostnames(profile)

	for _, rule := range profile.Rules {
		rr := map[string]any{}
		if len(rule.Matches) > 0 {
			rr["matches"] = toMatches(rule.Matches)
		}
		if len(rule.Backends) > 0 {
			rr["backendRefs"] = toBackendRefs(rule.Backends)
		}
		filters := toFilters(rule)
		if len(filters) > 0 {
			rr["filters"] = filters
		}
		rules = append(rules, rr)
	}

	labels := map[string]string{
		"hnb.cloud/managed-by": "hnb-gateway-provider",
		"hnb.cloud/tenant-id":  tenantID,
	}
	for k, v := range r.extraLabels {
		labels[k] = v
	}

	labelsAny := make(map[string]any, len(labels))
	for k, v := range labels {
		labelsAny[k] = v
	}

	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      profile.Name + "-httproute",
				"namespace": ns,
				"labels":    labelsAny,
			},
			"spec": map[string]any{
				"parentRefs": []any{
					map[string]any{
						"name":      profile.Name,
						"namespace": ns,
					},
				},
				"rules": rules,
			},
		},
	}

	if len(hostnames) > 0 {
		hostnamesRaw := make([]any, len(hostnames))
		for i, h := range hostnames {
			hostnamesRaw[i] = string(h)
		}
		route.Object["spec"].(map[string]any)["hostnames"] = hostnamesRaw
	}

	return route
}

func toProtocol(p string) string {
	switch p {
	case "HTTP", "HTTPS", "TLS", "TCP":
		return p
	default:
		return "HTTP"
	}
}

func toAllowRoute(t string) string {
	switch t {
	case "SameNamespace":
		return "Same"
	case "All":
		return "All"
	case "Selector":
		return "Selector"
	default:
		return "Same"
	}
}

func toMatches(matches []MatchCriteria) []any {
	out := make([]any, 0, len(matches))
	for _, m := range matches {
		gm := map[string]any{}
		if m.Path != nil {
			gm["path"] = map[string]any{
				"type":  toPathMatchType(m.Path.Type),
				"value": m.Path.Value,
			}
		}
		if m.Method != "" {
			gm["method"] = m.Method
		}
		if len(m.Headers) > 0 {
			headers := make([]any, 0, len(m.Headers))
			for _, h := range m.Headers {
				headers = append(headers, map[string]any{
					"type":  toHeaderMatchType(h.Type),
					"name":  h.Name,
					"value": h.Value,
				})
			}
			gm["headers"] = headers
		}
		if len(m.Query) > 0 {
			query := make([]any, 0, len(m.Query))
			for _, q := range m.Query {
				query = append(query, map[string]any{
					"type":  toQueryMatchType(q.Type),
					"name":  q.Name,
					"value": q.Value,
				})
			}
			gm["queryParams"] = query
		}
		out = append(out, gm)
	}
	return out
}

func toBackendRefs(backends []WeightedBackend) []any {
	out := make([]any, 0, len(backends))
	for _, b := range backends {
		ref := map[string]any{
			"name": b.Name,
			"port": float64(b.Port),
		}
		if b.Weight > 0 {
			ref["weight"] = float64(b.Weight)
		}
		out = append(out, ref)
	}
	return out
}

func toFilters(rule ProfileRule) []any {
	var filters []any

	if rule.Redirect != nil {
		r := map[string]any{
			"type": "RequestRedirect",
			"requestRedirect": map[string]any{
				"statusCode": float64(rule.Redirect.Code),
			},
		}
		if rule.Redirect.Scheme != "" {
			r["requestRedirect"].(map[string]any)["scheme"] = rule.Redirect.Scheme
		}
		if rule.Redirect.Hostname != "" {
			r["requestRedirect"].(map[string]any)["hostname"] = rule.Redirect.Hostname
		}
		if rule.Redirect.Path != "" {
			r["requestRedirect"].(map[string]any)["path"] = map[string]any{
				"type":            "ReplaceFullPath",
				"replaceFullPath": rule.Redirect.Path,
			}
		}
		if rule.Redirect.Port > 0 {
			r["requestRedirect"].(map[string]any)["port"] = float64(rule.Redirect.Port)
		}
		filters = append(filters, r)
	}

	if rule.Rewrite != nil {
		rw := map[string]any{
			"type": "URLRewrite",
			"urlRewrite": map[string]any{},
		}
		if rule.Rewrite.PathPrefix != "" {
			rw["urlRewrite"].(map[string]any)["path"] = map[string]any{
				"type":                "ReplacePrefixMatch",
				"replacePrefixMatch": rule.Rewrite.PathPrefix,
			}
		}
		if rule.Rewrite.Hostname != "" {
			rw["urlRewrite"].(map[string]any)["hostname"] = rule.Rewrite.Hostname
		}
		filters = append(filters, rw)
	}

	if rule.Headers != nil {
		h := map[string]any{
			"type": "RequestHeaderModifier",
			"requestHeaderModifier": map[string]any{},
		}
		if len(rule.Headers.Set) > 0 {
			set := make([]any, 0, len(rule.Headers.Set))
			for k, v := range rule.Headers.Set {
				set = append(set, map[string]any{"name": k, "value": v})
			}
			h["requestHeaderModifier"].(map[string]any)["set"] = set
		}
		if len(rule.Headers.Add) > 0 {
			add := make([]any, 0, len(rule.Headers.Add))
			for k, v := range rule.Headers.Add {
				add = append(add, map[string]any{"name": k, "value": v})
			}
			h["requestHeaderModifier"].(map[string]any)["add"] = add
		}
		if len(rule.Headers.Remove) > 0 {
			h["requestHeaderModifier"].(map[string]any)["remove"] = rule.Headers.Remove
		}
		filters = append(filters, h)
	}

	if rule.Mirror != nil {
		filters = append(filters, map[string]any{
			"type": "RequestMirror",
			"requestMirror": map[string]any{
				"backendRef": map[string]any{
					"name": rule.Mirror.Name,
					"port": rule.Mirror.Port,
				},
			},
		})
	}

	return filters
}

func collectHostnames(profile *GatewayProfile) []string {
	seen := map[string]bool{}
	var out []string
	for _, rule := range profile.Rules {
		if rule.Hostname != "" && !seen[rule.Hostname] {
			seen[rule.Hostname] = true
			out = append(out, rule.Hostname)
		}
	}
	return out
}

func toPathMatchType(t string) string {
	switch t {
	case "PathPrefix":
		return "PathPrefix"
	case "Exact":
		return "Exact"
	case "RegularExpression":
		return "RegularExpression"
	default:
		return "PathPrefix"
	}
}

func toHeaderMatchType(t string) string {
	switch t {
	case "Exact":
		return "Exact"
	case "RegularExpression":
		return "RegularExpression"
	default:
		return "Exact"
	}
}

func toQueryMatchType(t string) string {
	switch t {
	case "Exact":
		return "Exact"
	case "RegularExpression":
		return "RegularExpression"
	default:
		return "Exact"
	}
}