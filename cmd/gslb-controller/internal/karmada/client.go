package karmada

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	clusterGVR = schema.GroupVersionResource{
		Group:    "cluster.karmada.io",
		Version:  "v1alpha1",
		Resource: "clusters",
	}
)

type ClusterHealth struct {
	Name       string
	Ready      string
	Kubernetes string
	Conditions []map[string]any
}

type Client struct {
	dynClient dynamic.Interface
}

func NewClient(dynClient dynamic.Interface) *Client {
	return &Client{dynClient: dynClient}
}

func (c *Client) ListClusters(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynClient.Resource(clusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list karmada clusters: %w", err)
	}
	return list.Items, nil
}

func (c *Client) GetClusterHealth(ctx context.Context, name string) (*ClusterHealth, error) {
	cluster, err := c.dynClient.Resource(clusterGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get karmada cluster %s: %w", name, err)
	}

	health := &ClusterHealth{Name: name}
	conditions, ok, _ := unstructured.NestedSlice(cluster.Object, "status", "conditions")
	if !ok {
		health.Ready = "Unknown"
		return health, nil
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		entry := make(map[string]any)
		for k, v := range cond {
			entry[k] = v
		}
		health.Conditions = append(health.Conditions, entry)

		switch cond["type"] {
		case "Ready":
			health.Ready = fmt.Sprintf("%v", cond["status"])
		case "KubernetesVersion":
			if v, ok := cond["message"]; ok {
				health.Kubernetes = fmt.Sprintf("%v", v)
			}
		}
	}

	return health, nil
}

func (c *Client) GetClusterStatus(ctx context.Context, name string) string {
	health, err := c.GetClusterHealth(ctx, name)
	if err != nil {
		return "unreachable"
	}
	if health.Ready == "True" {
		return "healthy"
	}
	if health.Ready == "False" {
		return "unreachable"
	}
	return "unknown"
}

func (c *Client) IsHealthy(ctx context.Context, name string) bool {
	return c.GetClusterStatus(ctx, name) == "healthy"
}