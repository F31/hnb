package gateway

import (
	"fmt"
	"testing"
)

func TestGatewayRenderer_RenderGateway(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "gw-test", TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{
			{Name: "http", Port: 80, Protocol: "HTTP", Hostname: "example.com"},
		},
	}
	obj := r.RenderGateway(profile, "tenant-1")
	if obj == nil {
		t.Fatal("RenderGateway returned nil")
	}
	if obj.GetName() != "gw-test" {
		t.Errorf("expected name gw-test, got %s", obj.GetName())
	}
	if obj.GetNamespace() != "hnb-tenant-1" {
		t.Errorf("expected namespace hnb-tenant-1, got %s", obj.GetNamespace())
	}
	spec := obj.Object["spec"].(map[string]any)
	if spec["gatewayClassName"] != "test-class" {
		t.Errorf("expected gatewayClassName test-class, got %v", spec["gatewayClassName"])
	}
}

func TestGatewayRenderer_RenderGateway_WithTLS(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "gw-tls", TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{
			{
				Name: "https", Port: 443, Protocol: "HTTPS", Hostname: "secure.example.com",
				TLS: &TLSConfig{Mode: "Terminate", CertificateRef: "my-cert"},
			},
		},
	}
	obj := r.RenderGateway(profile, "tenant-1")
	listeners := obj.Object["spec"].(map[string]any)["listeners"].([]any)
	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(listeners))
	}
	l := listeners[0].(map[string]any)
	portVal := fmt.Sprintf("%v", l["port"])
	if portVal != "443" {
		t.Errorf("expected port 443, got %v", l["port"])
	}
	tls := l["tls"].(map[string]any)
	if tls["mode"] != "Terminate" {
		t.Errorf("expected TLS mode Terminate, got %v", tls["mode"])
	}
}

func TestGatewayRenderer_RenderGateway_WithExtraLabels(t *testing.T) {
	extra := map[string]string{"hnb.cloud/adapter": "cilium"}
	r := NewGatewayRenderer("cilium", extra, nil)
	profile := &GatewayProfile{
		Name: "gw-labels", TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
	}
	obj := r.RenderGateway(profile, "tenant-1")
	labels := obj.GetLabels()
	if labels["hnb.cloud/adapter"] != "cilium" {
		t.Errorf("expected extra label, got %v", labels)
	}
}

func TestGatewayRenderer_RenderHTTPRoute(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "route-test", TenantID: "tenant-1", Type: GwStandard,
		Rules: []ProfileRule{
			{
				Name: "rule-1",
				Matches: []MatchCriteria{
					{Path: &PathMatch{Type: "PathPrefix", Value: "/api"}},
				},
				Backends: []WeightedBackend{{Name: "svc-a", Port: 8080, Weight: 100}},
			},
		},
	}
	obj := r.RenderHTTPRoute(profile, "tenant-1")
	if obj == nil {
		t.Fatal("RenderHTTPRoute returned nil")
	}
	if obj.GetName() != "route-test-httproute" {
		t.Errorf("expected name route-test-httproute, got %s", obj.GetName())
	}
	spec := obj.Object["spec"].(map[string]any)
	rules := spec["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestGatewayRenderer_RenderHTTPRoute_WithFilters(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "route-filters", TenantID: "tenant-1", Type: GwStandard,
		Rules: []ProfileRule{
			{
				Name: "redirect-rule",
				Redirect: &RedirectRule{Scheme: "https", Code: 301},
				Backends: []WeightedBackend{{Name: "svc-a", Port: 8080, Weight: 100}},
			},
			{
				Name: "rewrite-rule",
				Rewrite: &RewriteRule{PathPrefix: "/new"},
				Backends: []WeightedBackend{{Name: "svc-b", Port: 9090, Weight: 100}},
			},
		},
	}
	obj := r.RenderHTTPRoute(profile, "tenant-1")
	spec := obj.Object["spec"].(map[string]any)
	rules := spec["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
}

func TestGatewayRenderer_RenderHTTPRoute_WithHostnames(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "route-host", TenantID: "tenant-1", Type: GwStandard,
		Rules: []ProfileRule{
			{
				Name: "rule-a", Hostname: "app.example.com",
				Backends: []WeightedBackend{{Name: "svc-a", Port: 8080, Weight: 100}},
			},
		},
	}
	obj := r.RenderHTTPRoute(profile, "tenant-1")
	spec := obj.Object["spec"].(map[string]any)
	hostnames := spec["hostnames"].([]any)
	if len(hostnames) != 1 || hostnames[0] != "app.example.com" {
		t.Errorf("expected hostname app.example.com, got %v", hostnames)
	}
}

func TestGatewayRenderer_RenderHTTPRoute_EmptyProfile(t *testing.T) {
	r := NewGatewayRenderer("test-class", nil, nil)
	profile := &GatewayProfile{
		Name: "empty", TenantID: "tenant-1", Type: GwStandard,
	}
	obj := r.RenderHTTPRoute(profile, "tenant-1")
	if obj == nil {
		t.Fatal("RenderHTTPRoute with empty profile returned nil")
	}
}

func TestGatewayRenderer_toProtocol(t *testing.T) {
	tests := []struct{ input, want string }{
		{"HTTP", "HTTP"}, {"HTTPS", "HTTPS"}, {"TLS", "TLS"}, {"TCP", "TCP"},
		{"", "HTTP"}, {"GRPC", "HTTP"},
	}
	for _, tt := range tests {
		got := toProtocol(tt.input)
		if got != tt.want {
			t.Errorf("toProtocol(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGatewayRenderer_toPathMatchType(t *testing.T) {
	tests := []struct{ input, want string }{
		{"PathPrefix", "PathPrefix"}, {"Exact", "Exact"}, {"RegularExpression", "RegularExpression"},
		{"", "PathPrefix"}, {"Invalid", "PathPrefix"},
	}
	for _, tt := range tests {
		got := toPathMatchType(tt.input)
		if got != tt.want {
			t.Errorf("toPathMatchType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGatewayRenderer_toAllowRoute(t *testing.T) {
	tests := []struct{ input, want string }{
		{"SameNamespace", "Same"}, {"All", "All"}, {"Selector", "Selector"},
		{"", "Same"}, {"Invalid", "Same"},
	}
	for _, tt := range tests {
		got := toAllowRoute(tt.input)
		if got != tt.want {
			t.Errorf("toAllowRoute(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGatewayRenderer_collectHostnames(t *testing.T) {
	profile := &GatewayProfile{
		Rules: []ProfileRule{
			{Hostname: "a.example.com", Backends: []WeightedBackend{{Name: "s", Port: 80}}},
			{Hostname: "b.example.com", Backends: []WeightedBackend{{Name: "s", Port: 80}}},
			{Hostname: "a.example.com", Backends: []WeightedBackend{{Name: "s", Port: 80}}},
		},
	}
	hosts := collectHostnames(profile)
	if len(hosts) != 2 {
		t.Errorf("expected 2 unique hostnames, got %d: %v", len(hosts), hosts)
	}
}

func TestGatewayRenderer_toFilters(t *testing.T) {
	rule := ProfileRule{
		Name: "filter-test",
		Redirect: &RedirectRule{Scheme: "https", Hostname: "new.example.com", Path: "/new-path", Port: 443, Code: 302},
		Headers: &HeaderModifier{
			Set:    map[string]string{"X-Custom": "value"},
			Remove: []string{"X-Internal"},
		},
		Mirror: &MirrorTarget{Name: "mirror-svc", Port: 8080},
		Backends: []WeightedBackend{{Name: "svc-a", Port: 80, Weight: 100}},
	}
	filters := toFilters(rule)
	if len(filters) == 0 {
		t.Fatal("toFilters returned empty")
	}
}

func TestGatewayRenderer_toMatches(t *testing.T) {
	matches := []MatchCriteria{
		{
			Path:   &PathMatch{Type: "PathPrefix", Value: "/api"},
			Method: "GET",
			Headers: []HeaderMatch{
				{Type: "Exact", Name: "X-Version", Value: "v1"},
			},
			Query: []QueryMatch{
				{Type: "Exact", Name: "env", Value: "prod"},
			},
		},
	}
	result := toMatches(matches)
	if len(result) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result))
	}
}