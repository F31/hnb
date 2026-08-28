package gateway

import (
	"testing"
)

func TestIstioAdapter_Name(t *testing.T) {
	a := NewIstioAdapter("istio")
	if a.Name() != "istio" {
		t.Errorf("expected istio, got %s", a.Name())
	}
}

func TestIstioAdapter_GatewayClassName(t *testing.T) {
	a := NewIstioAdapter("my-istio")
	if a.GatewayClassName() != "my-istio" {
		t.Errorf("expected my-istio, got %s", a.GatewayClassName())
	}
}

func TestIstioAdapter_DefaultGatewayClassName(t *testing.T) {
	a := NewIstioAdapter("")
	if a.GatewayClassName() != "istio" {
		t.Errorf("expected default istio, got %s", a.GatewayClassName())
	}
}

func TestIstioAdapter_ToUnstructuredGateway(t *testing.T) {
	a := NewIstioAdapter("istio")
	profile := &GatewayProfile{
		Name: "istio-gw", TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
		Rules:     []ProfileRule{{Name: "r1", Backends: []WeightedBackend{{Name: "s", Port: 80, Weight: 100}}}},
	}
	obj := a.ToUnstructuredGateway(profile, "tenant-1")
	if obj == nil {
		t.Fatal("ToUnstructuredGateway returned nil")
	}
	if obj.GetName() != "istio-gw" {
		t.Errorf("expected name istio-gw, got %s", obj.GetName())
	}
	spec := obj.Object["spec"].(map[string]any)
	if spec["gatewayClassName"] != "istio" {
		t.Errorf("expected gatewayClassName istio, got %v", spec["gatewayClassName"])
	}
}

func TestIstioAdapter_ToUnstructuredHTTPRoute(t *testing.T) {
	a := NewIstioAdapter("istio")
	profile := &GatewayProfile{
		Name: "istio-route", TenantID: "tenant-1", Type: GwStandard,
		Rules: []ProfileRule{{Name: "r1", Backends: []WeightedBackend{{Name: "s", Port: 80, Weight: 100}}}},
	}
	obj := a.ToUnstructuredHTTPRoute(profile, "tenant-1")
	if obj == nil {
		t.Fatal("ToUnstructuredHTTPRoute returned nil")
	}
	if obj.GetName() != "istio-route-httproute" {
		t.Errorf("expected istio-route-httproute, got %s", obj.GetName())
	}
}

func TestIstioAdapter_ToGatewayNamespace(t *testing.T) {
	a := NewIstioAdapter("istio")
	profile := &GatewayProfile{Name: "test", TenantID: "tenant-1"}
	ns := a.ToGatewayNamespace(profile, "tenant-1")
	if ns != "hnb-tenant-1" {
		t.Errorf("expected hnb-tenant-1, got %s", ns)
	}
}

func TestCiliumAdapter_Name(t *testing.T) {
	a := NewCiliumAdapter("cilium")
	if a.Name() != "cilium" {
		t.Errorf("expected cilium, got %s", a.Name())
	}
}

func TestCiliumAdapter_GatewayClassName(t *testing.T) {
	a := NewCiliumAdapter("")
	if a.GatewayClassName() != CiliumGatewayClass {
		t.Errorf("expected %s, got %s", CiliumGatewayClass, a.GatewayClassName())
	}
}

func TestCiliumAdapter_ToUnstructuredGateway(t *testing.T) {
	a := NewCiliumAdapter("cilium")
	profile := &GatewayProfile{
		Name: "cilium-gw", TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
		Rules:     []ProfileRule{{Name: "r1", Backends: []WeightedBackend{{Name: "s", Port: 80, Weight: 100}}}},
	}
	obj := a.ToUnstructuredGateway(profile, "tenant-1")
	if obj == nil {
		t.Fatal("ToUnstructuredGateway returned nil")
	}
	labels := obj.GetLabels()
	if labels["hnb.cloud/adapter"] != "cilium" {
		t.Errorf("expected cilium label, got %v", labels)
	}
	spec := obj.Object["spec"].(map[string]any)
	if spec["gatewayClassName"] != "cilium" {
		t.Errorf("expected gatewayClassName cilium, got %v", spec["gatewayClassName"])
	}
}

func TestCiliumAdapter_ToUnstructuredHTTPRoute(t *testing.T) {
	a := NewCiliumAdapter("cilium")
	profile := &GatewayProfile{
		Name: "cilium-route", TenantID: "tenant-1", Type: GwStandard,
		Rules: []ProfileRule{{Name: "r1", Backends: []WeightedBackend{{Name: "s", Port: 80, Weight: 100}}}},
	}
	obj := a.ToUnstructuredHTTPRoute(profile, "tenant-1")
	if obj == nil {
		t.Fatal("ToUnstructuredHTTPRoute returned nil")
	}
	labels := obj.GetLabels()
	if labels["hnb.cloud/adapter"] != "cilium" {
		t.Errorf("expected cilium label, got %v", labels)
	}
}

func TestCiliumAdapter_ToGatewayNamespace(t *testing.T) {
	a := NewCiliumAdapter("cilium")
	profile := &GatewayProfile{Name: "test", TenantID: "tenant-1"}
	ns := a.ToGatewayNamespace(profile, "tenant-1")
	if ns != "hnb-tenant-1" {
		t.Errorf("expected hnb-tenant-1, got %s", ns)
	}
}

func TestAdapters_InterfaceCompliance(t *testing.T) {
	var istioAd GatewayAdapter = NewIstioAdapter("istio")
	var ciliumAd GatewayAdapter = NewCiliumAdapter("cilium")

	for _, ad := range []GatewayAdapter{istioAd, ciliumAd} {
		if ad.Name() == "" {
			t.Error("Name() must not be empty")
		}
		if ad.GatewayClassName() == "" {
			t.Error("GatewayClassName() must not be empty")
		}
		profile := &GatewayProfile{
			Name: "iface-test", TenantID: "tenant-1", Type: GwStandard,
			Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
			Rules:     []ProfileRule{{Name: "r1", Backends: []WeightedBackend{{Name: "s", Port: 80, Weight: 100}}}},
		}
		if gw := ad.ToUnstructuredGateway(profile, "tenant-1"); gw == nil {
			t.Errorf("%s: ToUnstructuredGateway returned nil", ad.Name())
		}
		if route := ad.ToUnstructuredHTTPRoute(profile, "tenant-1"); route == nil {
			t.Errorf("%s: ToUnstructuredHTTPRoute returned nil", ad.Name())
		}
		if ns := ad.ToGatewayNamespace(profile, "tenant-1"); ns == "" {
			t.Errorf("%s: ToGatewayNamespace returned empty", ad.Name())
		}
	}
}