package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gatewayGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	routeGVR   = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
)

type RouteBinding struct {
	Namespace        string
	Name             string
	GatewayName      string
	GatewayNamespace string
	Hostname         string
	PathPrefix       string
	ServiceName      string
	ServicePort      int32
	ExternalURL      string
	HostHeader       string
	ExpectedBody     string
	Timeout          time.Duration
}

type RouteBindingApplier struct {
	client dynamic.Interface
	http   *http.Client
}

func NewRouteBindingApplier(client dynamic.Interface, httpClient *http.Client) *RouteBindingApplier {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &RouteBindingApplier{client: client, http: httpClient}
}

// ApplyAndProbe only succeeds after the route is accepted/resolved, the
// referenced Gateway is Programmed, and the external probe returns the
// expected response. Missing CRDs/controllers and unreachable entries fail
// closed.
func (a *RouteBindingApplier) ApplyAndProbe(ctx context.Context, binding RouteBinding) error {
	if binding.Namespace == "" || binding.Name == "" || binding.GatewayName == "" || binding.ServiceName == "" || binding.ServicePort < 1 {
		return fmt.Errorf("route binding fields are required")
	}
	timeout := binding.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	path := binding.PathPrefix
	if path == "" {
		path = "/"
	}
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]any{
			"name":      binding.Name,
			"namespace": binding.Namespace,
			"labels":    map[string]any{"hnb.io/managed-by": "hnb-gateway-provider"},
		},
		"spec": map[string]any{
			"parentRefs": []any{map[string]any{"name": binding.GatewayName, "namespace": binding.GatewayNamespace}},
			"hostnames":  []any{binding.Hostname},
			"rules": []any{map[string]any{
				"matches":     []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": path}}},
				"backendRefs": []any{map[string]any{"name": binding.ServiceName, "port": int64(binding.ServicePort)}},
			}},
		},
	}}
	routes := a.client.Resource(routeGVR).Namespace(binding.Namespace)
	existing, err := routes.Get(ctx, binding.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := routes.Create(ctx, route, metav1.CreateOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("Gateway API HTTPRoute CRD is not installed")
			}
			return fmt.Errorf("create HTTPRoute: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get HTTPRoute: %w", err)
	} else {
		route.SetResourceVersion(existing.GetResourceVersion())
		if _, err := routes.Update(ctx, route, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update HTTPRoute: %w", err)
		}
	}

	deadline := time.Now().Add(timeout)
	var lastReason string
	for {
		current, err := routes.Get(ctx, binding.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read HTTPRoute status: %w", err)
		}
		accepted, acceptedReason := conditionTrue(current, "Accepted")
		resolved, resolvedReason := conditionTrue(current, "ResolvedRefs")
		if acceptedReason != "" {
			lastReason = acceptedReason
		}
		if resolvedReason != "" {
			lastReason = resolvedReason
		}
		if accepted && resolved {
			gateway, err := a.client.Resource(gatewayGVR).Namespace(binding.GatewayNamespace).Get(ctx, binding.GatewayName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("read Gateway status: %w", err)
			}
			programmed, _ := conditionTrue(gateway, "Programmed")
			if programmed {
				break
			}
			lastReason = "Gateway is not Programmed"
		}
		if time.Now().After(deadline) {
			if lastReason == "" {
				lastReason = "HTTPRoute was not accepted/resolved"
			}
			return fmt.Errorf("route not effective before timeout: %s", lastReason)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	if binding.ExternalURL == "" {
		return fmt.Errorf("external probe URL is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, binding.ExternalURL, nil)
	if err != nil {
		return fmt.Errorf("build external probe: %w", err)
	}
	if binding.HostHeader != "" {
		request.Host = binding.HostHeader
	}
	response, err := a.http.Do(request)
	if err != nil {
		return fmt.Errorf("external entry probe failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("external entry returned status %d", response.StatusCode)
	}
	if binding.ExpectedBody != "" && !strings.Contains(string(body), binding.ExpectedBody) {
		return fmt.Errorf("external entry body did not contain the expected marker")
	}
	return nil
}

func conditionTrue(object *unstructured.Unstructured, conditionType string) (bool, string) {
	parents, _, _ := unstructured.NestedSlice(object.Object, "status", "parents")
	if len(parents) > 0 {
		if parent, ok := parents[0].(map[string]any); ok {
			if conditions, ok := parent["conditions"].([]any); ok {
				for _, raw := range conditions {
					condition, _ := raw.(map[string]any)
					if condition["type"] == conditionType {
						return condition["status"] == "True", stringValue(condition["reason"]) + " " + stringValue(condition["message"])
					}
				}
			}
		}
	}
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, _ := raw.(map[string]any)
		if condition["type"] == conditionType {
			return condition["status"] == "True", stringValue(condition["reason"]) + " " + stringValue(condition["message"])
		}
	}
	return false, ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
