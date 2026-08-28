package observer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/lib/pq"
)

var storageResourceKinds = []string{
	"StorageClass", "CSIDriver", "CSINode", "CSIStorageCapacity",
	"VolumeAttachment", "VolumeSnapshotClass", "VolumeSnapshot", "VolumeSnapshotContent",
}

type storageProjectionRecord struct {
	Kind       string
	Identity   KubernetesResourceIdentity
	Namespace  string
	DriverName string
	Attributes []byte
	Evidence   []storageDriverEvidence
}

type storageDriverEvidence struct {
	Kind       string
	DriverName string
	Details    []byte
}

func projectStorageInventory(ctx context.Context, tx *sql.Tx, o *Observation, identity Identity) error {
	records, err := storageProjectionRecords(o.StorageInventory)
	if err != nil {
		return err
	}
	var staleThresholdSeconds int64
	if err := tx.QueryRowContext(ctx, `
		SELECT stale_threshold_seconds FROM runtime_targets
		WHERE id = $1 AND tenant_id = $2`, o.TargetID, o.TenantID).Scan(&staleThresholdSeconds); err != nil {
		return fmt.Errorf("load storage freshness threshold: %w", err)
	}

	present := make(map[string][]string, len(storageResourceKinds))
	for _, kind := range storageResourceKinds {
		present[kind] = []string{}
	}
	for _, record := range records {
		present[record.Kind] = append(present[record.Kind], record.Identity.UID)
		if record.Identity.Deleted {
			if err := tombstoneStorageResource(ctx, tx, o, record.Kind, record.Identity.UID); err != nil {
				return err
			}
			continue
		}
		staleAfter := record.Identity.ObservedAt.Add(time.Duration(staleThresholdSeconds) * time.Second)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runtime_target_storage_inventory (
				tenant_id, target_id, resource_kind, resource_uid, resource_version,
				name, namespace, driver_name, source, observed_at, stale_after,
				observation_source, observation_source_id, observation_generation,
				observation_revision, attributes, deleted_at
			) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,$11,'agent',$12,$13,$14,$15::jsonb,NULL)
			ON CONFLICT (tenant_id, target_id, resource_kind, resource_uid) DO UPDATE SET
				resource_version = EXCLUDED.resource_version, name = EXCLUDED.name,
				namespace = EXCLUDED.namespace, driver_name = EXCLUDED.driver_name,
				source = EXCLUDED.source, observed_at = EXCLUDED.observed_at,
				stale_after = EXCLUDED.stale_after,
				observation_source = EXCLUDED.observation_source,
				observation_source_id = EXCLUDED.observation_source_id,
				observation_generation = EXCLUDED.observation_generation,
				observation_revision = EXCLUDED.observation_revision,
				attributes = EXCLUDED.attributes, deleted_at = NULL, updated_at = now()`,
			o.TenantID, o.TargetID, record.Kind, record.Identity.UID, record.Identity.ResourceVersion,
			record.Identity.Name, record.Namespace, record.DriverName, record.Identity.Source,
			record.Identity.ObservedAt, staleAfter, identity.ObserverID,
			o.ObserverGeneration, o.Sequence, string(record.Attributes)); err != nil {
			return fmt.Errorf("upsert storage %s: %w", record.Kind, err)
		}
		if err := replaceStorageDriverEvidence(ctx, tx, o, identity, record, staleAfter); err != nil {
			return err
		}
	}

	if o.InventoryMode == "Full" {
		for _, kind := range storageResourceKinds {
			if _, err := tx.ExecContext(ctx, `
				UPDATE runtime_target_storage_inventory
				SET deleted_at = $1, observation_generation = $2,
				    observation_revision = $3, updated_at = now()
				WHERE tenant_id = $4 AND target_id = $5 AND resource_kind = $6
				  AND deleted_at IS NULL AND NOT (resource_uid = ANY($7))`,
				o.ObservedAt, o.ObserverGeneration, o.Sequence, o.TenantID, o.TargetID,
				kind, pq.Array(present[kind])); err != nil {
				return fmt.Errorf("tombstone missing storage %s: %w", kind, err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE runtime_target_storage_driver_evidence
				SET deleted_at = $1, observation_generation = $2,
				    observation_revision = $3, updated_at = now()
				WHERE tenant_id = $4 AND target_id = $5 AND resource_kind = $6
				  AND deleted_at IS NULL AND NOT (resource_uid = ANY($7))`,
				o.ObservedAt, o.ObserverGeneration, o.Sequence, o.TenantID, o.TargetID,
				kind, pq.Array(present[kind])); err != nil {
				return fmt.Errorf("tombstone missing %s driver evidence: %w", kind, err)
			}
		}
	}

	if snapshot := o.StorageInventory.SnapshotAPI; snapshot != nil {
		staleAfter := snapshot.ObservedAt.Add(time.Duration(staleThresholdSeconds) * time.Second)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runtime_target_storage_snapshot_api (
				tenant_id, target_id, status, api_version, source, observed_at, stale_after,
				observation_source, observation_source_id, observation_generation, observation_revision
			) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,'agent',$8,$9,$10)
			ON CONFLICT (tenant_id, target_id) DO UPDATE SET
				status = EXCLUDED.status, api_version = EXCLUDED.api_version,
				source = EXCLUDED.source, observed_at = EXCLUDED.observed_at,
				stale_after = EXCLUDED.stale_after,
				observation_source = EXCLUDED.observation_source,
				observation_source_id = EXCLUDED.observation_source_id,
				observation_generation = EXCLUDED.observation_generation,
				observation_revision = EXCLUDED.observation_revision, updated_at = now()`,
			o.TenantID, o.TargetID, snapshot.Status, snapshot.APIVersion, snapshot.Source,
			snapshot.ObservedAt, staleAfter, identity.ObserverID, o.ObserverGeneration, o.Sequence); err != nil {
			return fmt.Errorf("upsert snapshot API evidence: %w", err)
		}
	}
	return projectStorageBindingDrift(ctx, tx, o, staleThresholdSeconds)
}

type bindingDriftRow struct {
	ID, StorageClassName, StorageClassUID, StorageClassResourceVersion string
	IsDefault                                                          bool
	Topology                                                           map[string][]string
}

type observedStorageClass struct {
	UID, ResourceVersion   string
	ObservedAt, StaleAfter time.Time
	Generation, Revision   int64
	Attributes             []byte
}

type observedStorageClassConfiguration struct {
	IsDefault         bool                `json:"isDefault"`
	AllowedTopologies []map[string]string `json:"allowedTopologies"`
}

func projectStorageBindingDrift(ctx context.Context, tx *sql.Tx, o *Observation, staleThresholdSeconds int64) error {
	now := time.Now()
	rows, err := tx.QueryContext(ctx, `
		SELECT id, storage_class_name, storage_class_uid, storage_class_resource_version, is_default, topology
		FROM storage_class_bindings
		WHERE tenant_id = $1 AND target_id = $2 AND sync_state <> 'Retired'
		ORDER BY id`, o.TenantID, o.TargetID)
	if err != nil {
		return fmt.Errorf("load storage bindings for drift: %w", err)
	}
	bindings := []bindingDriftRow{}
	for rows.Next() {
		var binding bindingDriftRow
		var topology []byte
		if err := rows.Scan(&binding.ID, &binding.StorageClassName, &binding.StorageClassUID, &binding.StorageClassResourceVersion, &binding.IsDefault, &topology); err != nil {
			rows.Close()
			return fmt.Errorf("scan storage binding for drift: %w", err)
		}
		if err := json.Unmarshal(topology, &binding.Topology); err != nil {
			rows.Close()
			return fmt.Errorf("decode storage binding topology: %w", err)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, binding := range bindings {
		observed, found, err := latestObservedStorageClass(ctx, tx, o.TenantID, o.TargetID, binding.StorageClassName)
		if err != nil {
			return err
		}
		status, reason, freshness := "False", "InSync", "Fresh"
		observedAt := o.ObservedAt
		generation, revision := o.ObserverGeneration, o.Sequence
		if !found {
			if o.ObservedAt.Add(time.Duration(staleThresholdSeconds) * time.Second).Before(now) {
				status, reason, freshness = "Unknown", "StaleObservation", "Stale"
			} else {
				status, reason = "True", "StorageClassMissing"
			}
		} else {
			observedAt, generation, revision = observed.ObservedAt, observed.Generation, observed.Revision
			if observed.StaleAfter.Before(now) {
				status, reason, freshness = "Unknown", "StaleObservation", "Stale"
			} else if observed.UID != binding.StorageClassUID {
				status, reason = "True", "StorageClassUIDChanged"
			} else if observed.ResourceVersion != binding.StorageClassResourceVersion {
				status, reason = "True", "StorageClassResourceVersionChanged"
			} else {
				var configuration observedStorageClassConfiguration
				if err := json.Unmarshal(observed.Attributes, &configuration); err != nil {
					return fmt.Errorf("decode observed StorageClass configuration: %w", err)
				}
				if configuration.IsDefault != binding.IsDefault || !reflect.DeepEqual(sortedStorageTopology(binding.Topology), storageClassTopology(configuration.AllowedTopologies)) {
					status, reason = "True", "StorageClassConfigurationChanged"
				}
			}
		}
		condition, _ := json.Marshal([]map[string]any{{
			"schemaVersion": "1.0.0", "type": "Drifted", "status": status, "reason": reason,
			"source": "runtime_target_storage_inventory", "observedAt": observedAt.UTC(), "freshness": freshness,
		}})
		syncState := "Active"
		if status == "True" {
			syncState = "Drifted"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE storage_class_bindings
			SET sync_state = CASE WHEN $1 = 'Unknown' THEN sync_state ELSE $2 END,
			    source = 'runtime_target_storage_inventory', observed_at = $3, freshness = $4,
			    conditions = $5::jsonb, observation_generation = $6, observation_revision = $7,
			    updated_at = now()
			WHERE tenant_id = $8 AND target_id = $9 AND id = $10`,
			status, syncState, observedAt, freshness, string(condition), generation, revision,
			o.TenantID, o.TargetID, binding.ID); err != nil {
			return fmt.Errorf("project storage binding drift: %w", err)
		}
	}
	return nil
}

func latestObservedStorageClass(ctx context.Context, tx *sql.Tx, tenantID, targetID, name string) (observedStorageClass, bool, error) {
	var observed observedStorageClass
	err := tx.QueryRowContext(ctx, `
		SELECT resource_uid, resource_version, observed_at, stale_after,
		       observation_generation, observation_revision, attributes
		FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND target_id = $2 AND resource_kind = 'StorageClass'
		  AND name = $3 AND deleted_at IS NULL
		ORDER BY observation_generation DESC, observation_revision DESC, resource_uid
		LIMIT 1`, tenantID, targetID, name).Scan(
		&observed.UID, &observed.ResourceVersion, &observed.ObservedAt, &observed.StaleAfter,
		&observed.Generation, &observed.Revision, &observed.Attributes)
	if err == sql.ErrNoRows {
		return observedStorageClass{}, false, nil
	}
	if err != nil {
		return observedStorageClass{}, false, fmt.Errorf("load observed StorageClass for drift: %w", err)
	}
	return observed, true, nil
}

func storageClassTopology(allowed []map[string]string) map[string][]string {
	result := map[string][]string{}
	for _, term := range allowed {
		for key, value := range term {
			result[key] = append(result[key], value)
		}
	}
	return sortedStorageTopology(result)
}

func sortedStorageTopology(topology map[string][]string) map[string][]string {
	result := make(map[string][]string, len(topology))
	for key, values := range topology {
		result[key] = append([]string(nil), values...)
		sort.Strings(result[key])
	}
	return result
}

func tombstoneStorageResource(ctx context.Context, tx *sql.Tx, o *Observation, kind, uid string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE runtime_target_storage_inventory
		SET deleted_at = $1, observation_generation = $2, observation_revision = $3, updated_at = now()
		WHERE tenant_id = $4 AND target_id = $5 AND resource_kind = $6 AND resource_uid = $7`,
		o.ObservedAt, o.ObserverGeneration, o.Sequence, o.TenantID, o.TargetID, kind, uid); err != nil {
		return fmt.Errorf("tombstone storage %s: %w", kind, err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE runtime_target_storage_driver_evidence
		SET deleted_at = $1, observation_generation = $2, observation_revision = $3, updated_at = now()
		WHERE tenant_id = $4 AND target_id = $5 AND resource_kind = $6 AND resource_uid = $7`,
		o.ObservedAt, o.ObserverGeneration, o.Sequence, o.TenantID, o.TargetID, kind, uid); err != nil {
		return fmt.Errorf("tombstone %s driver evidence: %w", kind, err)
	}
	return nil
}

func replaceStorageDriverEvidence(ctx context.Context, tx *sql.Tx, o *Observation, identity Identity, record storageProjectionRecord, staleAfter time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE runtime_target_storage_driver_evidence
		SET deleted_at = $1, observation_generation = $2, observation_revision = $3, updated_at = now()
		WHERE tenant_id = $4 AND target_id = $5 AND resource_kind = $6
		  AND resource_uid = $7 AND deleted_at IS NULL`,
		o.ObservedAt, o.ObserverGeneration, o.Sequence, o.TenantID, o.TargetID,
		record.Kind, record.Identity.UID); err != nil {
		return fmt.Errorf("replace %s driver evidence: %w", record.Kind, err)
	}
	for _, evidence := range record.Evidence {
		if evidence.DriverName == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runtime_target_storage_driver_evidence (
				tenant_id, target_id, evidence_kind, resource_kind, resource_uid,
				driver_name, source, observed_at, stale_after, observation_source,
				observation_source_id, observation_generation, observation_revision,
				details, deleted_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'agent',$10,$11,$12,$13::jsonb,NULL)
			ON CONFLICT (tenant_id, target_id, evidence_kind, resource_uid, driver_name) DO UPDATE SET
				resource_kind = EXCLUDED.resource_kind, source = EXCLUDED.source,
				observed_at = EXCLUDED.observed_at, stale_after = EXCLUDED.stale_after,
				observation_source = EXCLUDED.observation_source,
				observation_source_id = EXCLUDED.observation_source_id,
				observation_generation = EXCLUDED.observation_generation,
				observation_revision = EXCLUDED.observation_revision,
				details = EXCLUDED.details, deleted_at = NULL, updated_at = now()`,
			o.TenantID, o.TargetID, evidence.Kind, record.Kind, record.Identity.UID,
			evidence.DriverName, record.Identity.Source, record.Identity.ObservedAt,
			staleAfter, identity.ObserverID, o.ObserverGeneration, o.Sequence,
			string(evidence.Details)); err != nil {
			return fmt.Errorf("upsert %s driver evidence: %w", evidence.Kind, err)
		}
	}
	return nil
}

func storageProjectionRecords(inventory *StorageInventory) ([]storageProjectionRecord, error) {
	if inventory == nil {
		return nil, nil
	}
	records := make([]storageProjectionRecord, 0)
	for _, fact := range inventory.StorageClasses {
		record, err := newStorageProjectionRecord("StorageClass", fact.KubernetesResourceIdentity, "", fact.Provisioner, fact)
		if err != nil {
			return nil, err
		}
		record.Evidence = evidence("StorageClassReference", fact.Provisioner, fact)
		records = append(records, record)
	}
	for _, fact := range inventory.CSIDrivers {
		record, err := newStorageProjectionRecord("CSIDriver", fact.KubernetesResourceIdentity, "", fact.Name, fact)
		if err != nil {
			return nil, err
		}
		record.Evidence = evidence("CSIDriverRegistration", fact.Name, fact)
		records = append(records, record)
	}
	for _, fact := range inventory.CSINodes {
		record, err := newStorageProjectionRecord("CSINode", fact.KubernetesResourceIdentity, "", "", fact)
		if err != nil {
			return nil, err
		}
		for _, driver := range fact.Drivers {
			record.Evidence = append(record.Evidence, evidence("CSINodeRegistration", driver.Name, driver)...)
		}
		records = append(records, record)
	}
	for _, fact := range inventory.CSIStorageCapacities {
		record, err := newStorageProjectionRecord("CSIStorageCapacity", fact.KubernetesResourceIdentity, fact.Namespace, "", fact)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	for _, fact := range inventory.VolumeAttachments {
		record, err := newStorageProjectionRecord("VolumeAttachment", fact.KubernetesResourceIdentity, "", fact.Attacher, fact)
		if err != nil {
			return nil, err
		}
		record.Evidence = evidence("VolumeAttachmentReference", fact.Attacher, fact)
		records = append(records, record)
	}
	for _, fact := range inventory.VolumeSnapshotClasses {
		record, err := newStorageProjectionRecord("VolumeSnapshotClass", fact.KubernetesResourceIdentity, "", fact.Driver, fact)
		if err != nil {
			return nil, err
		}
		record.Evidence = evidence("VolumeSnapshotClassReference", fact.Driver, fact)
		records = append(records, record)
	}
	for _, fact := range inventory.VolumeSnapshots {
		record, err := newStorageProjectionRecord("VolumeSnapshot", fact.KubernetesResourceIdentity, fact.Namespace, "", fact)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	for _, fact := range inventory.VolumeSnapshotContents {
		record, err := newStorageProjectionRecord("VolumeSnapshotContent", fact.KubernetesResourceIdentity, "", fact.Driver, fact)
		if err != nil {
			return nil, err
		}
		record.Evidence = evidence("VolumeSnapshotContentReference", fact.Driver, fact)
		records = append(records, record)
	}
	return records, nil
}

func newStorageProjectionRecord(kind string, identity KubernetesResourceIdentity, namespace, driverName string, fact any) (storageProjectionRecord, error) {
	attributes, err := json.Marshal(fact)
	if err != nil {
		return storageProjectionRecord{}, fmt.Errorf("marshal storage %s: %w", kind, err)
	}
	return storageProjectionRecord{Kind: kind, Identity: identity, Namespace: namespace, DriverName: driverName, Attributes: attributes}, nil
}

func evidence(kind, driverName string, details any) []storageDriverEvidence {
	if driverName == "" {
		return nil
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return nil
	}
	return []storageDriverEvidence{{Kind: kind, DriverName: driverName, Details: encoded}}
}
