package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/google/uuid"
)

type SecurityRepo struct {
	db *sql.DB
}

func NewSecurityRepo(db *sql.DB) *SecurityRepo {
	return &SecurityRepo{db: db}
}

func (r *SecurityRepo) ListProfiles(tenantID string) ([]appstore.ScanProfile, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, name, engine, config, enabled, is_default, created_by, created_at, updated_at FROM artifact_scan_profiles WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProfiles(rows)
}

func (r *SecurityRepo) GetProfile(id, tenantID string) (*appstore.ScanProfile, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, name, engine, config, enabled, is_default, created_by, created_at, updated_at FROM artifact_scan_profiles WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles, err := scanProfiles(rows)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, sql.ErrNoRows
	}
	return &profiles[0], nil
}

func (r *SecurityRepo) CreateProfile(p *appstore.ScanProfile) error {
	config, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	p.ID = uuid.NewString()
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	_, err = r.db.Exec(`INSERT INTO artifact_scan_profiles (id, tenant_id, name, engine, config, enabled, is_default, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		p.ID, p.TenantID, p.Name, p.Engine, config, p.Enabled, p.IsDefault, p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *SecurityRepo) UpdateProfile(id, tenantID string, p *appstore.ScanProfile) error {
	config, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(`UPDATE artifact_scan_profiles SET name=$3, engine=$4, config=$5, enabled=$6, is_default=$7, updated_at=NOW() WHERE id=$1 AND tenant_id=$2`,
		id, tenantID, p.Name, p.Engine, config, p.Enabled, p.IsDefault)
	return requireAffected(result, err)
}

func (r *SecurityRepo) DeleteProfile(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM artifact_scan_profiles WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return requireAffected(result, err)
}

func (r *SecurityRepo) GetDefaultProfile(tenantID string) (*appstore.ScanProfile, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, name, engine, config, enabled, is_default, created_by, created_at, updated_at FROM artifact_scan_profiles WHERE tenant_id=$1 AND is_default=true AND enabled=true LIMIT 1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles, err := scanProfiles(rows)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, sql.ErrNoRows
	}
	return &profiles[0], nil
}

func (r *SecurityRepo) GetDBStatus(tenantID string) ([]appstore.VulnerabilityDB, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, profile_id, engine, db_label, policy, last_sync_at, next_sync_at, status, last_error, created_at, updated_at FROM artifact_vulnerability_db WHERE tenant_id=$1 ORDER BY last_sync_at DESC NULLS LAST`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVulnDBs(rows)
}

func (r *SecurityRepo) UpsertDBStatus(db *appstore.VulnerabilityDB) error {
	db.ID = uuid.NewString()
	_, err := r.db.Exec(`INSERT INTO artifact_vulnerability_db (id, tenant_id, profile_id, engine, db_label, policy, last_sync_at, next_sync_at, status, last_error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())
		ON CONFLICT (tenant_id, profile_id, db_label) DO UPDATE SET status=$9, last_sync_at=$7, next_sync_at=$8, last_error=$10, updated_at=NOW()`,
		db.ID, db.TenantID, db.ProfileID, db.Engine, db.DbLabel, db.Policy, db.LastSyncAt, db.NextSyncAt, db.Status, db.LastError)
	return err
}

func (r *SecurityRepo) ListReports(tenantID, artifactID, profileID, state string, limit, offset int) ([]appstore.ScanReport, int, error) {
	where := "WHERE tenant_id=$1"
	args := []any{tenantID}
	argIdx := 2
	if artifactID != "" {
		where += fmt.Sprintf(" AND artifact_id=$%d", argIdx)
		args = append(args, artifactID)
		argIdx++
	}
	if profileID != "" {
		where += fmt.Sprintf(" AND profile_id=$%d", argIdx)
		args = append(args, profileID)
		argIdx++
	}
	if state != "" {
		where += fmt.Sprintf(" AND state=$%d", argIdx)
		args = append(args, state)
		argIdx++
	}
	var total int
	if err := r.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM artifact_scan_reports %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := r.db.Query(fmt.Sprintf(`SELECT id, tenant_id, artifact_id, profile_id, state, severity_summary, findings, triggered_by, error_message, started_at, completed_at, created_at, updated_at FROM artifact_scan_reports %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	reports, err := scanReports(rows)
	if err != nil {
		return nil, 0, err
	}
	return reports, total, nil
}

func (r *SecurityRepo) GetReport(id, tenantID string) (*appstore.ScanReport, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, artifact_id, profile_id, state, severity_summary, findings, triggered_by, error_message, started_at, completed_at, created_at, updated_at FROM artifact_scan_reports WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports, err := scanReports(rows)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, sql.ErrNoRows
	}
	return &reports[0], nil
}

func (r *SecurityRepo) EnqueueScan(tenantID, artifactID, profileID, triggeredBy string) (*appstore.ScanReport, error) {
	report := &appstore.ScanReport{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		ArtifactID:  artifactID,
		ProfileID:   profileID,
		State:       "queued",
		TriggeredBy: triggeredBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_, err := r.db.Exec(`INSERT INTO artifact_scan_reports (id, tenant_id, artifact_id, profile_id, state, triggered_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, report.ID, report.TenantID, report.ArtifactID, report.ProfileID, report.State, report.TriggeredBy, report.CreatedAt, report.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (r *SecurityRepo) UpdateReportState(id, tenantID, state string, severitySummary, findings []byte, errorMessage string) error {
	query := `UPDATE artifact_scan_reports SET state=$3, updated_at=NOW()`
	args := []any{id, tenantID, state}
	argIdx := 4
	if severitySummary != nil {
		query += fmt.Sprintf(", severity_summary=$%d", argIdx)
		args = append(args, severitySummary)
		argIdx++
	}
	if findings != nil {
		query += fmt.Sprintf(", findings=$%d", argIdx)
		args = append(args, findings)
		argIdx++
	}
	if errorMessage != "" {
		query += fmt.Sprintf(", error_message=$%d", argIdx)
		args = append(args, errorMessage)
		argIdx++
	}
	if state == "running" {
		query += ", started_at=NOW()"
	}
	if state == "completed" || state == "failed" {
		query += ", completed_at=NOW()"
	}
	query += fmt.Sprintf(" WHERE id=$1 AND tenant_id=$2")
	result, err := r.db.Exec(query, args...)
	return requireAffected(result, err)
}

func (r *SecurityRepo) ClaimQueued(limit int) ([]appstore.ScanReport, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, artifact_id, profile_id, state, severity_summary, findings, triggered_by, error_message, started_at, completed_at, created_at, updated_at FROM artifact_scan_reports WHERE state='queued' ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReports(rows)
}

func scanProfiles(rows *sql.Rows) ([]appstore.ScanProfile, error) {
	res := make([]appstore.ScanProfile, 0)
	for rows.Next() {
		var p appstore.ScanProfile
		var config []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Engine, &config, &p.Enabled, &p.IsDefault, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if len(config) > 0 {
			_ = json.Unmarshal(config, &p.Config)
		}
		res = append(res, p)
	}
	return res, rows.Err()
}

func scanVulnDBs(rows *sql.Rows) ([]appstore.VulnerabilityDB, error) {
	res := make([]appstore.VulnerabilityDB, 0)
	for rows.Next() {
		var db appstore.VulnerabilityDB
		var lastSyncAt, nextSyncAt sql.NullTime
		var lastError sql.NullString
		if err := rows.Scan(&db.ID, &db.TenantID, &db.ProfileID, &db.Engine, &db.DbLabel, &db.Policy, &lastSyncAt, &nextSyncAt, &db.Status, &lastError, &db.CreatedAt, &db.UpdatedAt); err != nil {
			return nil, err
		}
		if lastSyncAt.Valid {
			db.LastSyncAt = &lastSyncAt.Time
		}
		if nextSyncAt.Valid {
			db.NextSyncAt = &nextSyncAt.Time
		}
		if lastError.Valid {
			db.LastError = lastError.String
		}
		res = append(res, db)
	}
	return res, rows.Err()
}

func scanReports(rows *sql.Rows) ([]appstore.ScanReport, error) {
	res := make([]appstore.ScanReport, 0)
	for rows.Next() {
		var r appstore.ScanReport
		var severitySummary, findings []byte
		var triggeredBy, errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ArtifactID, &r.ProfileID, &r.State, &severitySummary, &findings, &triggeredBy, &errorMessage, &startedAt, &completedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if triggeredBy.Valid {
			r.TriggeredBy = triggeredBy.String
		}
		if errorMessage.Valid {
			r.ErrorMessage = errorMessage.String
		}
		if startedAt.Valid {
			r.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		if len(severitySummary) > 0 {
			_ = json.Unmarshal(severitySummary, &r.SeveritySummary)
		}
		if len(findings) > 0 {
			_ = json.Unmarshal(findings, &r.Findings)
		}
		res = append(res, r)
	}
	return res, rows.Err()
}

func (r *SecurityRepo) CountAffectedImages(tenantID string, cve string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(DISTINCT artifact_id) FROM artifact_scan_reports WHERE tenant_id=$1 AND findings::text LIKE $2 AND state='completed'`, tenantID, "%"+cve+"%").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
