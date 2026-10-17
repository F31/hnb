package provision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ManagementClient applies CAPI manifests to a management cluster and waits
// for the workload cluster control plane to become ready. Split behind an
// interface so the orchestrator is testable with a fake management client.
type ManagementClient interface {
	// Apply upserts the given unstructured objects into the management cluster.
	Apply(ctx context.Context, objects []unstructured.Unstructured) error
	// WaitControlPlaneReady blocks until the KubeadmControlPlane reports
	// Ready=true (or the timeout elapses).
	WaitControlPlaneReady(ctx context.Context, namespace, name string, timeout time.Duration) error
	// Kubeconfig returns the workload cluster's admin kubeconfig from the CAPI
	// Cluster object (Status.KubeconfigSecretRef).
	Kubeconfig(ctx context.Context, namespace, clusterName string) ([]byte, error)
}

// realManagementClient drives a real management cluster via client-go dynamic.
type realManagementClient struct {
	client dynamic.Interface
	// readKubeconfig reads a secret by name from the management cluster
	// namespace and returns its kubeconfig bytes.
	readKubeconfig func(ctx context.Context, namespace, name string) ([]byte, error)
}

// NewManagementClient builds a management client from a kubeconfig path/ctx.
// Set runtimeContext to load a specific kube context, or "" for the current.
func NewManagementClient(managementKubeconfig, runtimeContext string) (ManagementClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if managementKubeconfig != "" {
		loadingRules.ExplicitPath = managementKubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if runtimeContext != "" {
		overrides.CurrentContext = runtimeContext
	}
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restCfg, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load management cluster kubeconfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	return &realManagementClient{
		client:         dyn,
		readKubeconfig: secretKubeconfigReader(restCfg),
	}, nil
}

func (m *realManagementClient) Apply(_ context.Context, objects []unstructured.Unstructured) error {
	for i := range objects {
		obj := &objects[i]
		gvk := obj.GroupVersionKind()
		ri := m.client.Resource(gvrFor(gvk))
		ns := obj.GetNamespace()
		if ns == "" {
			ns = metav1.NamespaceDefault
		}
		data, _ := json.Marshal(obj.Object)
		_, err := ri.Namespace(ns).Patch(
			context.Background(),
			obj.GetName(),
			types.ApplyPatchType,
			data,
			metav1.PatchOptions{FieldManager: "hnb-runtime-target-lifecycle-provider", Force: boolp(true)},
		)
		if err != nil {
			return fmt.Errorf("apply %s/%s: %w", gvk.Kind, obj.GetName(), err)
		}
	}
	return nil
}

func (m *realManagementClient) WaitControlPlaneReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	gvr := gvrFor(schema.GroupVersionKind{Group: clusterAPIGroup, Version: clusterAPIVersion, Kind: "KubeadmControlPlane"})
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		obj, err := m.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		ready := meta.IsStatusConditionTrue(conditionsOf(obj), "Ready")
		return ready, nil
	})
}

func (m *realManagementClient) Kubeconfig(ctx context.Context, namespace, clusterName string) ([]byte, error) {
	clusterGVR := gvrFor(clusterGVK)
	obj, err := m.client.Resource(clusterGVR).Namespace(namespace).Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get cluster %s: %w", clusterName, err)
	}
	status, ok := obj.Object["status"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cluster %s has no status", clusterName)
	}
	ref, ok := status["kubeconfigSecretRef"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cluster %s has no kubeconfig secret", clusterName)
	}
	secretName, _ := ref["name"].(string)
	if secretName == "" {
		return nil, fmt.Errorf("cluster %s kubeconfig secret ref has no name", clusterName)
	}
	if m.readKubeconfig == nil {
		return nil, fmt.Errorf("kubeconfig reader not configured")
	}
	return m.readKubeconfig(ctx, namespace, secretName)
}

// gvrFor maps a GVK to a GVR (all CAPI resources we render are cluster-scoped
// or namespaced; the mapping is mechanical).
func gvrFor(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	return gvk.GroupVersion().WithResource(pluralResource(gvk.Kind))
}

func pluralResource(kind string) string {
	// Pluralization for the kinds we render; generic fallback "s".
	switch kind {
	case "Cluster":
		return "clusters"
	case "KubeadmControlPlane":
		return "kubeadmcontrolplanes"
	case "KubeadmConfigTemplate":
		return "kubeadmconfigtemplates"
	case "MachineDeployment":
		return "machinedeployments"
	case "SSHCluster":
		return "sshclusters"
	case "SSHMachineTemplate":
		return "sshmachinetemplates"
	case "SSHMachineDeployment":
		return "sshmachinedeployments"
	}
	return kind + "s"
}

// conditionsOf returns the status.conditions array from an unstructured obj.
func conditionsOf(obj *unstructured.Unstructured) []metav1.Condition {
	status, ok := obj.Object["status"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := status["conditions"].([]any)
	if !ok {
		return nil
	}
	var out []metav1.Condition
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		statusTxt, _ := m["status"].(string)
		typ, _ := m["type"].(string)
		out = append(out, metav1.Condition{Type: typ, Status: metav1.ConditionStatus(statusTxt)})
	}
	return out
}

// secretKubeconfigReader returns a function that reads a Secret's
// data.kubeconfig (or data["value"]) via the discovery REST client.
func secretKubeconfigReader(cfg *rest.Config) func(ctx context.Context, namespace, name string) ([]byte, error) {
	corev1 := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	dyn, err := dynamic.NewForConfig(cfg)
	return func(ctx context.Context, namespace, name string) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		obj, err := dyn.Resource(corev1).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get kubeconfig secret %s: %w", name, err)
		}
		data, ok := obj.Object["data"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("secret %s has no data", name)
		}
		for _, key := range []string{"kubeconfig", "value"} {
			raw, ok := data[key].(string)
			if ok && raw != "" {
				return decodeBase64Kubeconfig(raw)
			}
		}
		return nil, fmt.Errorf("secret %s has no kubeconfig key", name)
	}
}

func decodeBase64Kubeconfig(encoded string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
}

func boolp(v bool) *bool { return &v }
