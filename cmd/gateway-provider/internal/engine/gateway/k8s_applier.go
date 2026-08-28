package gateway

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type K8sApplier struct {
	adapter    GatewayAdapter
	dynClient  dynamic.Interface
	gatewayGVR schema.GroupVersionResource
	routeGVR   schema.GroupVersionResource
	vsGVR      schema.GroupVersionResource
}

func NewK8sApplier(adapter GatewayAdapter, kubeconfig string) (*K8sApplier, error) {
	var cfg *rest.Config
	var err error

	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}

	return &K8sApplier{
		adapter:   adapter,
		dynClient: dynClient,
		gatewayGVR: schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways",
		},
		routeGVR: schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes",
		},
		vsGVR: schema.GroupVersionResource{
			Group: "networking.istio.io", Version: "v1beta1", Resource: "virtualservices",
		},
	}, nil
}

func (a *K8sApplier) Adapter() GatewayAdapter {
	return a.adapter
}

func (a *K8sApplier) ApplyGateway(ctx context.Context, profile *GatewayProfile, tenantID string) error {
	obj := a.adapter.ToUnstructuredGateway(profile, tenantID)
	ns := obj.GetNamespace()
	name := obj.GetName()

	existing, err := a.dynClient.Resource(a.gatewayGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("Creating Gateway %s/%s (adapter: %s)", ns, name, a.adapter.Name())
		_, err = a.dynClient.Resource(a.gatewayGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("get gateway: %w", err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	klog.V(2).Infof("Updating Gateway %s/%s", ns, name)
	_, err = a.dynClient.Resource(a.gatewayGVR).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (a *K8sApplier) ApplyHTTPRoute(ctx context.Context, profile *GatewayProfile, tenantID string) error {
	obj := a.adapter.ToUnstructuredHTTPRoute(profile, tenantID)
	ns := obj.GetNamespace()
	name := obj.GetName()

	existing, err := a.dynClient.Resource(a.routeGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("Creating HTTPRoute %s/%s", ns, name)
		_, err = a.dynClient.Resource(a.routeGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("get httproute: %w", err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	klog.V(2).Infof("Updating HTTPRoute %s/%s", ns, name)
	_, err = a.dynClient.Resource(a.routeGVR).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (a *K8sApplier) ApplyVirtualService(ctx context.Context, spec map[string]any, namespace string) error {
	obj := &unstructured.Unstructured{Object: spec}
	name := obj.GetName()

	existing, err := a.dynClient.Resource(a.vsGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("Creating VirtualService %s/%s", namespace, name)
		_, err = a.dynClient.Resource(a.vsGVR).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("get virtualservice: %w", err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	klog.V(2).Infof("Updating VirtualService %s/%s", namespace, name)
	_, err = a.dynClient.Resource(a.vsGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (a *K8sApplier) DeleteGateway(ctx context.Context, name, namespace string) error {
	err := a.dynClient.Resource(a.gatewayGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("Gateway %s/%s already deleted", namespace, name)
		return nil
	}
	return err
}

func (a *K8sApplier) DeleteHTTPRoute(ctx context.Context, name, namespace string) error {
	err := a.dynClient.Resource(a.routeGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("HTTPRoute %s/%s already deleted", namespace, name)
		return nil
	}
	return err
}

func (a *K8sApplier) DeleteVirtualService(ctx context.Context, name, namespace string) error {
	err := a.dynClient.Resource(a.vsGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		klog.V(2).Infof("VirtualService %s/%s already deleted", namespace, name)
		return nil
	}
	return err
}

func (a *K8sApplier) GetNamespace(tenantID string, profile *GatewayProfile) string {
	return a.adapter.ToGatewayNamespace(profile, tenantID)
}

func (a *K8sApplier) ListGateways(ctx context.Context, namespace string, opts metav1.ListOptions) ([]unstructured.Unstructured, error) {
	var list *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		list, err = a.dynClient.Resource(a.gatewayGVR).Namespace(namespace).List(ctx, opts)
	} else {
		list, err = a.dynClient.Resource(a.gatewayGVR).List(ctx, opts)
	}
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (a *K8sApplier) GetHTTPRoute(ctx context.Context, name, namespace string) (*unstructured.Unstructured, error) {
	return a.dynClient.Resource(a.routeGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}