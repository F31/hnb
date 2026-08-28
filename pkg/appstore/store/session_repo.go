package store

import (
	"database/sql"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

type UploadSessionRepo struct{ db *sql.DB }

func NewUploadSessionRepo(db *sql.DB) *UploadSessionRepo { return &UploadSessionRepo{db: db} }

func (r *UploadSessionRepo) Create(s *appstore.UploadSession) error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := r.db.Exec(`INSERT INTO upload_sessions
		(id, tenant_id, release_id, filename, artifact_type, size_bytes, status, harbor_url, repository, robot_id, robot_name, expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,$14)`,
		s.ID, s.TenantID, s.ReleaseID, s.Filename, s.ArtifactType, s.SizeBytes,
		string(s.Status), s.HarborURL, s.Repository, s.RobotID, s.RobotName, s.ExpiresAt, s.CreatedAt, s.UpdatedAt)
	return err
}

func (r *UploadSessionRepo) Get(id, tenantID string) (*appstore.UploadSession, error) {
	var s appstore.UploadSession
	var releaseID sql.NullString
	var robotName sql.NullString
	var artifactID sql.NullString
	var digest sql.NullString
	err := r.db.QueryRow(`SELECT id, tenant_id, release_id, filename, artifact_type, size_bytes, status, harbor_url, repository, robot_id, robot_name, artifact_id, digest, expires_at, created_at, updated_at
		FROM upload_sessions WHERE id=$1 AND tenant_id=$2`, id, tenantID).
		Scan(&s.ID, &s.TenantID, &releaseID, &s.Filename, &s.ArtifactType, &s.SizeBytes,
			&s.Status, &s.HarborURL, &s.Repository, &s.RobotID, &robotName, &artifactID, &digest,
			&s.ExpiresAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if robotName.Valid {
		s.RobotName = robotName.String
	}
	if releaseID.Valid {
		rid := releaseID.String
		s.ReleaseID = &rid
	}
	if artifactID.Valid {
		aid := artifactID.String
		s.ArtifactID = &aid
	}
	if digest.Valid {
		d := digest.String
		s.Digest = &d
	}
	return &s, nil
}

func (r *UploadSessionRepo) UpdateStatus(id, tenantID string, status appstore.UploadSessionStatus) error {
	result, err := r.db.Exec(`UPDATE upload_sessions SET status=$1, updated_at=NOW() WHERE id=$2 AND tenant_id=$3`,
		string(status), id, tenantID)
	return requireAffected(result, err)
}

func (r *UploadSessionRepo) ListExpired(limit int) ([]appstore.UploadSession, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, release_id, filename, artifact_type, size_bytes, status, harbor_url, repository, robot_id, robot_name, artifact_id, digest, expires_at, created_at, updated_at
		FROM upload_sessions WHERE status IN ('pending','uploading') AND expires_at < NOW() ORDER BY expires_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.UploadSession
	for rows.Next() {
		var s appstore.UploadSession
		var releaseID sql.NullString
		var robotName sql.NullString
		var artifactID sql.NullString
		var digest sql.NullString
		if err := rows.Scan(&s.ID, &s.TenantID, &releaseID, &s.Filename, &s.ArtifactType, &s.SizeBytes,
			&s.Status, &s.HarborURL, &s.Repository, &s.RobotID, &robotName, &artifactID, &digest,
			&s.ExpiresAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if robotName.Valid {
			s.RobotName = robotName.String
		}
		if releaseID.Valid {
			rid := releaseID.String
			s.ReleaseID = &rid
		}
		if artifactID.Valid {
			aid := artifactID.String
			s.ArtifactID = &aid
		}
		if digest.Valid {
			d := digest.String
			s.Digest = &d
		}
		res = append(res, s)
	}
	return res, nil
}
