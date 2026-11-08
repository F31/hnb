package provider

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func serviceDeleteExecution(generation int64, expectedUID string) ExecutionContext {
	execution := testExecution("service.delete", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo-svc", "expected_uid": expectedUID,
	})
	return execution
}

func TestServiceDeleteRemovesManagedService(t *testing.T) {
	stored := serviceExecution(1, "8080")
	client := k8sfake.NewClientset(managedService(stored))
	execution := serviceDeleteExecution(2, "service-uid")
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "service.deleted" || result.Outputs["uid"] != "service-uid" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := client.CoreV1().Services("hnb-e2e").Get(context.Background(), "demo-svc", metav1.GetOptions{}); err == nil {
		t.Fatal("service still exists after delete")
	}
}

func TestServiceDeleteMissingIsReplay(t *testing.T) {
	client := k8sfake.NewClientset()
	execution := serviceDeleteExecution(2, "")
	result, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "service.deleted" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceDeleteRejectsUIDMismatch(t *testing.T) {
	stored := serviceExecution(1, "8080")
	client := k8sfake.NewClientset(managedService(stored))
	execution := serviceDeleteExecution(2, "other-uid")
	if _, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "expected_uid") {
		t.Fatalf("err = %v", err)
	}
	if _, err := client.CoreV1().Services("hnb-e2e").Get(context.Background(), "demo-svc", metav1.GetOptions{}); err != nil {
		t.Fatal("service must survive a rejected delete")
	}
}

func TestServiceDeleteRejectsForeignService(t *testing.T) {
	foreign := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "hnb-e2e", Name: "demo-svc", UID: types.UID("service-uid")},
	}
	client := k8sfake.NewClientset(foreign)
	execution := serviceDeleteExecution(2, "")
	if _, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "not owned") {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceDeleteRejectsNewerFencingGeneration(t *testing.T) {
	stored := serviceExecution(3, "8080")
	client := k8sfake.NewClientset(managedService(stored))
	execution := serviceDeleteExecution(2, "")
	if _, err := NewExecutor(client, allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "fenc") {
		t.Fatalf("err = %v", err)
	}
}

func httpRoute(namespace, name string, annotations map[string]string) *unstructured.Unstructured {
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]any{
			"namespace": namespace, "name": name, "uid": "route-uid",
		},
	}}
	if annotations != nil {
		route.SetAnnotations(annotations)
	}
	return route
}

func httpRouteDeleteExecution(generation int64, expectedUID string) ExecutionContext {
	execution := testExecution("httproute.delete", generation)
	execution.Inputs = stringMapToAny(map[string]string{
		"namespace": "hnb-e2e", "name": "demo-route", "expected_uid": expectedUID,
	})
	return execution
}

func newDynamicFake(objects ...runtime.Object) *fake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return fake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{httpRouteGVR: "HTTPRouteList"}, objects...)
}

func TestHTTPRouteDeleteRemovesRoute(t *testing.T) {
	dyn := newDynamicFake(httpRoute("hnb-e2e", "demo-route", nil))
	execution := httpRouteDeleteExecution(1, "route-uid")
	result, err := NewExecutorWithDynamic(k8sfake.NewClientset(), dyn, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "httproute.deleted" || result.Outputs["uid"] != "route-uid" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := dyn.Resource(httpRouteGVR).Namespace("hnb-e2e").Get(context.Background(), "demo-route", metav1.GetOptions{}); err == nil {
		t.Fatal("httproute still exists after delete")
	}
}

func TestHTTPRouteDeleteMissingIsReplay(t *testing.T) {
	dyn := newDynamicFake()
	execution := httpRouteDeleteExecution(1, "")
	result, err := NewExecutorWithDynamic(k8sfake.NewClientset(), dyn, allowedNamespaces(), 10).Execute(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs["action"] != "httproute.deleted" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPRouteDeleteRejectsUIDMismatch(t *testing.T) {
	dyn := newDynamicFake(httpRoute("hnb-e2e", "demo-route", nil))
	execution := httpRouteDeleteExecution(1, "other-uid")
	if _, err := NewExecutorWithDynamic(k8sfake.NewClientset(), dyn, allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "expected_uid") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPRouteDeleteRejectsForeignManagedRoute(t *testing.T) {
	dyn := newDynamicFake(httpRoute("hnb-e2e", "demo-route", map[string]string{
		managedByAnnotation: "other-provider",
	}))
	execution := httpRouteDeleteExecution(1, "")
	if _, err := NewExecutorWithDynamic(k8sfake.NewClientset(), dyn, allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "another provider") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPRouteDeleteRequiresDynamicClient(t *testing.T) {
	execution := httpRouteDeleteExecution(1, "")
	if _, err := NewExecutor(k8sfake.NewClientset(), allowedNamespaces(), 10).Execute(context.Background(), execution); err == nil ||
		!strings.Contains(err.Error(), "dynamic client") {
		t.Fatalf("err = %v", err)
	}
}
