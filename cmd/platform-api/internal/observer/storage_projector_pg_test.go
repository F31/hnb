package observer

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func storageClassObservationFact(observedAt time.Time, uid, resourceVersion, name, provisioner string, deleted bool) map[string]any {
	fact := map[string]any{
		"uid": uid, "resourceVersion": resourceVersion, "name": name,
		"source": "kubernetes.storage.k8s.io/v1", "observedAt": observedAt,
		"provisioner": provisioner,
	}
	if deleted {
		fact["deleted"] = true
	}
	return fact
}

func storageCapacityObservationFact(observedAt time.Time, uid, name, storageClass string, capacityBytes *int64) map[string]any {
	fact := map[string]any{
		"uid": uid, "resourceVersion": "1", "name": name, "namespace": "storage-system",
		"source": "kubernetes.storage.k8s.io/v1", "observedAt": observedAt,
		"storageClassName": storageClass,
	}
	if capacityBytes != nil {
		fact["capacityBytes"] = *capacityBytes
	}
	return fact
}

func fullStorageInventory(observedAt time.Time, storageClasses ...map[string]any) map[string]any {
	classes := make([]any, len(storageClasses))
	for i := range storageClasses {
		classes[i] = storageClasses[i]
	}
	return map[string]any{
		"storageClasses": classes, "csiDrivers": []any{}, "csiNodes": []any{},
		"csiStorageCapacities": []any{}, "volumeAttachments": []any{},
		"snapshotApi": map[string]any{
			"status": "NotInstalled", "source": "kubernetes.apidiscovery.k8s.io/v1", "observedAt": observedAt,
		},
	}
}

func storageProjectionCounts(t *testing.T, db *sql.DB, tenantID string) (active, tombstoned, evidence int) {
	t.Helper()
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_inventory WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_inventory WHERE tenant_id = $1 AND deleted_at IS NOT NULL`, tenantID).Scan(&tombstoned); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM runtime_target_storage_driver_evidence WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	return active, tombstoned, evidence
}

func seedStorageBinding(t *testing.T, db *sql.DB, tenantID, targetID, offeringID, bindingID, name, uid, resourceVersion string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO workload_storage_offerings
		(id,tenant_id,name,service_mode,access_modes,volume_expansion,snapshots,clones,protection_class)
		VALUES($1,$2,'test offering','Block','["ReadWriteOnce"]','Unknown','Unknown','Unknown','standard')`, offeringID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO storage_class_bindings
		(id,tenant_id,offering_id,offering_version,target_id,storage_class_name,storage_class_uid,
		 storage_class_resource_version,sync_state,is_default,source,observed_at,freshness)
		VALUES($1,$2,$3,1,$4,$5,$6,$7,'Imported',false,'platform.desired-state',now(),'Unknown')`,
		bindingID, tenantID, offeringID, targetID, name, uid, resourceVersion); err != nil {
		t.Fatal(err)
	}
}

func bindingDrift(t *testing.T, db *sql.DB, tenantID, bindingID string) (syncState, status, reason, freshness string) {
	t.Helper()
	if err := db.QueryRow(`SELECT sync_state,COALESCE(conditions->0->>'status',''),COALESCE(conditions->0->>'reason',''),COALESCE(conditions->0->>'freshness',freshness)
		FROM storage_class_bindings WHERE tenant_id=$1 AND id=$2`, tenantID, bindingID).
		Scan(&syncState, &status, &reason, &freshness); err != nil {
		t.Fatal(err)
	}
	return
}

func TestProjectorPGProjectsStorageBindingDriftFromOrderedInventory(t *testing.T) {
	cases := []struct {
		name, wantState, wantStatus, wantReason, wantFreshness string
		observedAt                                             time.Time
		facts                                                  []map[string]any
	}{
		{name: "in sync", wantState: "Active", wantStatus: "False", wantReason: "InSync", wantFreshness: "Fresh"},
		{name: "resource version changed", wantState: "Drifted", wantStatus: "True", wantReason: "StorageClassResourceVersionChanged", wantFreshness: "Fresh",
			facts: []map[string]any{{"uid": "sc-uid", "resourceVersion": "2", "name": "fast", "source": "kubernetes.storage.k8s.io/v1", "provisioner": "example.csi.io"}}},
		{name: "uid changed", wantState: "Drifted", wantStatus: "True", wantReason: "StorageClassUIDChanged", wantFreshness: "Fresh",
			facts: []map[string]any{{"uid": "replacement-uid", "resourceVersion": "1", "name": "fast", "source": "kubernetes.storage.k8s.io/v1", "provisioner": "example.csi.io"}}},
		{name: "missing class", wantState: "Drifted", wantStatus: "True", wantReason: "StorageClassMissing", wantFreshness: "Fresh", facts: []map[string]any{}},
		{name: "configuration changed", wantState: "Drifted", wantStatus: "True", wantReason: "StorageClassConfigurationChanged", wantFreshness: "Fresh",
			facts: []map[string]any{{"uid": "sc-uid", "resourceVersion": "1", "name": "fast", "source": "kubernetes.storage.k8s.io/v1", "provisioner": "example.csi.io", "isDefault": true}}},
		{name: "stale observation", wantState: "Imported", wantStatus: "Unknown", wantReason: "StaleObservation", wantFreshness: "Stale", observedAt: time.Now().UTC().Add(-time.Hour), facts: []map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openProjectorPG(t)
			tenantID := "tenant-binding-drift-" + uuid.NewString()[:8]
			targetID, offeringID, bindingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
			seedStorageBinding(t, db, tenantID, targetID, offeringID, bindingID, "fast", "sc-uid", "1")
			observedAt := tc.observedAt
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			facts := tc.facts
			if facts == nil {
				facts = []map[string]any{storageClassObservationFact(observedAt, "sc-uid", "1", "fast", "example.csi.io", false)}
			}
			for _, fact := range facts {
				fact["observedAt"] = observedAt
			}
			identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
			projector := NewProjector(NewPGCursorStore(db))
			if err := projector.Accept(context.Background(), identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt, facts...))); err != nil {
				t.Fatal(err)
			}
			state, status, reason, freshness := bindingDrift(t, db, tenantID, bindingID)
			if state != tc.wantState || status != tc.wantStatus || reason != tc.wantReason || freshness != tc.wantFreshness {
				t.Fatalf("drift=%s/%s/%s/%s want %s/%s/%s/%s", state, status, reason, freshness, tc.wantState, tc.wantStatus, tc.wantReason, tc.wantFreshness)
			}
		})
	}
}

func TestProjectorPGStorageBindingDriftIsTenantScopedAndReplaySafe(t *testing.T) {
	db := openProjectorPG(t)
	targetA, targetB := uuid.NewString(), uuid.NewString()
	tenantA, tenantB := "tenant-binding-a-"+uuid.NewString()[:8], "tenant-binding-b-"+uuid.NewString()[:8]
	seedProjectorTarget(t, db, tenantA, targetA, "kubernetes")
	seedProjectorTarget(t, db, tenantB, targetB, "kubernetes")
	bindingA := uuid.NewString()
	seedStorageBinding(t, db, tenantA, targetA, uuid.NewString(), bindingA, "fast", "uid-a", "1")
	observedAt := time.Now().UTC()
	identityB := Identity{TenantID: tenantB, TargetID: targetB, TargetKind: "KubernetesTarget", ObserverID: "agent-b", ObserverKind: "Agent"}
	payload := observationWithStorage(tenantB, targetB, identityB.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "uid-b", "2", "fast", "example.csi.io", false)))
	projector := NewProjector(NewPGCursorStore(db))
	if err := projector.Accept(context.Background(), identityB, payload); err != nil {
		t.Fatal(err)
	}
	if err := projector.Accept(context.Background(), identityB, payload); err != ErrReplay {
		t.Fatalf("replay err=%v", err)
	}
	state, _, _, freshness := bindingDrift(t, db, tenantA, bindingA)
	if state != "Imported" || freshness != "Unknown" {
		t.Fatalf("tenant B observation changed tenant A binding: %s/%s", state, freshness)
	}
}

func TestProjectorPGDuplicateStorageObservationIsIdempotent(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-duplicate-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6501"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	payload := observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "sc-uid", "1", "fast", "example.csi.io", false)))

	if err := projector.Accept(ctx, identity, payload); err != nil {
		t.Fatalf("accept storage observation: %v", err)
	}
	var updatedAt time.Time
	if err := db.QueryRow(`SELECT updated_at FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND target_id = $2 AND resource_uid = 'sc-uid'`, tenantID, targetID).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if err := projector.Accept(ctx, identity, payload); err != ErrReplay {
		t.Fatalf("duplicate err=%v want ErrReplay", err)
	}

	active, tombstoned, evidence := storageProjectionCounts(t, db, tenantID)
	var replayUpdatedAt time.Time
	var cursorSequence int64
	if err := db.QueryRow(`SELECT updated_at FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND target_id = $2 AND resource_uid = 'sc-uid'`, tenantID, targetID).Scan(&replayUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT observation_revision FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, identity.ObserverID).Scan(&cursorSequence); err != nil {
		t.Fatal(err)
	}
	if active != 1 || tombstoned != 0 || evidence != 1 || cursorSequence != 1 || !replayUpdatedAt.Equal(updatedAt) {
		t.Fatalf("duplicate changed projection: active/tombstoned/evidence/cursor=%d/%d/%d/%d updated=%v want 1/0/1/1 unchanged",
			active, tombstoned, evidence, cursorSequence, replayUpdatedAt)
	}
}

func TestProjectorPGStorageSequenceGapDoesNotProject(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-gap-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6502"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "sc-uid", "1", "fast", "example.csi.io", false)))); err != nil {
		t.Fatalf("accept initial storage observation: %v", err)
	}
	gap := observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 3, observedAt.Add(time.Second), "Delta", map[string]any{
		"storageClasses": []any{storageClassObservationFact(observedAt.Add(time.Second), "sc-uid", "1", "fast", "example.csi.io", true)},
	})
	if err := projector.Accept(ctx, identity, gap); err != ErrGap {
		t.Fatalf("gap err=%v want ErrGap", err)
	}

	active, tombstoned, evidence := storageProjectionCounts(t, db, tenantID)
	var cursorSequence int64
	var processingError string
	if err := db.QueryRow(`SELECT observation_revision FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, identity.ObserverID).Scan(&cursorSequence); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT processing_error FROM runtime_target_observation_inbox
		WHERE tenant_id = $1 AND target_id = $2 AND observation_revision = 3`, tenantID, targetID).Scan(&processingError); err != nil {
		t.Fatal(err)
	}
	if active != 1 || tombstoned != 0 || evidence != 1 || cursorSequence != 1 || processingError == "" {
		t.Fatalf("gap changed projection: active/tombstoned/evidence/cursor/error=%d/%d/%d/%d/%q",
			active, tombstoned, evidence, cursorSequence, processingError)
	}

	sequenceTwo := observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 2, observedAt.Add(500*time.Millisecond), "Delta", map[string]any{
		"storageClasses": []any{storageClassObservationFact(observedAt.Add(500*time.Millisecond), "sc-uid", "2", "fast", "example.csi.io", false)},
	})
	if err := projector.Accept(ctx, identity, sequenceTwo); err != nil {
		t.Fatalf("accept missing sequence: %v", err)
	}
	if err := projector.Accept(ctx, identity, gap); err != nil {
		t.Fatalf("accept deferred sequence after gap recovery: %v", err)
	}
	active, tombstoned, evidence = storageProjectionCounts(t, db, tenantID)
	var recoveredError sql.NullString
	var processed bool
	if err := db.QueryRow(`SELECT processing_error, processed_at IS NOT NULL FROM runtime_target_observation_inbox
		WHERE tenant_id = $1 AND target_id = $2 AND observation_revision = 3`, tenantID, targetID).Scan(&recoveredError, &processed); err != nil {
		t.Fatal(err)
	}
	if active != 0 || tombstoned != 1 || evidence != 0 || !processed || recoveredError.Valid {
		t.Fatalf("recovered gap projection active/tombstoned/evidence/processed/error=%d/%d/%d/%v/%v want 0/1/0/true/null",
			active, tombstoned, evidence, processed, recoveredError)
	}
}

func TestProjectorPGLowerStorageGenerationIsFenced(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-fenced-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6503"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "sc-uid", "1", "fast", "example.csi.io", false)))); err != nil {
		t.Fatalf("accept generation 1: %v", err)
	}
	reset := map[string]any{
		"schemaVersion": "1.0.0", "eventId": uuid.NewString(),
		"tenantId": tenantID, "targetId": targetID, "targetKind": "KubernetesTarget",
		"observerId": identity.ObserverID, "observerKind": "Agent",
		"previousObserverGeneration": 1, "newObserverGeneration": 2,
		"observedAt": observedAt.Add(time.Second), "observerLeaseId": uuid.NewString(), "reason": "observer-restarted",
	}
	resetPayload, _ := json.Marshal(reset)
	if err := projector.ApplyReset(ctx, identity, resetPayload); err != nil {
		t.Fatalf("apply source reset: %v", err)
	}
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 2, 1, observedAt.Add(2*time.Second), "Full", fullStorageInventory(observedAt.Add(2*time.Second),
		storageClassObservationFact(observedAt.Add(2*time.Second), "sc-uid", "2", "fast", "example.csi.io", false)))); err != nil {
		t.Fatalf("accept generation 2: %v", err)
	}
	stale := observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 2, observedAt.Add(3*time.Second), "Delta", map[string]any{
		"storageClasses": []any{storageClassObservationFact(observedAt.Add(3*time.Second), "sc-uid", "2", "fast", "example.csi.io", true)},
	})
	if err := projector.Accept(ctx, identity, stale); err != ErrFenced {
		t.Fatalf("lower generation err=%v want ErrFenced", err)
	}

	var resourceVersion string
	var deleted bool
	var cursorGeneration, cursorSequence int64
	if err := db.QueryRow(`SELECT resource_version, deleted_at IS NOT NULL FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND target_id = $2 AND resource_uid = 'sc-uid'`, tenantID, targetID).Scan(&resourceVersion, &deleted); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT observation_generation, observation_revision FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`, tenantID, targetID, identity.ObserverID).Scan(&cursorGeneration, &cursorSequence); err != nil {
		t.Fatal(err)
	}
	if resourceVersion != "2" || deleted || cursorGeneration != 2 || cursorSequence != 1 {
		t.Fatalf("stale generation changed projection: resourceVersion/deleted/cursor=%s/%v/%d:%d", resourceVersion, deleted, cursorGeneration, cursorSequence)
	}
}

func TestProjectorPGCrossTenantStorageObservationIsRejected(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantA := "tenant-storage-a-" + uuid.NewString()[:8]
	tenantB := "tenant-storage-b-" + uuid.NewString()[:8]
	targetA := "a15eba09-0a41-5b92-b972-69af1f0f6504"
	targetB := "a15eba09-0a41-5b92-b972-69af1f0f6505"
	seedProjectorTarget(t, db, tenantA, targetA, "kubernetes")
	seedProjectorTarget(t, db, tenantB, targetB, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identityA := Identity{TenantID: tenantA, TargetID: targetA, TargetKind: "KubernetesTarget", ObserverID: "agent-storage-a", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	payloadForB := observationWithStorage(tenantB, targetB, "agent-storage-b", 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "sc-tenant-b", "1", "fast", "example.csi.io", false)))
	if err := projector.Accept(ctx, identityA, payloadForB); err == nil {
		t.Fatal("expected cross-tenant payload/identity rejection")
	}

	for _, tenantID := range []string{tenantA, tenantB} {
		active, tombstoned, evidence := storageProjectionCounts(t, db, tenantID)
		var cursors int
		if err := db.QueryRow(`SELECT count(*) FROM runtime_target_observation_cursors WHERE tenant_id = $1`, tenantID).Scan(&cursors); err != nil {
			t.Fatal(err)
		}
		if active != 0 || tombstoned != 0 || evidence != 0 || cursors != 0 {
			t.Fatalf("cross-tenant rejection persisted tenant %s state: %d/%d/%d cursor=%d", tenantID, active, tombstoned, evidence, cursors)
		}
	}
}

func TestProjectorPGStorageFullDeltaAndTombstones(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-modes-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6506"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	fast := storageClassObservationFact(observedAt, "sc-fast", "1", "fast", "example.csi.io", false)
	slow := storageClassObservationFact(observedAt, "sc-slow", "1", "slow", "example.csi.io", false)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt, fast, slow))); err != nil {
		t.Fatalf("accept full: %v", err)
	}
	fastV2 := storageClassObservationFact(observedAt.Add(time.Second), "sc-fast", "2", "fast", "example.csi.io", false)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 2, observedAt.Add(time.Second), "Delta", map[string]any{
		"storageClasses": []any{fastV2},
	})); err != nil {
		t.Fatalf("accept delta update: %v", err)
	}
	active, tombstoned, _ := storageProjectionCounts(t, db, tenantID)
	if active != 2 || tombstoned != 0 {
		t.Fatalf("delta modified unlisted resource: active/tombstoned=%d/%d want 2/0", active, tombstoned)
	}
	slowTombstone := storageClassObservationFact(observedAt.Add(2*time.Second), "sc-slow", "1", "slow", "example.csi.io", true)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 3, observedAt.Add(2*time.Second), "Delta", map[string]any{
		"storageClasses": []any{slowTombstone},
	})); err != nil {
		t.Fatalf("accept delta tombstone: %v", err)
	}
	active, tombstoned, _ = storageProjectionCounts(t, db, tenantID)
	if active != 1 || tombstoned != 1 {
		t.Fatalf("delta tombstone counts=%d/%d want 1/1", active, tombstoned)
	}
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 4, observedAt.Add(3*time.Second), "Full", fullStorageInventory(observedAt.Add(3*time.Second)))); err != nil {
		t.Fatalf("accept empty full: %v", err)
	}
	active, tombstoned, _ = storageProjectionCounts(t, db, tenantID)
	if active != 0 || tombstoned != 2 {
		t.Fatalf("full omission tombstone counts=%d/%d want 0/2", active, tombstoned)
	}
}

func TestProjectorPGStorageClassRetainsMissingDriverEvidence(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-missing-driver-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6507"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", fullStorageInventory(observedAt,
		storageClassObservationFact(observedAt, "sc-missing-driver", "1", "orphaned", "missing.csi.io", false)))); err != nil {
		t.Fatalf("accept missing-driver inventory: %v", err)
	}

	var missingReferences int
	if err := db.QueryRow(`
		SELECT count(*)
		FROM runtime_target_storage_driver_evidence reference
		WHERE reference.tenant_id = $1 AND reference.target_id = $2
		  AND reference.evidence_kind = 'StorageClassReference' AND reference.deleted_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM runtime_target_storage_driver_evidence registration
		      WHERE registration.tenant_id = reference.tenant_id
		        AND registration.target_id = reference.target_id
		        AND registration.driver_name = reference.driver_name
		        AND registration.evidence_kind = 'CSIDriverRegistration'
		        AND registration.deleted_at IS NULL
		  )`, tenantID, targetID).Scan(&missingReferences); err != nil {
		t.Fatal(err)
	}
	active, tombstoned, evidence := storageProjectionCounts(t, db, tenantID)
	if missingReferences != 1 || active != 1 || tombstoned != 0 || evidence != 1 {
		t.Fatalf("missing-driver projection references/resources/evidence=%d/%d/%d/%d want 1/1/0/1",
			missingReferences, active, tombstoned, evidence)
	}
}

func TestProjectorPGStorageCapacityPreservesValueAbsenceAndFreshness(t *testing.T) {
	db := openProjectorPG(t)
	ctx := context.Background()
	tenantID := "tenant-storage-capacity-" + uuid.NewString()[:8]
	targetID := "a15eba09-0a41-5b92-b972-69af1f0f6508"
	seedProjectorTarget(t, db, tenantID, targetID, "kubernetes")
	projector := NewProjector(NewPGCursorStore(db))
	identity := Identity{TenantID: tenantID, TargetID: targetID, TargetKind: "KubernetesTarget", ObserverID: "agent-storage", ObserverKind: "Agent"}
	observedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	knownBytes := int64(10 * 1024 * 1024 * 1024)
	inventory := fullStorageInventory(observedAt)
	inventory["csiStorageCapacities"] = []any{
		storageCapacityObservationFact(observedAt, "capacity-known", "fast-zone-a", "fast", &knownBytes),
		storageCapacityObservationFact(observedAt, "capacity-unreported", "elastic-zone-b", "elastic", nil),
	}

	if err := projector.Accept(ctx, identity, observationWithStorage(tenantID, targetID, identity.ObserverID, 1, 1, observedAt, "Full", inventory)); err != nil {
		t.Fatalf("accept capacity inventory: %v", err)
	}

	var projectedKnown int64
	var knownStaleAfter, unreportedStaleAfter time.Time
	if err := db.QueryRow(`SELECT (attributes->>'capacityBytes')::bigint, stale_after
		FROM runtime_target_storage_inventory WHERE tenant_id = $1 AND resource_uid = 'capacity-known'`, tenantID).
		Scan(&projectedKnown, &knownStaleAfter); err != nil {
		t.Fatal(err)
	}
	var capacityFieldPresent bool
	if err := db.QueryRow(`SELECT attributes ? 'capacityBytes', stale_after
		FROM runtime_target_storage_inventory WHERE tenant_id = $1 AND resource_uid = 'capacity-unreported'`, tenantID).
		Scan(&capacityFieldPresent, &unreportedStaleAfter); err != nil {
		t.Fatal(err)
	}
	wantStaleAfter := observedAt.Add(300 * time.Second)
	if projectedKnown != knownBytes || capacityFieldPresent || !knownStaleAfter.Equal(wantStaleAfter) || !unreportedStaleAfter.Equal(wantStaleAfter) || !knownStaleAfter.Before(time.Now()) {
		t.Fatalf("capacity projection known/present/stale=%d/%v/%v/%v want %d/false/%v/stale",
			projectedKnown, capacityFieldPresent, knownStaleAfter, unreportedStaleAfter, knownBytes, wantStaleAfter)
	}
}
