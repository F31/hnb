package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/google/uuid"
)

type RecycleBinRepo struct {
	db *sql.DB
}

func NewRecycleBinRepo(db *sql.DB) *RecycleBinRepo {
	return &RecycleBinRepo{db: db}
}

func (r *RecycleBinRepo) EnsureSettings(tenantID string) (*appstore.RecycleBinSettings, error) {
	s := &appstore.RecycleBinSettings{
		TenantID:          tenantID,
		ProductRetention:  "7 days",
		ReleaseRetention:  "7 days",
		ArtifactRetention: "24 hours",
		Enabled:           true,
	}
	_, err := r.db.Exec(`INSERT INTO recycle_bin_settings (tenant_id, product_retention, release_retention, artifact_retention, enabled, updated_at)
		VALUES ($1,$2::interval,$3::interval,$4::interval,$5,NOW())
		ON CONFLICT (tenant_id) DO NOTHING`,
		tenantID, s.ProductRetention, s.ReleaseRetention, s.ArtifactRetention, s.Enabled)
	if err != nil {
		return nil, err
	}
	return r.GetSettings(tenantID)
}

func (r *RecycleBinRepo) GetSettings(tenantID string) (*appstore.RecycleBinSettings, error) {
	var s appstore.RecycleBinSettings
	var productRetention, releaseRetention, artifactRetention string
	err := r.db.QueryRow(`SELECT tenant_id, product_retention::text, release_retention::text, artifact_retention::text, enabled, updated_at
		FROM recycle_bin_settings WHERE tenant_id=$1`, tenantID).
		Scan(&s.TenantID, &productRetention, &releaseRetention, &artifactRetention, &s.Enabled, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.ProductRetention = productRetention
	s.ReleaseRetention = releaseRetention
	s.ArtifactRetention = artifactRetention
	return &s, nil
}

func (r *RecycleBinRepo) UpdateSettings(tenantID string, s *appstore.RecycleBinSettings) error {
	result, err := r.db.Exec(`UPDATE recycle_bin_settings SET product_retention=$2::interval, release_retention=$3::interval,
		artifact_retention=$4::interval, enabled=$5, updated_at=NOW() WHERE tenant_id=$1`,
		tenantID, s.ProductRetention, s.ReleaseRetention, s.ArtifactRetention, s.Enabled)
	return requireAffected(result, err)
}

func (r *RecycleBinRepo) Tombstone(tenantID, resourceType, resourceID, resourceName, retention string, operationID, actorID, reason string, originalData map[string]any) (*appstore.RecycleBinEntry, error) {
	var data []byte
	if originalData != nil {
		var err error
		data, err = json.Marshal(originalData)
		if err != nil {
			return nil, err
		}
	}
	entry := &appstore.RecycleBinEntry{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		State:        "retained",
		DeleteAfter:  time.Now().Add(parseInterval(retention)),
		OperationID:  operationID,
		RequestedBy:  actorID,
		Reason:       reason,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	var originalDataArg any = nil
	if len(data) > 0 {
		originalDataArg = data
	}
	var oidStr, reqByStr, reasonStr sql.NullString
	err := r.db.QueryRow(`INSERT INTO recycle_bin_entries (id, tenant_id, resource_type, resource_id, resource_name, state, delete_after, operation_id, requested_by, reason, original_data, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,NULLIF($10,''),$11,$12,$13)
		RETURNING id, tenant_id, resource_type, resource_id, resource_name, state, delete_after, operation_id, requested_by, reason, created_at, updated_at`,
		entry.ID, entry.TenantID, entry.ResourceType, entry.ResourceID, entry.ResourceName, entry.State, entry.DeleteAfter,
		entry.OperationID, entry.RequestedBy, entry.Reason, originalDataArg, entry.CreatedAt, entry.UpdatedAt).
		Scan(&entry.ID, &entry.TenantID, &entry.ResourceType, &entry.ResourceID, &entry.ResourceName, &entry.State, &entry.DeleteAfter,
			&oidStr, &reqByStr, &reasonStr, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if oidStr.Valid {
		entry.OperationID = oidStr.String
	}
	if reqByStr.Valid {
		entry.RequestedBy = reqByStr.String
	}
	if reasonStr.Valid {
		entry.Reason = reasonStr.String
	}
	return entry, nil
}

func (r *RecycleBinRepo) Cancel(id, tenantID string) error {
	result, err := r.db.Exec(`UPDATE recycle_bin_entries SET state='cancelled', updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND state='retained'`, id, tenantID)
	return requireAffected(result, err)
}

func (r *RecycleBinRepo) SweepPending(now time.Time) ([]appstore.RecycleBinEntry, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, resource_type, resource_id, resource_name, state, delete_after, operation_id, requested_by, reason, created_at, updated_at
		FROM recycle_bin_entries WHERE state='retained' AND delete_after <= $1 LIMIT 50`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (r *RecycleBinRepo) ListByTenant(tenantID, resourceType, state string, page, pageSize int) ([]appstore.RecycleBinEntry, int, error) {
	where := "WHERE tenant_id=$1"
	args := []any{tenantID}
	argIdx := 2
	if resourceType != "" {
		where += fmt.Sprintf(" AND resource_type=$%d", argIdx)
		args = append(args, resourceType)
		argIdx++
	}
	if state != "" {
		where += fmt.Sprintf(" AND state=$%d", argIdx)
		args = append(args, state)
		argIdx++
	}
	var total int
	if err := r.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM recycle_bin_entries %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := r.db.Query(fmt.Sprintf(`SELECT id, tenant_id, resource_type, resource_id, resource_name, state, delete_after, operation_id, requested_by, reason, created_at, updated_at
		FROM recycle_bin_entries %s ORDER BY delete_after ASC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries, err := scanEntries(rows)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RecycleBinRepo) GetEntry(id, tenantID string) (*appstore.RecycleBinEntry, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, resource_type, resource_id, resource_name, state, delete_after, operation_id, requested_by, reason, created_at, updated_at
		FROM recycle_bin_entries WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, sql.ErrNoRows
	}
	return &entries[0], nil
}

func (r *RecycleBinRepo) PurgeNow(id, tenantID string) error {
	result, err := r.db.Exec(`UPDATE recycle_bin_entries SET state='deleting', updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND state='retained'`, id, tenantID)
	return requireAffected(result, err)
}

func (r *RecycleBinRepo) MarkDeleted(id, tenantID string) error {
	result, err := r.db.Exec(`UPDATE recycle_bin_entries SET state='deleted', updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND state='deleting'`, id, tenantID)
	return requireAffected(result, err)
}

func (r *RecycleBinRepo) MarkDeleting(id, tenantID string) error {
	result, err := r.db.Exec(`UPDATE recycle_bin_entries SET state='deleting', updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND state='retained'`, id, tenantID)
	return requireAffected(result, err)
}

func scanEntries(rows *sql.Rows) ([]appstore.RecycleBinEntry, error) {
	var res []appstore.RecycleBinEntry
	for rows.Next() {
		var e appstore.RecycleBinEntry
		var operationID, requestedBy, reason sql.NullString
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ResourceType, &e.ResourceID, &e.ResourceName, &e.State, &e.DeleteAfter,
			&operationID, &requestedBy, &reason, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if operationID.Valid {
			e.OperationID = operationID.String
		}
		if requestedBy.Valid {
			e.RequestedBy = requestedBy.String
		}
		if reason.Valid {
			e.Reason = reason.String
		}
		res = append(res, e)
	}
	return res, rows.Err()
}

func parseInterval(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d
	}
	switch s {
	case "1 day", "1 days", "24 hours":
		return 24 * time.Hour
	case "7 days", "7 day":
		return 7 * 24 * time.Hour
	case "30 days", "1 month":
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}
