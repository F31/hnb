package observer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testIdentity() ObserverIdentity {
	return ObserverIdentity{
		TenantID: "tenant-a", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c",
		TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent",
	}
}

func testNode(id string) Node {
	return Node{
		NodeID: id, Name: id, LifecycleState: "ACTIVE", HealthState: "HEALTHY",
		ConnectivityState: "CONNECTED", Freshness: "FRESH", ObservedAt: time.Now().UTC().Add(-time.Second),
		LastKnownStateAt: time.Now().UTC().Add(-time.Second),
		Resources:        map[string]int64{"cpuMillis": 1000, "memoryBytes": 2048},
	}
}

func TestProducerFullMonotonicSequence(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	observedAt := time.Now().UTC().Add(-time.Second)
	target := &TargetState{LifecycleState: "ACTIVE", HealthState: "HEALTHY", ConnectivityState: "CONNECTED", LastKnownStateAt: observedAt, StaleThresholdSeconds: 300}

	first, err := p.Full(observedAt, target, nil, []Node{testNode("node-1")})
	if err != nil {
		t.Fatal(err)
	}
	if p.Sequence() != 2 {
		t.Fatalf("sequence after full = %d want 2", p.Sequence())
	}
	second, err := p.Full(observedAt, target, nil, []Node{testNode("node-1"), testNode("node-2")})
	if err != nil {
		t.Fatal(err)
	}
	var o1, o2 Observation
	if err := json.Unmarshal(first, &o1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &o2); err != nil {
		t.Fatal(err)
	}
	if o1.Sequence != 1 || o2.Sequence != 2 || o1.ObserverGeneration != 1 || o2.ObserverGeneration != 1 {
		t.Fatalf("seq/gen = %d/%d and %d/%d", o1.Sequence, o1.ObserverGeneration, o2.Sequence, o2.ObserverGeneration)
	}
}

func TestProducerDeltaTombstones(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	observedAt := time.Now().UTC().Add(-time.Second)
	target := &TargetState{LifecycleState: "ACTIVE", HealthState: "HEALTHY", ConnectivityState: "CONNECTED", LastKnownStateAt: observedAt, StaleThresholdSeconds: 300}

	if _, err := p.Full(observedAt, target, nil, []Node{testNode("node-1"), testNode("node-2")}); err != nil {
		t.Fatal(err)
	}
	delta, err := p.DeltaFromCache(observedAt, target, nil, []Node{testNode("node-1")})
	if err != nil {
		t.Fatal(err)
	}
	var o Observation
	if err := json.Unmarshal(delta, &o); err != nil {
		t.Fatal(err)
	}
	if o.InventoryMode != "Delta" {
		t.Fatalf("mode = %s", o.InventoryMode)
	}
	if len(o.Nodes) != 1 || !o.Nodes[0].Deleted || o.Nodes[0].NodeID != "node-2" {
		t.Fatalf("delta nodes = %+v", o.Nodes)
	}
}

func TestProducerStorageDeltaUsesUIDAndResourceVersion(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	observedAt := time.Now().UTC().Add(-time.Second)
	storage := &StorageInventory{
		SnapshotAPI: &SnapshotAPI{Status: "NotInstalled", Source: "kubernetes.apidiscovery.k8s.io/v1", ObservedAt: observedAt},
		StorageClasses: []StorageClassFact{
			{KubernetesResourceIdentity: KubernetesResourceIdentity{UID: "sc-1", ResourceVersion: "1", Name: "fast", Source: storageAPISource, ObservedAt: observedAt}},
			{KubernetesResourceIdentity: KubernetesResourceIdentity{UID: "sc-2", ResourceVersion: "1", Name: "archive", Source: storageAPISource, ObservedAt: observedAt}},
		},
		CSIDrivers: []CSIDriverFact{}, CSINodes: []CSINodeFact{},
		CSIStorageCapacities: []CSIStorageCapacityFact{}, VolumeAttachments: []VolumeAttachmentFact{},
	}
	if _, err := p.FullWithStorage(observedAt, nil, nil, []Node{testNode("node-1")}, storage); err != nil {
		t.Fatal(err)
	}
	current := &StorageInventory{
		SnapshotAPI: &SnapshotAPI{Status: "NotInstalled", Source: "kubernetes.apidiscovery.k8s.io/v1", ObservedAt: observedAt},
		StorageClasses: []StorageClassFact{
			{KubernetesResourceIdentity: KubernetesResourceIdentity{UID: "sc-1", ResourceVersion: "2", Name: "fast", Source: storageAPISource, ObservedAt: observedAt}},
		},
		CSIDrivers: []CSIDriverFact{}, CSINodes: []CSINodeFact{},
		CSIStorageCapacities: []CSIStorageCapacityFact{}, VolumeAttachments: []VolumeAttachmentFact{},
	}
	payload, err := p.DeltaFromCacheWithStorage(observedAt, nil, nil, []Node{testNode("node-1")}, current)
	if err != nil {
		t.Fatal(err)
	}
	var observation Observation
	if err := json.Unmarshal(payload, &observation); err != nil {
		t.Fatal(err)
	}
	if observation.InventoryMode != "Delta" || observation.StorageInventory == nil || len(observation.StorageInventory.StorageClasses) != 2 {
		t.Fatalf("storage delta = %+v", observation.StorageInventory)
	}
	byUID := make(map[string]StorageClassFact)
	for _, fact := range observation.StorageInventory.StorageClasses {
		byUID[fact.UID] = fact
	}
	if byUID["sc-1"].ResourceVersion != "2" || byUID["sc-1"].Deleted {
		t.Fatalf("updated fact = %+v", byUID["sc-1"])
	}
	if !byUID["sc-2"].Deleted || byUID["sc-2"].ResourceVersion != "1" {
		t.Fatalf("tombstone = %+v", byUID["sc-2"])
	}
}

func TestProducerFullStorageSerializesEmptyCoreCollections(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	observedAt := time.Now().UTC()
	payload, err := p.FullWithStorage(observedAt, nil, nil, []Node{}, &StorageInventory{
		SnapshotAPI: &SnapshotAPI{Status: "NotInstalled", Source: "kubernetes.apidiscovery.k8s.io/v1", ObservedAt: observedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	if err := json.Unmarshal(payload, &encoded); err != nil {
		t.Fatal(err)
	}
	storage := encoded["storageInventory"].(map[string]any)
	for _, field := range []string{"storageClasses", "csiDrivers", "csiNodes", "csiStorageCapacities", "volumeAttachments"} {
		value, ok := storage[field].([]any)
		if !ok || len(value) != 0 {
			t.Fatalf("%s = %#v, want empty array", field, storage[field])
		}
	}
	for _, field := range []string{"volumeSnapshotClasses", "volumeSnapshots", "volumeSnapshotContents"} {
		if _, exists := storage[field]; exists {
			t.Fatalf("snapshot field %s must be omitted when the API is not installed", field)
		}
	}
	if storage["snapshotApi"].(map[string]any)["status"] != "NotInstalled" {
		t.Fatalf("snapshotApi = %#v", storage["snapshotApi"])
	}
}

func TestProducerDeltaCarriesSnapshotAPICapabilityChange(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	observedAt := time.Now().UTC()
	core := StorageInventory{
		StorageClasses: []StorageClassFact{}, CSIDrivers: []CSIDriverFact{}, CSINodes: []CSINodeFact{},
		CSIStorageCapacities: []CSIStorageCapacityFact{}, VolumeAttachments: []VolumeAttachmentFact{},
		SnapshotAPI: &SnapshotAPI{Status: "NotInstalled", Source: snapshotAPISource, ObservedAt: observedAt},
	}
	if _, err := p.FullWithStorage(observedAt, nil, nil, []Node{}, &core); err != nil {
		t.Fatal(err)
	}
	current := core
	current.SnapshotAPI = &SnapshotAPI{Status: "Installed", APIVersion: snapshotAPIVersion, Source: snapshotAPISource, ObservedAt: observedAt.Add(time.Second)}
	current.VolumeSnapshotClasses = []VolumeSnapshotClassFact{}
	current.VolumeSnapshots = []VolumeSnapshotFact{}
	current.VolumeSnapshotContents = []VolumeSnapshotContentFact{}
	payload, err := p.DeltaFromCacheWithStorage(observedAt.Add(time.Second), nil, nil, []Node{}, &current)
	if err != nil {
		t.Fatal(err)
	}
	var observation Observation
	if err := json.Unmarshal(payload, &observation); err != nil {
		t.Fatal(err)
	}
	if observation.StorageInventory == nil || observation.StorageInventory.SnapshotAPI == nil || observation.StorageInventory.SnapshotAPI.Status != "Installed" {
		t.Fatalf("snapshot API delta = %+v", observation.StorageInventory)
	}
}

func TestProducerSourceReset(t *testing.T) {
	var persisted int64
	p := NewProducer(testIdentity(), 1, 1, func(g int64) error { persisted = g; return nil })
	if err := p.SourceReset(2); err != nil {
		t.Fatal(err)
	}
	if p.Generation() != 2 || p.Sequence() != 1 || persisted != 2 {
		t.Fatalf("gen=%d seq=%d persisted=%d", p.Generation(), p.Sequence(), persisted)
	}
	if err := p.SourceReset(2); err == nil {
		t.Fatal("expected same-generation reset rejection")
	}
}

func TestProducerPayloadBounds(t *testing.T) {
	p := NewProducer(testIdentity(), 1, 1, nil)
	p.maxPayloadBytes = 256
	observedAt := time.Now().UTC().Add(-time.Second)
	target := &TargetState{LifecycleState: "ACTIVE", HealthState: "HEALTHY", ConnectivityState: "CONNECTED", LastKnownStateAt: observedAt, StaleThresholdSeconds: 300}
	big := Node{
		NodeID: "node-" + strings.Repeat("x", 500), LifecycleState: "ACTIVE", HealthState: "HEALTHY",
		ConnectivityState: "CONNECTED", Freshness: "FRESH", ObservedAt: observedAt, LastKnownStateAt: observedAt,
		Resources: map[string]int64{"cpuMillis": 1, "memoryBytes": 1},
	}
	if _, err := p.Full(observedAt, target, nil, []Node{big}); err == nil {
		t.Fatal("expected payload bound rejection")
	}
}

func TestKubeDiscoveryParsingHelpers(t *testing.T) {
	if got := parseCPUToMillis("2"); got != 2000 {
		t.Fatalf("2 cpu = %d", got)
	}
	if got := parseCPUToMillis("250m"); got != 250 {
		t.Fatalf("250m = %d", got)
	}
	if got := parseBytes("4Gi"); got != 4<<30 {
		t.Fatalf("4Gi = %d", got)
	}
	if got := parseBytes("512Mi"); got != 512<<20 {
		t.Fatalf("512Mi = %d", got)
	}
}

func TestCapabilityDigestStable(t *testing.T) {
	cap1 := &Capability{SnapshotID: uuid.NewString(), KubernetesVersion: "v1.31.0", RuntimeVersion: "v1.31.0", Architectures: []string{"amd64"}}
	cap2 := &Capability{SnapshotID: uuid.NewString(), KubernetesVersion: "v1.31.0", RuntimeVersion: "v1.31.0", Architectures: []string{"amd64"}}
	if capContentDigest(cap1) != capContentDigest(cap2) {
		t.Fatal("content digest should be stable across snapshot IDs")
	}
}
