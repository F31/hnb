package observer

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func edgeIdentity() ObserverIdentity {
	return ObserverIdentity{
		TenantID: "tenant-a", TargetID: "6d384d43-243b-5e14-b7e4-c03be376cb7c",
		TargetKind: "EdgeRuntimeTarget", ObserverID: "cloudcore-1", ObserverKind: "CloudCore",
	}
}

func fakeCloudCoreClient() *fake.Clientset {
	client := fake.NewSimpleClientset()
	_, _ = client.CoreV1().Nodes().Create(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-node-1", Labels: map[string]string{"hnb.io/node-group": "group-a"}},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{Architecture: "arm64", KubeletVersion: "v1.31.0", OperatingSystem: "linux"},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	}, metav1.CreateOptions{})
	return client
}

func TestCloudCoreObserverFullAndDelta(t *testing.T) {
	client := fakeCloudCoreClient()
	producer := NewProducer(edgeIdentity(), 1, 1, nil)
	observer := NewCloudCoreObserver(producer, client)
	ctx := context.Background()

	first, err := observer.ReportOnce(ctx)
	if err != nil {
		t.Fatalf("first report: %v", err)
	}
	var o1 Observation
	if err := json.Unmarshal(first, &o1); err != nil {
		t.Fatal(err)
	}
	if o1.InventoryMode != "Full" || o1.ObserverKind != "CloudCore" || o1.ObserverGeneration != 1 || o1.Sequence != 1 {
		t.Fatalf("o1 = %+v", o1)
	}
	if len(o1.Nodes) != 1 || o1.Nodes[0].NodeID != "edge-node-1" || o1.Nodes[0].ConnectivityState != "CONNECTED" {
		t.Fatalf("nodes = %+v", o1.Nodes)
	}
	if o1.Capability == nil || o1.Capability.KubeEdgeVersion == "" {
		t.Fatal("capability missing")
	}

	second, err := observer.ReportOnce(ctx)
	if err != nil {
		t.Fatalf("second report: %v", err)
	}
	var o2 Observation
	if err := json.Unmarshal(second, &o2); err != nil {
		t.Fatal(err)
	}
	if o2.InventoryMode != "Delta" || o2.Sequence != 2 {
		t.Fatalf("o2 = %+v", o2)
	}
}

func TestCloudCoreObserverNodeDisconnect(t *testing.T) {
	client := fakeCloudCoreClient()
	producer := NewProducer(edgeIdentity(), 1, 1, nil)
	observer := NewCloudCoreObserver(producer, client)
	ctx := context.Background()

	if _, err := observer.ReportOnce(ctx); err != nil {
		t.Fatal(err)
	}
	// Mark the node NotReady and re-report: it should flip to DISCONNECTED.
	node, _ := client.CoreV1().Nodes().Get(ctx, "edge-node-1", metav1.GetOptions{})
	node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
	_, _ = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})

	payload, err := observer.ReportOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var o Observation
	if err := json.Unmarshal(payload, &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Nodes) != 1 || o.Nodes[0].ConnectivityState != "DISCONNECTED" || o.Nodes[0].HealthState != "UNHEALTHY" {
		t.Fatalf("nodes = %+v", o.Nodes)
	}
}

func TestEdgeProducerSourceReset(t *testing.T) {
	producer := NewProducer(edgeIdentity(), 1, 1, nil)
	if err := producer.SourceReset(2); err != nil {
		t.Fatal(err)
	}
	if producer.Generation() != 2 || producer.Sequence() != 1 {
		t.Fatalf("gen=%d seq=%d", producer.Generation(), producer.Sequence())
	}
}
