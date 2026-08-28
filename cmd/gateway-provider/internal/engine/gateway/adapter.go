package gateway

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type GatewayAdapter interface {
	Name() string
	GatewayClassName() string
	ToUnstructuredGateway(profile *GatewayProfile, tenantID string) *unstructured.Unstructured
	ToUnstructuredHTTPRoute(profile *GatewayProfile, tenantID string) *unstructured.Unstructured
	ToGatewayNamespace(profile *GatewayProfile, tenantID string) string
}

type VirtualServiceProvider interface {
	ToVirtualService(profile *GatewayProfile, tenantID string) map[string]any
}

type GenericAdapter struct {
	name             string
	gatewayClassName string
	renderer         *GatewayRenderer
}

func NewGenericAdapter(name, gatewayClassName string, extraLabels, extraAnnotations map[string]string) *GenericAdapter {
	if gatewayClassName == "" {
		gatewayClassName = name
	}
	return &GenericAdapter{
		name:             name,
		gatewayClassName: gatewayClassName,
		renderer:         NewGatewayRenderer(gatewayClassName, extraLabels, extraAnnotations),
	}
}

func (a *GenericAdapter) Name() string {
	return a.name
}

func (a *GenericAdapter) GatewayClassName() string {
	return a.gatewayClassName
}

func (a *GenericAdapter) ToUnstructuredGateway(profile *GatewayProfile, tenantID string) *unstructured.Unstructured {
	return a.renderer.RenderGateway(profile, tenantID)
}

func (a *GenericAdapter) ToUnstructuredHTTPRoute(profile *GatewayProfile, tenantID string) *unstructured.Unstructured {
	return a.renderer.RenderHTTPRoute(profile, tenantID)
}

func (a *GenericAdapter) ToGatewayNamespace(profile *GatewayProfile, tenantID string) string {
	workspace := profile.WorkspaceID
	if workspace == "" {
		workspace = tenantID
	}
	return "hnb-" + workspace
}