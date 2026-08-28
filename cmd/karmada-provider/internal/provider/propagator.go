package provider

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type Propagator struct {
	dynClient  dynamic.Interface
	kubeClient kubernetes.Interface
}

func NewPropagator(kubeconfig string) (*Propagator, error) {
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
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube client: %w", err)
	}

	return &Propagator{dynClient: dynClient, kubeClient: kubeClient}, nil
}

func (p *Propagator) ApplyPropagationPolicy(ctx context.Context, name, namespace string, clusterLabels map[string]string, placement string) error {
	policy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "policy.karmada.io/v1alpha1",
			"kind":       "PropagationPolicy",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"resourceSelectors": []any{
					map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"namespace":  namespace,
						"name":       name,
					},
				},
				"placement": map[string]any{
					"clusterAffinity": map[string]any{
						"labelSelector": map[string]any{
							"matchLabels": clusterLabels,
						},
					},
					"clusterTolerations": []any{},
					"replicaScheduling": map[string]any{
						"replicaDivisionPreference": placement,
					},
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{
		Group:    "policy.karmada.io",
		Version:  "v1alpha1",
		Resource: "propagationpolicies",
	}

	existing, err := p.dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.V(2).Infof("Creating PropagationPolicy %s/%s", namespace, name)
		_, err = p.dynClient.Resource(gvr).Namespace(namespace).Create(ctx, policy, metav1.CreateOptions{})
	} else {
		policy.SetResourceVersion(existing.GetResourceVersion())
		klog.V(2).Infof("Updating PropagationPolicy %s/%s", namespace, name)
		_, err = p.dynClient.Resource(gvr).Namespace(namespace).Update(ctx, policy, metav1.UpdateOptions{})
	}
	return err
}

func (p *Propagator) DeletePropagationPolicy(ctx context.Context, name, namespace string) error {
	gvr := schema.GroupVersionResource{
		Group:    "policy.karmada.io",
		Version:  "v1alpha1",
		Resource: "propagationpolicies",
	}
	return p.dynClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (p *Propagator) ListMemberClusters(ctx context.Context) ([]map[string]any, error) {
	gvr := schema.GroupVersionResource{
		Group:    "cluster.karmada.io",
		Version:  "v1alpha1",
		Resource: "clusters",
	}
	list, err := p.dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list karmada clusters: %w", err)
	}

	result := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		cluster := map[string]any{
			"name":   item.GetName(),
			"labels": item.GetLabels(),
		}
		status, ok, _ := unstructured.NestedMap(item.Object, "status")
		if ok {
			cluster["status"] = status
		}
		result = append(result, cluster)
	}
	return result, nil
}

func (p *Propagator) GetClusterHealth(ctx context.Context, name string) (string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "cluster.karmada.io",
		Version:  "v1alpha1",
		Resource: "clusters",
	}
	cluster, err := p.dynClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "unreachable", err
	}
	conditions, ok, _ := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	if !ok {
		return "unknown", nil
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return "healthy", nil
		}
	}
	return "degraded", nil
}