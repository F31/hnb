package network

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type CiliumTenantIsolation struct {
	newClient func(target *RuntimeTarget) (dynamic.Interface, error)
}

func NewCiliumTenantIsolation() *CiliumTenantIsolation {
	return &CiliumTenantIsolation{newClient: newCiliumDynamicClient}
}

func (c *CiliumTenantIsolation) ApplyTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "cilium.io", Version: "v2", Resource: "ciliumnetworkpolicies",
	}
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumNetworkPolicy",
			"metadata": map[string]any{
				"name":      "hnb-tenant-default-deny",
				"namespace": k8sNamespace,
				"labels": map[string]any{
					"hnb.io/tenant":      tenantID,
					"hnb.io/workspace":   workspaceID,
					"hnb.io/managed-by":  "hnb-platform",
					"hnb.io/policy-type": "default-deny",
				},
			},
			"spec": map[string]any{
				"endpointSelector": map[string]any{},
				"ingress": []any{
					map[string]any{
						"fromEndpoints": []any{
							map[string]any{
								"matchLabels": map[string]any{
									"hnb.io/tenant":    tenantID,
									"hnb.io/workspace": workspaceID,
								},
							},
						},
					},
				},
				"egress": []any{
					map[string]any{
						"toEntities": []any{
							"cluster", "host", "init", "remote-node",
						},
					},
					map[string]any{
						"toEndpoints": []any{
							map[string]any{
								"matchLabels": map[string]any{
									"hnb.io/tenant":    tenantID,
									"hnb.io/workspace": workspaceID,
								},
							},
						},
					},
				},
			},
		},
	}
	return applyCNP(ctx, client, gvr, policy)
}

func (c *CiliumTenantIsolation) RemoveTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "cilium.io", Version: "v2", Resource: "ciliumnetworkpolicies",
	}
	return client.Resource(gvr).Namespace(k8sNamespace).Delete(ctx, "hnb-tenant-default-deny", metav1.DeleteOptions{})
}

func (c *CiliumTenantIsolation) ApplyCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "cilium.io", Version: "v2", Resource: "ciliumclusterwidepolicies",
	}
	name := fmt.Sprintf("hnb-cross-tenant-deny-%s-%s", tenantID[:8], workspaceID[:8])
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumClusterwideNetworkPolicy",
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"hnb.io/tenant":      tenantID,
					"hnb.io/workspace":   workspaceID,
					"hnb.io/managed-by":  "hnb-platform",
					"hnb.io/policy-type": "cross-tenant-deny",
				},
			},
			"spec": map[string]any{
				"endpointSelector": map[string]any{
					"matchLabels": map[string]any{
						"hnb.io/tenant":    tenantID,
						"hnb.io/workspace": workspaceID,
					},
				},
				"ingressDeny": []any{
					map[string]any{
						"fromEndpoints": []any{
							map[string]any{
								"matchLabels": map[string]any{
									"hnb.io/tenant": "{}",
								},
							},
						},
					},
				},
			},
		},
	}
	return applyCCNP(ctx, client, gvr, policy)
}

func (c *CiliumTenantIsolation) RemoveCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "cilium.io", Version: "v2", Resource: "ciliumclusterwidepolicies",
	}
	name := fmt.Sprintf("hnb-cross-tenant-deny-%s-%s", tenantID[:8], workspaceID[:8])
	return client.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

func applyCNP(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, policy *unstructured.Unstructured) error {
	ns := policy.GetNamespace()
	name := policy.GetName()
	existing, err := client.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		policy.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Namespace(ns).Update(ctx, policy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update CNP %s/%s: %w", ns, name, err)
		}
		klog.Infof("[cilium-tenant] updated policy %s/%s", ns, name)
		return nil
	}
	_, err = client.Resource(gvr).Namespace(ns).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create CNP %s/%s: %w", ns, name, err)
	}
	klog.Infof("[cilium-tenant] created policy %s/%s", ns, name)
	return nil
}

func applyCCNP(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, policy *unstructured.Unstructured) error {
	name := policy.GetName()
	existing, err := client.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		policy.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Update(ctx, policy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update CCNP %s: %w", name, err)
		}
		klog.Infof("[cilium-tenant] updated CCNP %s", name)
		return nil
	}
	_, err = client.Resource(gvr).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create CCNP %s: %w", name, err)
	}
	klog.Infof("[cilium-tenant] created CCNP %s", name)
	return nil
}

func newCiliumDynamicClient(target *RuntimeTarget) (dynamic.Interface, error) {
	restConfig, err := buildRestConfig(target)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restConfig)
}

func buildRestConfig(target *RuntimeTarget) (*rest.Config, error) {
	if target.Kubeconfig != "" {
		return clientcmd.RESTConfigFromKubeConfig([]byte(target.Kubeconfig))
	}
	if target.APIServerURL != "" {
		config := &rest.Config{Host: target.APIServerURL}
		return config, nil
	}
	return rest.InClusterConfig()
}