package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestRouteBindingFailsWhenHTTPRouteCRDIsMissing(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	client.PrependReactor("create", "httproutes", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "probe-app")
	})
	err := NewRouteBindingApplier(client, nil).ApplyAndProbe(context.Background(), RouteBinding{
		Namespace: "probes", Name: "probe-app", GatewayName: "gw", GatewayNamespace: "gateway",
		Hostname: "app.example.test", ServiceName: "probe-server", ServicePort: 8080,
		ExternalURL: "http://127.0.0.1:1/", Timeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "CRD is not installed") {
		t.Fatalf("missing CRD error = %v", err)
	}
}

func TestRouteConditionParsing(t *testing.T) {
	route := unstructuredWithConditions("Route", []any{
		map[string]any{"type": "Accepted", "status": "True", "reason": "Accepted"},
		map[string]any{"type": "ResolvedRefs", "status": "False", "reason": "BackendNotFound", "message": "missing"},
	})
	if accepted, _ := conditionTrue(route, "Accepted"); !accepted {
		t.Fatal("Accepted was not parsed")
	}
	if resolved, reason := conditionTrue(route, "ResolvedRefs"); resolved || !strings.Contains(reason, "BackendNotFound") {
		t.Fatalf("ResolvedRefs = %v %q", resolved, reason)
	}
}

func unstructuredWithConditions(kind string, conditions []any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       kind,
		"status": map[string]any{
			"parents":    []any{map[string]any{"conditions": conditions}},
			"conditions": conditions,
		},
	}}
}
