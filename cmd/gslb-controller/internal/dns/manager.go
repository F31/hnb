package dns

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	dnsEndpointGVR = schema.GroupVersionResource{
		Group:    "externaldns.k8s.io",
		Version:  "v1alpha1",
		Resource: "dnsendpoints",
	}
)

type DNSRecord struct {
	DNSName string
	Targets []string
	Weight  int
	TTL     int
	SetID   string
}

type Manager struct {
	client    dynamic.NamespaceableResourceInterface
	namespace string
	ownerID   string
}

func NewManager(dynClient dynamic.Interface, namespace string) *Manager {
	return &Manager{
		client:    dynClient.Resource(dnsEndpointGVR),
		namespace: namespace,
		ownerID:   fmt.Sprintf("gslb-controller-%s", uuid.New().String()[:8]),
	}
}

func (m *Manager) EnsureEndpoint(ctx context.Context, name string, records []DNSRecord) error {
	if len(records) == 0 {
		return m.deleteEndpoint(ctx, name)
	}

	existing, err := m.client.Namespace(m.namespace).Get(ctx, name, metav1.GetOptions{})
	exists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get dnsendpoint %s: %w", name, err)
		}
		exists = false
	}

	endpoints := make([]any, 0, len(records))
	for _, r := range records {
		endpoints = append(endpoints, map[string]any{
			"dnsName": r.DNSName,
			"targets": r.Targets,
			"recordType": "A",
			"setIdentifier": r.SetID,
			"recordTTL": r.TTL,
			"providerSpecific": []any{
				map[string]string{
					"name":  "weight",
					"value": fmt.Sprintf("%d", r.Weight),
				},
			},
		})
	}

	if exists {
		existing.Object["spec"] = map[string]any{
			"endpoints": endpoints,
		}
		_, err = m.client.Namespace(m.namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update dnsendpoint %s: %w", name, err)
		}
		return nil
	}

	ep := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "externaldns.k8s.io/v1alpha1",
			"kind":       "DNSEndpoint",
			"metadata": map[string]any{
				"name":      name,
				"namespace": m.namespace,
				"labels": map[string]string{
					"app.kubernetes.io/managed-by": "gslb-controller",
					"gslb.hnb.cloud/owner":         m.ownerID,
				},
			},
			"spec": map[string]any{
				"endpoints": endpoints,
			},
		},
	}

	_, err = m.client.Namespace(m.namespace).Create(ctx, ep, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create dnsendpoint %s: %w", name, err)
	}
	return nil
}

func (m *Manager) deleteEndpoint(ctx context.Context, name string) error {
	err := m.client.Namespace(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete dnsendpoint %s: %w", name, err)
	}
	return nil
}

func (m *Manager) ListEndpoints(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := m.client.Namespace(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "gslb.hnb.cloud/owner=" + m.ownerID,
	})
	if err != nil {
		return nil, fmt.Errorf("list dnsendpoints: %w", err)
	}
	return list.Items, nil
}

func (m *Manager) CleanupOrphaned(ctx context.Context, activeNames map[string]bool) error {
	existing, err := m.ListEndpoints(ctx)
	if err != nil {
		return err
	}
	for _, ep := range existing {
		name := ep.GetName()
		if !activeNames[name] {
			if err := m.deleteEndpoint(ctx, name); err != nil {
				return fmt.Errorf("cleanup orphaned dnsendpoint %s: %w", name, err)
			}
		}
	}
	return nil
}