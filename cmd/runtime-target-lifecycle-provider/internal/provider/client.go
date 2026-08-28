package provider

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// realKubernetesClient creates a Kubernetes client from a kubeconfig byte slice.
// Used by both KubernetesManager and EdgeManager to connect to their respective
// target clusters (Kubernetes target or CloudCore management cluster).
func realKubernetesClient(kubeconfig []byte) (kubernetes.Interface, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %w", err)
	}
	config.Timeout = 30 * time.Second
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return client, nil
}