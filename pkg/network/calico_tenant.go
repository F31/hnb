package network

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

type CalicoTenantIsolation struct {
	newClient func(target *RuntimeTarget) (dynamic.Interface, error)
}

func NewCalicoTenantIsolation() *CalicoTenantIsolation {
	return &CalicoTenantIsolation{newClient: newCalicoDynamicClient}
}

func (c *CalicoTenantIsolation) ApplyTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "projectcalico.org", Version: "v3", Resource: "networkpolicies",
	}
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "projectcalico.org/v3",
			"kind":       "NetworkPolicy",
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
				"selector": "all()",
				"ingress": []any{
					map[string]any{
						"action": "Allow",
						"source": map[string]any{
							"selector": fmt.Sprintf("hnb.io/tenant == '%s' && hnb.io/workspace == '%s'", tenantID, workspaceID),
						},
					},
				},
				"egress": []any{
					map[string]any{
						"action": "Allow",
						"destination": map[string]any{
							"selector": fmt.Sprintf("hnb.io/tenant == '%s' && hnb.io/workspace == '%s'", tenantID, workspaceID),
						},
					},
				},
			},
		},
	}
	return applyCalicoNP(ctx, client, gvr, policy)
}

func (c *CalicoTenantIsolation) RemoveTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "projectcalico.org", Version: "v3", Resource: "networkpolicies",
	}
	return client.Resource(gvr).Namespace(k8sNamespace).Delete(ctx, "hnb-tenant-default-deny", metav1.DeleteOptions{})
}

func (c *CalicoTenantIsolation) ApplyCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "projectcalico.org", Version: "v3", Resource: "globalnetworkpolicies",
	}
	name := fmt.Sprintf("hnb-cross-tenant-deny-%s-%s", tenantID[:8], workspaceID[:8])
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "projectcalico.org/v3",
			"kind":       "GlobalNetworkPolicy",
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
				"selector": fmt.Sprintf("hnb.io/tenant == '%s'", tenantID),
				"ingress": []any{
					map[string]any{
						"action":  "Deny",
						"source": map[string]any{
							"selector": "hnb.io/tenant != ''",
						},
					},
				},
			},
		},
	}
	return applyCalicoGNP(ctx, client, gvr, policy)
}

func (c *CalicoTenantIsolation) RemoveCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	client, err := c.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "projectcalico.org", Version: "v3", Resource: "globalnetworkpolicies",
	}
	name := fmt.Sprintf("hnb-cross-tenant-deny-%s-%s", tenantID[:8], workspaceID[:8])
	return client.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

func applyCalicoNP(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, policy *unstructured.Unstructured) error {
	ns := policy.GetNamespace()
	name := policy.GetName()
	existing, err := client.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		policy.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Namespace(ns).Update(ctx, policy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update Calico NP %s/%s: %w", ns, name, err)
		}
		klog.Infof("[calico-tenant] updated policy %s/%s", ns, name)
		return nil
	}
	_, err = client.Resource(gvr).Namespace(ns).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create Calico NP %s/%s: %w", ns, name, err)
	}
	klog.Infof("[calico-tenant] created policy %s/%s", ns, name)
	return nil
}

func applyCalicoGNP(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, policy *unstructured.Unstructured) error {
	name := policy.GetName()
	existing, err := client.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		policy.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Update(ctx, policy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update Calico GNP %s: %w", name, err)
		}
		klog.Infof("[calico-tenant] updated GNP %s", name)
		return nil
	}
	_, err = client.Resource(gvr).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create Calico GNP %s: %w", name, err)
	}
	klog.Infof("[calico-tenant] created GNP %s", name)
	return nil
}

func newCalicoDynamicClient(target *RuntimeTarget) (dynamic.Interface, error) {
	restConfig, err := buildRestConfig(target)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restConfig)
}