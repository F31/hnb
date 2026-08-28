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

// KubeOVNTenantIsolation implements TenantIsolationManager for Kube-OVN.
//
// Kube-OVN does not have a cluster-wide policy CRD equivalent to
// CiliumClusterwideNetworkPolicy or Calico GlobalNetworkPolicy.
// Its primary isolation mechanisms are:
//   - L1: Standard K8s NetworkPolicy per namespace (default-deny + intra-tenant allow)
//   - L2 (optional): Kube-OVN subnet ACL (requires subnet-per-tenant, not automated here)
//
// For production multi-tenant isolation on shared clusters, Cilium or Calico
// are strongly recommended. Kube-OVN's L1 policies provide namespace-level
// isolation but cannot prevent cross-tenant pod-to-pod traffic at the cluster level.
type KubeOVNTenantIsolation struct {
	newClient func(target *RuntimeTarget) (dynamic.Interface, error)
}

func NewKubeOVNTenantIsolation() *KubeOVNTenantIsolation {
	return &KubeOVNTenantIsolation{newClient: newKubeOVNDynamicClient}
}

func (k *KubeOVNTenantIsolation) ApplyTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace, tenantID, workspaceID string) error {
	client, err := k.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "namespaces",
	}
	nsObj, err := client.Resource(gvr).Get(ctx, k8sNamespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", k8sNamespace, err)
	}

	labels := nsObj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["hnb.io/tenant"] = tenantID
	labels["hnb.io/workspace"] = workspaceID
	nsObj.SetLabels(labels)
	_, err = client.Resource(gvr).Update(ctx, nsObj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("label namespace %s: %w", k8sNamespace, err)
	}

	netGVR := schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "networkpolicies",
	}
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
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
				"podSelector": map[string]any{},
				"policyTypes": []any{"Ingress", "Egress"},
				"ingress": []any{
					map[string]any{
						"from": []any{
							map[string]any{
								"namespaceSelector": map[string]any{
									"matchLabels": map[string]any{
										"hnb.io/tenant":    tenantID,
										"hnb.io/workspace": workspaceID,
									},
								},
							},
						},
					},
				},
				"egress": []any{
					map[string]any{
						"to": []any{
							map[string]any{
								"namespaceSelector": map[string]any{
									"matchLabels": map[string]any{
										"hnb.io/tenant":    tenantID,
										"hnb.io/workspace": workspaceID,
									},
								},
							},
						},
					},
					map[string]any{
						"to": []any{
							map[string]any{
								"ipBlock": map[string]any{
									"cidr": "0.0.0.0/0",
									"except": []any{
										"10.0.0.0/8",
										"172.16.0.0/12",
										"192.168.0.0/16",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return applyKubeOVNPolicy(ctx, client, netGVR, policy)
}

func (k *KubeOVNTenantIsolation) RemoveTenantIsolation(ctx context.Context, target *RuntimeTarget, k8sNamespace string) error {
	client, err := k.newClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}
	gvr := schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "networkpolicies",
	}
	return client.Resource(gvr).Namespace(k8sNamespace).Delete(ctx, "hnb-tenant-default-deny", metav1.DeleteOptions{})
}

func (k *KubeOVNTenantIsolation) ApplyCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	klog.Warningf("[kube-ovn] cross-tenant deny not supported at cluster level; L2 isolation requires subnet-level ACL (not automated)")
	return nil
}

func (k *KubeOVNTenantIsolation) RemoveCrossTenantDeny(ctx context.Context, target *RuntimeTarget, tenantID, workspaceID string) error {
	klog.Warningf("[kube-ovn] cross-tenant deny removal not applicable")
	return nil
}

func applyKubeOVNPolicy(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, policy *unstructured.Unstructured) error {
	ns := policy.GetNamespace()
	name := policy.GetName()
	existing, err := client.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		policy.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Resource(gvr).Namespace(ns).Update(ctx, policy, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update K8s NP %s/%s: %w", ns, name, err)
		}
		klog.Infof("[kube-ovn-tenant] updated policy %s/%s", ns, name)
		return nil
	}
	_, err = client.Resource(gvr).Namespace(ns).Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create K8s NP %s/%s: %w", ns, name, err)
	}
	klog.Infof("[kube-ovn-tenant] created policy %s/%s", ns, name)
	return nil
}

func newKubeOVNDynamicClient(target *RuntimeTarget) (dynamic.Interface, error) {
	restConfig, err := buildRestConfig(target)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restConfig)
}