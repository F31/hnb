package observer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func openProjectorPG(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedProjectorTarget(t *testing.T, db *sql.DB, tenantID, targetID, targetType string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO tenants (id, name, display_name) VALUES ($1, $1, $1)
		ON CONFLICT (id) DO NOTHING`, tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	workspaceID := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO workspaces (id, tenant_id, name) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`, workspaceID, tenantID, "obs-workspace"); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO runtime_targets (id, tenant_id, name, target_type, connection_type, status, distribution, workspace_id, is_active)
		VALUES ($1, $2, $3, $4, 'agent', 'unknown', 'standard', $5, true)
		ON CONFLICT (id) DO NOTHING`, targetID, tenantID, "obs-test-"+targetID[:8], targetType, workspaceID); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM runtime_target_storage_driver_evidence WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_target_storage_snapshot_api WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_target_storage_inventory WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_target_nodes WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM capability_snapshots WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_target_observation_cursors WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_target_observation_inbox WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM runtime_targets WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM workspaces WHERE tenant_id = $1`, tenantID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = $1`, tenantID)
	})
}

func observationWithStorage(tenantID, targetID, observerID string, generation, sequence int64, observedAt time.Time, mode string, storage map[string]any) []byte {
	var observation map[string]any
	_ = json.Unmarshal(observationWith(tenantID, targetID, "KubernetesTarget", "Agent", observerID, generation, sequence, observedAt, mode, nil), &observation)
	delete(observation, "target")
	observation["storageInventory"] = storage
	payload, _ := json.Marshal(observation)
	return payload
}

func observationWith(tenantID, targetID, targetKind, observerKind, observerID string, generation, sequence int64, observedAt time.Time, mode string, nodes []map[string]any) []byte {
	o := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": tenantID, "targetId": targetID, "targetKind": targetKind,
		"observerId": observerID, "observerKind": observerKind,
		"observerGeneration": generation, "sequence": sequence, "observedAt": observedAt,
		"inventoryMode": mode,
		"target": map[string]any{
			"lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED",
			"lastKnownStateAt": observedAt, "staleThresholdSeconds": 300,
		},
	}
	if nodes != nil {
		o["nodes"] = nodes
	}
	data, _ := json.Marshal(o)
	return data
}

func TestProjectorPGAtomicProjection(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	nodes := []map[string]any{
		{"nodeId": "node-1", "lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED", "freshness": "FRESH",
			"observedAt": observedAt, "lastKnownStateAt": observedAt, "resources": map[string]any{"cpuMillis": 1000, "memoryBytes": 2048}},
	}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nodes)); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}

	var lifecycle, health, connectivity string
	var source string
	var lastKnown time.Time
	if err := db.QueryRow(`SELECT lifecycle_state, health_state, connectivity_state, observation_source, last_known_state_at
		FROM runtime_targets WHERE id = $1 AND tenant_id = $2`, targetID, tenantID).Scan(&lifecycle, &health, &connectivity, &source, &lastKnown); err != nil {
		t.Fatalf("read target: %v", err)
	}
	if lifecycle != "ACTIVE" || health != "HEALTHY" || connectivity != "CONNECTED" || source != "agent" {
		t.Fatalf("target projection = %s/%s/%s/%s", lifecycle, health, connectivity, source)
	}
	if diff := lastKnown.Sub(observedAt); diff > 2*time.Microsecond || diff < -2*time.Microsecond {
		t.Fatalf("lastKnown = %v want ~%v (diff %v)", lastKnown, observedAt, diff)
	}

	var nodeCount int
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount != 1 {
		t.Fatalf("node count = %d want 1", nodeCount)
	}

	var cursorSeq int64
	if err := db.QueryRow(`SELECT observation_revision FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, "agent-1").Scan(&cursorSeq); err != nil {
		t.Fatal(err)
	}
	if cursorSeq != 1 {
		t.Fatalf("cursor sequence = %d want 1", cursorSeq)
	}
}

func TestProjectorPGReplayIsIdempotent(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	first := observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nil)
	if err := projector.Accept(ctx, identity, first); err != nil {
		t.Fatalf("accept: %v", err)
	}
	var nodeCountBefore int
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&nodeCountBefore)

	if err := projector.Accept(ctx, identity, first); err != ErrReplay {
		t.Fatalf("replay err=%v want ErrReplay", err)
	}
	var nodeCountAfter int
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&nodeCountAfter)
	if nodeCountBefore != nodeCountAfter {
		t.Fatalf("replay changed node projection: %d -> %d", nodeCountBefore, nodeCountAfter)
	}
}

func TestProjectorPGGapAndFencing(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nil)); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}
	// Gap: seq 3 without seq 2.
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 3, observedAt, "Full", nil)); err != ErrGap {
		t.Fatalf("gap err=%v want ErrGap", err)
	}
	var gapErr string
	if err := db.QueryRow(`SELECT processing_error FROM runtime_target_observation_inbox
		WHERE tenant_id = $1 AND observation_revision = 3`, tenantID).Scan(&gapErr); err != nil {
		t.Fatalf("gap row: %v", err)
	}
	if gapErr == "" {
		t.Fatal("gap row missing processing_error")
	}
	// After the gap is resolved, seq 2 is accepted.
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 2, observedAt, "Full", nil)); err != nil {
		t.Fatalf("seq2 after gap: %v", err)
	}
	// seq 1 is now a replay (already committed).
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nil)); err != ErrReplay {
		t.Fatalf("seq1 replay err=%v want ErrReplay", err)
	}
}

func TestProjectorPGFullTombstone(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	nodes := []map[string]any{
		{"nodeId": "node-1", "lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED", "freshness": "FRESH",
			"observedAt": observedAt, "lastKnownStateAt": observedAt, "resources": map[string]any{"cpuMillis": 1000, "memoryBytes": 2048}},
		{"nodeId": "node-2", "lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED", "freshness": "FRESH",
			"observedAt": observedAt, "lastKnownStateAt": observedAt, "resources": map[string]any{"cpuMillis": 2000, "memoryBytes": 4096}},
	}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nodes)); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}
	// Full seq2 drops node-2 → tombstone.
	single := []map[string]any{nodes[0]}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 2, observedAt, "Full", single)); err != nil {
		t.Fatalf("accept seq2: %v", err)
	}
	var alive, tombstoned int
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&alive)
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NOT NULL`, tenantID).Scan(&tombstoned)
	if alive != 1 || tombstoned != 1 {
		t.Fatalf("alive=%d tombstoned=%d want 1/1", alive, tombstoned)
	}
}

func TestProjectorPGDeltaTombstone(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	nodes := []map[string]any{
		{"nodeId": "node-1", "lifecycleState": "ACTIVE", "healthState": "HEALTHY", "connectivityState": "CONNECTED", "freshness": "FRESH",
			"observedAt": observedAt, "lastKnownStateAt": observedAt, "resources": map[string]any{"cpuMillis": 1000, "memoryBytes": 2048}},
	}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nodes)); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}
	// Delta tombstone node-1 (deleted=true) without listing others.
	delta := []map[string]any{
		{"nodeId": "node-1", "deleted": true},
	}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 2, observedAt, "Delta", delta)); err != nil {
		t.Fatalf("accept delta tombstone: %v", err)
	}
	var alive, tombstoned int
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&alive)
	_ = db.QueryRow(`SELECT count(*) FROM runtime_target_nodes WHERE tenant_id = $1 AND deleted_at IS NOT NULL`, tenantID).Scan(&tombstoned)
	if alive != 0 || tombstoned != 1 {
		t.Fatalf("alive=%d tombstoned=%d want 0/1", alive, tombstoned)
	}
}

func TestProjectorPGSourceReset(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 1, observedAt, "Full", nil)); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}
	reset := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": tenantID, "targetId": targetID, "targetKind": "KubernetesTarget",
		"observerId": "agent-1", "observerKind": "Agent",
		"previousObserverGeneration": 1, "newObserverGeneration": 2,
		"observedAt": observedAt, "observerLeaseId": uuid.NewString(), "reason": "observer-restarted",
	}
	payload, _ := json.Marshal(reset)
	if err := projector.ApplyReset(ctx, identity, payload); err != nil {
		t.Fatalf("source-reset: %v", err)
	}
	var generation int64
	if err := db.QueryRow(`SELECT observation_generation FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, "agent-1").Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != 2 {
		t.Fatalf("generation = %d want 2", generation)
	}
	// After reset, gen2 seq1 accepted; gen1 seq2 fenced.
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 2, 1, observedAt, "Full", nil)); err != nil {
		t.Fatalf("accept gen2 seq1: %v", err)
	}
	if err := projector.Accept(ctx, identity, observationWith(tenantID, targetID, "KubernetesTarget", "Agent", "agent-1", 1, 2, observedAt, "Full", nil)); err != ErrFenced {
		t.Fatalf("old gen err=%v want ErrFenced", err)
	}
}

func TestProjectorPGCapabilitySnapshotDedup(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-obs-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	store := NewPGCursorStore(db)
	projector := NewProjector(store)
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-1", ObserverKind: "Agent"}

	observedAt := time.Now().UTC().Add(-time.Minute)
	base := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": tenantID, "targetId": targetID, "targetKind": "KubernetesTarget",
		"observerId": "agent-1", "observerKind": "Agent",
		"observerGeneration": 1, "sequence": 1, "observedAt": observedAt, "inventoryMode": "Full",
		"capability": map[string]any{
			"snapshotId": uuid.NewString(), "digest": "sha256:" + fmt.Sprintf("%064x", 1),
			"runtimeVersion": "v1.31.0", "architectures": []string{"amd64"},
			"resources": map[string]any{"cpuMillis": 4000, "memoryBytes": 16384},
		},
	}
	first, _ := json.Marshal(base)
	if err := projector.Accept(ctx, identity, first); err != nil {
		t.Fatalf("accept seq1: %v", err)
	}
	base["sequence"] = 2
	base["eventId"] = uuid.NewString()
	second, _ := json.Marshal(base)
	if err := projector.Accept(ctx, identity, second); err != nil {
		t.Fatalf("accept seq2 same content: %v", err)
	}
	var snapshots int
	if err := db.QueryRow(`SELECT count(*) FROM capability_snapshots WHERE tenant_id = $1`, tenantID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("capability snapshots = %d want 1 (dedup)", snapshots)
	}
}

func TestProjectorPGStorageFullDeltaTombstonesFreshnessAndEvidence(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-" + uuid.NewString()[:8]
	targetID := "515eba09-0a41-5b92-b972-69af1f0f655c"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	resource := func(uid, version, name string) map[string]any {
		return map[string]any{
			"uid": uid, "resourceVersion": version, "name": name,
			"source": "kubernetes.storage.k8s.io/v1", "observedAt": observedAt,
		}
	}
	fast := resource("sc-fast-uid", "1", "fast")
	fast["provisioner"] = "example.csi.io"
	slow := resource("sc-slow-uid", "1", "slow")
	slow["provisioner"] = "example.csi.io"
	driver := resource("driver-uid", "1", "example.csi.io")
	csiNode := resource("csi-node-uid", "1", "worker-1")
	csiNode["drivers"] = []any{map[string]any{"name": "example.csi.io", "nodeId": "node-1", "topologyKeys": []any{}}}
	full := map[string]any{
		"storageClasses": []any{fast, slow}, "csiDrivers": []any{driver}, "csiNodes": []any{csiNode},
		"csiStorageCapacities": []any{}, "volumeAttachments": []any{},
		"snapshotApi": map[string]any{
			"status": "NotInstalled", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": observedAt,
		},
	}
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, "agent-storage", 1, 1, observedAt, "Full", full)); err != nil {
		t.Fatalf("accept storage full: %v", err)
	}

	var active, evidenceCount int
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_inventory WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_driver_evidence WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	if active != 4 || evidenceCount != 4 {
		t.Fatalf("active resources/evidence=%d/%d want 4/4", active, evidenceCount)
	}
	var staleAfter time.Time
	if err := db.QueryRow(`SELECT stale_after FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND resource_kind = 'StorageClass' AND resource_uid = 'sc-fast-uid'`, tenantID).Scan(&staleAfter); err != nil {
		t.Fatal(err)
	}
	if !staleAfter.Equal(observedAt.Add(300 * time.Second)) {
		t.Fatalf("stale_after=%v want %v", staleAfter, observedAt.Add(300*time.Second))
	}
	var snapshotStatus string
	if err := db.QueryRow(`SELECT status FROM runtime_target_storage_snapshot_api WHERE tenant_id = $1 AND target_id = $2`, tenantID, targetID).Scan(&snapshotStatus); err != nil {
		t.Fatal(err)
	}
	if snapshotStatus != "NotInstalled" {
		t.Fatalf("snapshot status=%q", snapshotStatus)
	}

	fastTombstone := resource("sc-fast-uid", "1", "fast")
	fastTombstone["deleted"] = true
	delta := map[string]any{"storageClasses": []any{fastTombstone}}
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, "agent-storage", 1, 2, observedAt.Add(time.Second), "Delta", delta)); err != nil {
		t.Fatalf("accept storage delta: %v", err)
	}
	var fastDeleted bool
	if err := db.QueryRow(`SELECT deleted_at IS NOT NULL FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND resource_kind = 'StorageClass' AND resource_uid = 'sc-fast-uid'`, tenantID).Scan(&fastDeleted); err != nil {
		t.Fatal(err)
	}
	if !fastDeleted {
		t.Fatal("delta tombstone did not mark fast StorageClass deleted")
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 3 {
		t.Fatalf("delta changed unlisted resources: active=%d want 3", active)
	}

	fullOnlySlow := map[string]any{
		"storageClasses": []any{slow}, "csiDrivers": []any{}, "csiNodes": []any{},
		"csiStorageCapacities": []any{}, "volumeAttachments": []any{},
		"snapshotApi": map[string]any{
			"status": "Unsupported", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": observedAt.Add(2 * time.Second),
		},
	}
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, "agent-storage", 1, 3, observedAt.Add(2*time.Second), "Full", fullOnlySlow)); err != nil {
		t.Fatalf("accept replacement full: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_driver_evidence
		WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	var cursorSequence int64
	if err := db.QueryRow(`SELECT observation_revision FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, "agent-storage").Scan(&cursorSequence); err != nil {
		t.Fatal(err)
	}
	if active != 1 || evidenceCount != 1 || cursorSequence != 3 {
		t.Fatalf("full projection resources/evidence/cursor=%d/%d/%d want 1/1/3", active, evidenceCount, cursorSequence)
	}
}
