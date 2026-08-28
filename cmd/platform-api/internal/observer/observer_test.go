package observer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testIdentity() Identity {
	return Identity{
		TenantID: "tenant-a", TargetID: "515eba09-0a41-5b92-b972-69af1f0f655c",
		TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent",
	}
}

func baseObservation(sequence int64) []byte {
	o := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": "tenant-a", "targetId": "515eba09-0a41-5b92-b972-69af1f0f655c",
		"targetKind": "KubernetesTarget", "observerId": "agent-1", "observerKind": "Agent",
		"observerGeneration": 1, "sequence": sequence, "observedAt": time.Now().UTC().Add(-time.Second),
		"inventoryMode": "Full",
		"target": map[string]any{
			"lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED",
			"lastKnownStateAt": time.Now().UTC().Add(-time.Second), "staleThresholdSeconds": 300,
		},
		"nodes": []any{
			map[string]any{
				"nodeId": "node-1", "lifecycleState": "ACTIVE", "healthState": "HEALTHY",
				"connectivityState": "CONNECTED", "freshness": "FRESH",
				"observedAt": time.Now().UTC().Add(-time.Second), "lastKnownStateAt": time.Now().UTC().Add(-time.Second),
				"resources": map[string]any{"cpuMillis": 1000, "memoryBytes": 2048},
			},
		},
	}
	data, _ := json.Marshal(o)
	return data
}

func TestValidateObservationRejectsIdentityMismatch(t *testing.T) {
	id := testIdentity()
	bad := []struct {
		name   string
		mutate func(*map[string]any)
	}{
		{"tenant", func(m *map[string]any) { (*m)["tenantId"] = "tenant-b" }},
		{"target", func(m *map[string]any) { (*m)["targetId"] = uuid.NewString() }},
		{"kind", func(m *map[string]any) { (*m)["targetKind"] = "EdgeRuntimeTarget" }},
		{"observerId", func(m *map[string]any) { (*m)["observerId"] = "agent-2" }},
		{"observerKind", func(m *map[string]any) { (*m)["observerKind"] = "CloudCore" }},
		{"unknownField", func(m *map[string]any) { (*m)["sneaky"] = true }},
		{"schemaVersion", func(m *map[string]any) { (*m)["schemaVersion"] = "9.9.9" }},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			var obj map[string]any
			if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&obj)
			payload, _ := json.Marshal(obj)
			if _, err := ValidateObservation(id, payload); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestValidateObservationRejectsFutureObservedAt(t *testing.T) {
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
		t.Fatal(err)
	}
	obj["observedAt"] = time.Now().UTC().Add(10 * time.Minute)
	payload, _ := json.Marshal(obj)
	if _, err := ValidateObservation(testIdentity(), payload); err == nil {
		t.Fatal("expected future-clock rejection")
	}
}

func TestValidateObservationRejectsKindSourceMismatch(t *testing.T) {
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
		t.Fatal(err)
	}
	obj["observerKind"] = "CloudCore"
	payload, _ := json.Marshal(obj)
	if _, err := ValidateObservation(testIdentity(), payload); err == nil {
		t.Fatal("expected kind/source mismatch rejection")
	}
}

func TestValidateObservationFullRejectsTombstone(t *testing.T) {
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
		t.Fatal(err)
	}
	nodes := obj["nodes"].([]any)
	node := nodes[0].(map[string]any)
	node["deleted"] = true
	payload, _ := json.Marshal(obj)
	if _, err := ValidateObservation(testIdentity(), payload); err == nil {
		t.Fatal("expected full-inventory tombstone rejection")
	}
}

func TestValidateObservationStorageInventoryFullAndDelta(t *testing.T) {
	observedAt := time.Now().UTC().Add(-time.Second)
	storageClass := map[string]any{
		"uid": "storage-class-uid-1", "resourceVersion": "1842", "name": "fast",
		"source": "kubernetes.storage.k8s.io/v1", "observedAt": observedAt,
		"provisioner": "example.csi.io", "allowVolumeExpansion": true,
	}
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
		t.Fatal(err)
	}
	obj["storageInventory"] = map[string]any{
		"storageClasses": []any{storageClass}, "csiDrivers": []any{}, "csiNodes": []any{},
		"csiStorageCapacities": []any{}, "volumeAttachments": []any{},
		"snapshotApi": map[string]any{"status": "NotInstalled", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": observedAt},
	}
	payload, _ := json.Marshal(obj)
	full, err := ValidateObservation(testIdentity(), payload)
	if err != nil {
		t.Fatalf("validate full storage inventory: %v", err)
	}
	if full.StorageInventory == nil || len(full.StorageInventory.StorageClasses) != 1 {
		t.Fatalf("storage inventory not normalized: %+v", full.StorageInventory)
	}
	if got := full.StorageInventory.StorageClasses[0]; got.UID != "storage-class-uid-1" || got.ResourceVersion != "1842" {
		t.Fatalf("unstable identity normalization: %+v", got.KubernetesResourceIdentity)
	}

	obj["inventoryMode"] = "Delta"
	storageClass["resourceVersion"] = "1843"
	storageClass["deleted"] = true
	payload, _ = json.Marshal(obj)
	delta, err := ValidateObservation(testIdentity(), payload)
	if err != nil {
		t.Fatalf("validate delta tombstone: %v", err)
	}
	if !delta.StorageInventory.StorageClasses[0].Deleted {
		t.Fatal("delta storage tombstone was not retained")
	}

	obj["inventoryMode"] = "Full"
	payload, _ = json.Marshal(obj)
	if _, err := ValidateObservation(testIdentity(), payload); err == nil {
		t.Fatal("expected Full storage tombstone rejection")
	}
}

func TestValidateObservationStorageInventoryRequiresStableIdentity(t *testing.T) {
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(1), &obj); err != nil {
		t.Fatal(err)
	}
	obj["storageInventory"] = map[string]any{
		"storageClasses": []any{}, "csiNodes": []any{}, "csiStorageCapacities": []any{}, "volumeAttachments": []any{},
		"snapshotApi": map[string]any{"status": "NotInstalled", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": time.Now().UTC().Add(-time.Second)},
		"csiDrivers": []any{map[string]any{
			"resourceVersion": "1843", "name": "example.csi.io",
			"source": "kubernetes.storage.k8s.io/v1", "observedAt": time.Now().UTC().Add(-time.Second),
		}},
	}
	payload, _ := json.Marshal(obj)
	if _, err := ValidateObservation(testIdentity(), payload); err == nil {
		t.Fatal("expected missing Kubernetes UID rejection")
	}
}

func TestObservationDigestIncludesStorageInventory(t *testing.T) {
	payload := baseObservation(1)
	withoutStorage, err := ValidateObservation(testIdentity(), payload)
	if err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{}
	if err := json.Unmarshal(payload, &obj); err != nil {
		t.Fatal(err)
	}
	obj["storageInventory"] = map[string]any{
		"storageClasses": []any{}, "csiDrivers": []any{}, "csiNodes": []any{}, "csiStorageCapacities": []any{},
		"snapshotApi": map[string]any{"status": "NotInstalled", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": time.Now().UTC().Add(-time.Second)},
		"volumeAttachments": []any{map[string]any{
			"uid": "attachment-uid-1", "resourceVersion": "1", "name": "attachment-a",
			"source": "kubernetes.storage.k8s.io/v1", "observedAt": time.Now().UTC().Add(-time.Second),
		}},
	}
	payload, _ = json.Marshal(obj)
	withStorage, err := ValidateObservation(testIdentity(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if withoutStorage.Digest() == withStorage.Digest() {
		t.Fatal("storage inventory must participate in the observation digest")
	}
}

type fakeStore struct {
	current cursor
	exists  bool
	saved   int
	replays int
	gaps    int
	resets  int
}

func (f *fakeStore) LoadCursor(_ context.Context, _, _, _ string) (cursor, bool, error) {
	return f.current, f.exists, nil
}
func (f *fakeStore) SaveObservation(_ context.Context, _ *Observation, _ Identity, _ string) error {
	f.saved++
	return nil
}
func (f *fakeStore) ApplySourceReset(_ context.Context, _ *SourceReset, _ Identity) error {
	f.resets++
	return nil
}
func (f *fakeStore) RecordReplay(_ context.Context, _ *Observation) error {
	f.replays++
	return nil
}
func (f *fakeStore) RecordGap(_ context.Context, _ *Observation, _ int64) error {
	f.gaps++
	return nil
}

func TestProjectorInitialAcceptsGeneration1Sequence1(t *testing.T) {
	st := &fakeStore{}
	p := NewProjector(st)
	if err := p.Accept(context.Background(), testIdentity(), baseObservation(1)); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if st.saved != 1 {
		t.Fatalf("saved=%d want 1", st.saved)
	}
}

func TestProjectorRejectsNonInitialFirstObservation(t *testing.T) {
	st := &fakeStore{}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), baseObservation(2))
	if err != ErrGap {
		t.Fatalf("err=%v want ErrGap", err)
	}
	if st.gaps != 1 {
		t.Fatalf("gaps=%d want 1", st.gaps)
	}
}

func TestProjectorAcceptsSequentialAfterCursor(t *testing.T) {
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 1}, exists: true}
	p := NewProjector(st)
	if err := p.Accept(context.Background(), testIdentity(), baseObservation(2)); err != nil {
		t.Fatalf("accept seq2: %v", err)
	}
	if st.saved != 1 {
		t.Fatalf("saved=%d want 1", st.saved)
	}
}

func TestProjectorRejectsGapAfterCursor(t *testing.T) {
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 1}, exists: true}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), baseObservation(3))
	if err != ErrGap {
		t.Fatalf("err=%v want ErrGap", err)
	}
	if st.saved != 0 {
		t.Fatalf("saved=%d want 0", st.saved)
	}
}

func TestProjectorRejectsLowerGeneration(t *testing.T) {
	st := &fakeStore{current: cursor{ObserverGeneration: 2, Sequence: 3}, exists: true}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), baseObservation(1))
	if err != ErrFenced {
		t.Fatalf("err=%v want ErrFenced", err)
	}
}

func TestProjectorRejectsGenerationJumpWithoutReset(t *testing.T) {
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 1}, exists: true}
	p := NewProjector(st)
	obj := map[string]any{}
	if err := json.Unmarshal(baseObservation(2), &obj); err != nil {
		t.Fatal(err)
	}
	obj["observerGeneration"] = 2
	payload, _ := json.Marshal(obj)
	err := p.Accept(context.Background(), testIdentity(), payload)
	if err != ErrFenced {
		t.Fatalf("err=%v want ErrFenced (generation jump)", err)
	}
}

func TestProjectorReplayIdenticalDigest(t *testing.T) {
	committed := baseObservation(1)
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 1, PayloadDigest: mustDigest(committed)}, exists: true}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), committed)
	if err != ErrReplay {
		t.Fatalf("err=%v want ErrReplay", err)
	}
}

func TestProjectorReplaySameEventID(t *testing.T) {
	committed := baseObservation(2)
	var obj map[string]any
	if err := json.Unmarshal(committed, &obj); err != nil {
		t.Fatal(err)
	}
	eventID := obj["eventId"].(string)
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 2, LastMessageID: eventID}, exists: true}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), committed)
	if err != ErrReplay {
		t.Fatalf("err=%v want ErrReplay (same eventId)", err)
	}
}

func TestProjectorConflictOnSameSequenceDifferentDigest(t *testing.T) {
	st := &fakeStore{current: cursor{ObserverGeneration: 1, Sequence: 1, PayloadDigest: "sha256:deadbeef"}, exists: true}
	p := NewProjector(st)
	err := p.Accept(context.Background(), testIdentity(), baseObservation(1))
	if err == nil || err == ErrGap || err == ErrReplay || err == ErrFenced {
		t.Fatalf("err=%v want conflict error", err)
	}
}

func TestSourceResetValidates(t *testing.T) {
	id := testIdentity()
	reset := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": "tenant-a", "targetId": "515eba09-0a41-5b92-b972-69af1f0f655c",
		"targetKind": "KubernetesTarget", "observerId": "agent-1", "observerKind": "Agent",
		"previousObserverGeneration": 1, "newObserverGeneration": 2,
		"observedAt": time.Now().UTC().Add(-time.Second), "observerLeaseId": uuid.NewString(), "reason": "observer-restarted",
	}
	payload, _ := json.Marshal(reset)
	got, err := ValidateSourceReset(id, payload)
	if err != nil {
		t.Fatalf("validate source-reset: %v", err)
	}
	if got.NewObserverGeneration != 2 || got.PreviousObserverGeneration != 1 {
		t.Fatalf("reset=%+v", got)
	}
}

func TestSourceResetRejectsBadGenerationAndIdentity(t *testing.T) {
	id := testIdentity()
	reset := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": "tenant-a", "targetId": "515eba09-0a41-5b92-b972-69af1f0f655c",
		"targetKind": "KubernetesTarget", "observerId": "agent-1", "observerKind": "Agent",
		"previousObserverGeneration": 2, "newObserverGeneration": 2,
		"observedAt": time.Now().UTC().Add(-time.Second), "observerLeaseId": uuid.NewString(), "reason": "observer-restarted",
	}
	payload, _ := json.Marshal(reset)
	if _, err := ValidateSourceReset(id, payload); err == nil {
		t.Fatal("expected equal-generation reset rejection")
	}
	reset["previousObserverGeneration"] = 1
	reset["tenantId"] = "tenant-b"
	payload, _ = json.Marshal(reset)
	if _, err := ValidateSourceReset(id, payload); err == nil {
		t.Fatal("expected identity mismatch rejection")
	}
}

func mustDigest(payload []byte) string {
	o, err := ValidateObservation(testIdentity(), payload)
	if err != nil {
		panic(err)
	}
	return o.Digest()
}
