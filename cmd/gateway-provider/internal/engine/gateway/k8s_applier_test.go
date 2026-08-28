package gateway

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newTestApplier() *K8sApplier {
	adapter := NewIstioAdapter("istio")
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	return &K8sApplier{
		adapter:   adapter,
		dynClient: fakeClient,
		gatewayGVR: schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways",
		},
		routeGVR: schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes",
		},
		vsGVR: schema.GroupVersionResource{
			Group: "networking.istio.io", Version: "v1beta1", Resource: "virtualservices",
		},
	}
}

func profile(name string) *GatewayProfile {
	return &GatewayProfile{
		ID: "test-id", Name: name, TenantID: "tenant-1", Type: GwStandard,
		Listeners: []Listener{{Name: "http", Port: 80, Protocol: "HTTP"}},
		Rules: []ProfileRule{
			{Name: "rule-1", Backends: []WeightedBackend{{Name: "svc-a", Port: 8080, Weight: 100}}},
		},
	}
}

func TestK8sApplier_ApplyGateway(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	p := profile("apply-gw-test")

	if err := a.ApplyGateway(ctx, p, "tenant-1"); err != nil {
		t.Fatalf("ApplyGateway failed: %v", err)
	}

	ns := a.adapter.ToGatewayNamespace(p, "tenant-1")
	obj, err := a.dynClient.Resource(a.gatewayGVR).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get gateway after create: %v", err)
	}
	if obj.GetName() != p.Name {
		t.Errorf("expected name %s, got %s", p.Name, obj.GetName())
	}
}

func TestK8sApplier_ApplyGateway_Update(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	p := profile("apply-gw-update")

	if err := a.ApplyGateway(ctx, p, "tenant-1"); err != nil {
		t.Fatalf("first ApplyGateway failed: %v", err)
	}

	p.Listeners = []Listener{{Name: "https", Port: 443, Protocol: "HTTPS"}}
	if err := a.ApplyGateway(ctx, p, "tenant-1"); err != nil {
		t.Fatalf("second ApplyGateway (update) failed: %v", err)
	}
}

func TestK8sApplier_ApplyHTTPRoute(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	p := profile("apply-route-test")

	if err := a.ApplyHTTPRoute(ctx, p, "tenant-1"); err != nil {
		t.Fatalf("ApplyHTTPRoute failed: %v", err)
	}

	ns := a.adapter.ToGatewayNamespace(p, "tenant-1")
	name := p.Name + "-httproute"
	obj, err := a.dynClient.Resource(a.routeGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get httproute after create: %v", err)
	}
	if obj.GetName() != name {
		t.Errorf("expected name %s, got %s", name, obj.GetName())
	}
}

func TestK8sApplier_DeleteGateway(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	p := profile("del-gw-test")
	ns := a.adapter.ToGatewayNamespace(p, "tenant-1")

	a.ApplyGateway(ctx, p, "tenant-1")
	if err := a.DeleteGateway(ctx, p.Name, ns); err != nil {
		t.Fatalf("DeleteGateway failed: %v", err)
	}

	_, err := a.dynClient.Resource(a.gatewayGVR).Namespace(ns).Get(ctx, p.Name, metav1.GetOptions{})
	if err == nil {
		t.Error("expected NotFound after delete")
	}
}

func TestK8sApplier_DeleteGateway_NotFound(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()

	if err := a.DeleteGateway(ctx, "nonexistent", "hnb-tenant-1"); err != nil {
		t.Fatalf("DeleteGateway on nonexistent should succeed: %v", err)
	}
}

func TestK8sApplier_DeleteHTTPRoute(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	p := profile("del-route-test")
	ns := a.adapter.ToGatewayNamespace(p, "tenant-1")

	a.ApplyHTTPRoute(ctx, p, "tenant-1")
	if err := a.DeleteHTTPRoute(ctx, p.Name+"-httproute", ns); err != nil {
		t.Fatalf("DeleteHTTPRoute failed: %v", err)
	}

	_, err := a.dynClient.Resource(a.routeGVR).Namespace(ns).Get(ctx, p.Name+"-httproute", metav1.GetOptions{})
	if err == nil {
		t.Error("expected NotFound after delete")
	}
}

func TestK8sApplier_DeleteHTTPRoute_NotFound(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()

	if err := a.DeleteHTTPRoute(ctx, "nonexistent", "hnb-tenant-1"); err != nil {
		t.Fatalf("DeleteHTTPRoute on nonexistent should succeed: %v", err)
	}
}

func TestK8sApplier_GetNamespace(t *testing.T) {
	a := newTestApplier()
	p := profile("ns-test")
	ns := a.GetNamespace("tenant-1", p)
	if ns != "hnb-tenant-1" {
		t.Errorf("expected hnb-tenant-1, got %s", ns)
	}
}