package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type storageInventoryQuery struct {
	Kind, Name, DriverName, Freshness string
	Limit, Offset                     int
}

type storageProjectionRow struct {
	Kind, UID, ResourceVersion, Name, Namespace, DriverName, Source string
	ObservedAt, StaleAfter                                          time.Time
	Attributes                                                      json.RawMessage
}

type storageSnapshotProjection struct {
	Status, APIVersion, Source string
	ObservedAt, StaleAfter     time.Time
}

type storageOverviewProjection struct {
	Targets, KnownCapacity, NotReportedCapacity int
	ObservedAt                                  *time.Time
	Fresh                                       bool
}

type storageMetricProjection struct {
	ProviderID, ResourceKind, ResourceUID string
	Metrics                               json.RawMessage
	StaleAfter                            time.Time
}

type storageStore interface {
	Overview(context.Context, string) (storageOverviewProjection, error)
	TargetOwned(context.Context, string, string) (bool, error)
	Inventory(context.Context, string, string, storageInventoryQuery) ([]storageProjectionRow, map[string]bool, *storageSnapshotProjection, error)
	Metrics(context.Context, string, string) ([]storageMetricProjection, error)
}

func (s *postgresStorageStore) Metrics(ctx context.Context, tenantID, targetID string) ([]storageMetricProjection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider_id, resource_kind, resource_uid, metrics, stale_after
		FROM storage_metric_snapshots
		WHERE tenant_id = $1 AND target_id = $2
		ORDER BY provider_id, resource_kind, resource_uid`, tenantID, targetID)
	if err != nil {
		return nil, fmt.Errorf("read target storage metrics: %w", err)
	}
	defer rows.Close()
	items := make([]storageMetricProjection, 0)
	for rows.Next() {
		var item storageMetricProjection
		if err := rows.Scan(&item.ProviderID, &item.ResourceKind, &item.ResourceUID, &item.Metrics, &item.StaleAfter); err != nil {
			return nil, fmt.Errorf("scan target storage metrics: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate target storage metrics: %w", err)
	}
	return items, nil
}

type postgresStorageStore struct{ db *sql.DB }

func NewPostgresStorageStore(db *sql.DB) storageStore { return &postgresStorageStore{db: db} }

func (s *postgresStorageStore) Overview(ctx context.Context, tenantID string) (storageOverviewProjection, error) {
	const query = `
		SELECT count(DISTINCT target_id), max(observed_at),
		       COALESCE(bool_and(stale_after > now()), false),
		       count(*) FILTER (WHERE resource_kind = 'CSIStorageCapacity' AND attributes ? 'capacityBytes'),
		       count(*) FILTER (WHERE resource_kind = 'CSIStorageCapacity' AND NOT (attributes ? 'capacityBytes'))
		FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND deleted_at IS NULL`
	var result storageOverviewProjection
	var observedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, query, tenantID).Scan(
		&result.Targets, &observedAt, &result.Fresh, &result.KnownCapacity, &result.NotReportedCapacity,
	); err != nil {
		return result, fmt.Errorf("read storage overview projection: %w", err)
	}
	if observedAt.Valid {
		value := observedAt.Time.UTC()
		result.ObservedAt = &value
	}
	return result, nil
}

func (s *postgresStorageStore) TargetOwned(ctx context.Context, tenantID, targetID string) (bool, error) {
	var owned bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM runtime_targets WHERE id = $1 AND tenant_id = $2 AND is_active = true
	)`, targetID, tenantID).Scan(&owned)
	if err != nil {
		return false, fmt.Errorf("check storage target ownership: %w", err)
	}
	return owned, nil
}

func (s *postgresStorageStore) Inventory(ctx context.Context, tenantID, targetID string, filter storageInventoryQuery) ([]storageProjectionRow, map[string]bool, *storageSnapshotProjection, error) {
	const query = `
		SELECT resource_kind, resource_uid, resource_version, name,
		       COALESCE(namespace, ''), COALESCE(driver_name, ''), source,
		       observed_at, stale_after, attributes
		FROM runtime_target_storage_inventory
		WHERE tenant_id = $1 AND target_id = $2 AND deleted_at IS NULL
		  AND ($3 = '' OR resource_kind = $3)
		  AND ($4 = '' OR name ILIKE '%' || $4 || '%')
		  AND ($5 = '' OR driver_name = $5)
		  AND ($6 = '' OR CASE WHEN stale_after > now() THEN 'Fresh' ELSE 'Stale' END = $6)
		ORDER BY resource_kind, name, resource_uid
		LIMIT $7 OFFSET $8`
	rows, err := s.db.QueryContext(ctx, query, tenantID, targetID, filter.Kind, filter.Name, filter.DriverName, filter.Freshness, filter.Limit, filter.Offset)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read target storage inventory: %w", err)
	}
	defer rows.Close()
	items := make([]storageProjectionRow, 0)
	for rows.Next() {
		var item storageProjectionRow
		if err := rows.Scan(&item.Kind, &item.UID, &item.ResourceVersion, &item.Name, &item.Namespace,
			&item.DriverName, &item.Source, &item.ObservedAt, &item.StaleAfter, &item.Attributes); err != nil {
			return nil, nil, nil, fmt.Errorf("scan target storage inventory: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate target storage inventory: %w", err)
	}

	registrations := make(map[string]bool)
	evidenceRows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT driver_name
		FROM runtime_target_storage_driver_evidence
		WHERE tenant_id = $1 AND target_id = $2 AND evidence_kind = 'CSIDriverRegistration'
		  AND deleted_at IS NULL`, tenantID, targetID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read storage driver evidence: %w", err)
	}
	defer evidenceRows.Close()
	for evidenceRows.Next() {
		var driver string
		if err := evidenceRows.Scan(&driver); err != nil {
			return nil, nil, nil, fmt.Errorf("scan storage driver evidence: %w", err)
		}
		registrations[driver] = true
	}
	if err := evidenceRows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate storage driver evidence: %w", err)
	}

	var snapshot storageSnapshotProjection
	var apiVersion sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT status, api_version, source, observed_at, stale_after
		FROM runtime_target_storage_snapshot_api
		WHERE tenant_id = $1 AND target_id = $2`, tenantID, targetID).Scan(
		&snapshot.Status, &apiVersion, &snapshot.Source, &snapshot.ObservedAt, &snapshot.StaleAfter,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, nil, fmt.Errorf("read storage snapshot API projection: %w", err)
	}
	if err == nil {
		snapshot.APIVersion = apiVersion.String
		return items, registrations, &snapshot, nil
	}
	return items, registrations, nil, nil
}
