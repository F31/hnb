package observer

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CloudCoreObserver discovers edge node inventory and cluster capability
// through the authenticated CloudCore API (never by connecting to EdgeCore
// directly), and emits RT-008 observation envelopes with monotonic
// generation/sequence.
type CloudCoreObserver struct {
	producer *Producer
	client   kubernetes.Interface
}

func NewCloudCoreObserver(producer *Producer, client kubernetes.Interface) *CloudCoreObserver {
	return &CloudCoreObserver{producer: producer, client: client}
}

// DiscoverNodes lists edge nodes via the CloudCore API.
func (o *CloudCoreObserver) DiscoverNodes(ctx context.Context, observedAt time.Time) ([]Node, error) {
	items, err := o.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(items.Items))
	for _, item := range items.Items {
		nodes = append(nodes, nodeFromCoreV1(item, observedAt))
	}
	return nodes, nil
}

// DiscoverCapability builds the capability snapshot from the CloudCore server
// version and aggregated node resources.
func (o *CloudCoreObserver) DiscoverCapability(ctx context.Context, nodes []Node, observedAt time.Time) (*Capability, error) {
	info, err := o.client.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}
	var cpuMillis, memoryBytes int64
	archs := map[string]bool{}
	for _, node := range nodes {
		cpuMillis += node.Resources["cpuMillis"]
		memoryBytes += node.Resources["memoryBytes"]
		if node.Architecture != "" {
			archs[node.Architecture] = true
		}
	}
	cap := &Capability{
		KubeEdgeVersion: info.GitVersion,
		RuntimeVersion:  info.GitVersion,
		Architectures:   keys(archs),
	}
	cap.Resources.CpuMillis = cpuMillis
	cap.Resources.MemoryBytes = memoryBytes
	cap.Digest = capContentDigest(cap)
	return cap, nil
}

// ReportOnce performs one observation cycle and emits Full then Delta.
func (o *CloudCoreObserver) ReportOnce(ctx context.Context) ([]byte, error) {
	observedAt := time.Now().UTC()
	nodes, err := o.DiscoverNodes(ctx, observedAt)
	if err != nil {
		return nil, err
	}
	capability, err := o.DiscoverCapability(ctx, nodes, observedAt)
	if err != nil {
		return nil, err
	}
	target := &TargetState{
		LifecycleState:        "ACTIVE",
		HealthState:           "HEALTHY",
		ConnectivityState:     "CONNECTED",
		LastKnownStateAt:      observedAt,
		StaleThresholdSeconds: 300,
		RuntimeVersion:        capability.KubeEdgeVersion,
	}
	if len(o.producer.LastInventory()) == 0 {
		return o.producer.Full(observedAt, target, capability, nodes)
	}
	return o.producer.DeltaFromCache(observedAt, target, capability, nodes)
}

func nodeFromCoreV1(item corev1.Node, observedAt time.Time) Node {
	id := item.Name
	if item.UID != "" {
		id = string(item.UID)
	}
	connected := false
	for _, cond := range item.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			connected = true
			break
		}
	}
	connectivity := "DISCONNECTED"
	health := "UNHEALTHY"
	if connected {
		connectivity = "CONNECTED"
		health = "HEALTHY"
	}
	arch := item.Status.NodeInfo.Architecture
	if arch == "" {
		arch = item.Status.NodeInfo.OperatingSystem
	}
	return Node{
		NodeID:            id,
		Name:              item.Name,
		LifecycleState:    "ACTIVE",
		HealthState:       health,
		ConnectivityState: connectivity,
		Freshness:         "FRESH",
		ObservedAt:        observedAt,
		LastKnownStateAt:  observedAt,
		RuntimeVersion:    item.Status.NodeInfo.KubeletVersion,
		KubeletVersion:    item.Status.NodeInfo.KubeletVersion,
		Architecture:      arch,
		Labels:            item.Labels,
		Resources: map[string]int64{
			"cpuMillis":   parseCPUToMillis(item.Status.Allocatable.Cpu().String()),
			"memoryBytes": parseBytes(item.Status.Allocatable.Memory().String()),
		},
	}
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
