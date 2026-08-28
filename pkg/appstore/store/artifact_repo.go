package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

var (
	ErrUploadSessionExpired  = errors.New("upload session expired")
	ErrUploadSessionState    = errors.New("upload session is not confirmable")
	ErrUploadReleaseState    = errors.New("upload release is not draft")
	ErrArtifactNotAttachable = errors.New("artifact must be verified and active")
)

type ArtifactRepo struct{ db *sql.DB }

func NewArtifactRepo(db *sql.DB) *ArtifactRepo { return &ArtifactRepo{db: db} }

func (r *ArtifactRepo) Create(a *appstore.ArtifactDescriptor) error {
	metadata, err := json.Marshal(a.Metadata)
	if err != nil {
		return err
	}
	a.CreatedAt = time.Now()
	_, err = r.db.Exec(`INSERT INTO artifacts
		(id, tenant_id, package_id, storage_profile_id, name, artifact_type, media_type, repository, registry_url, digest, size_bytes, verification_status, lifecycle_state, metadata, created_at, updated_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,NULLIF($7,''),$8,NULLIF($9,''),$10,$11,$12,$13,$14,$15,$15)`,
		a.ID, a.TenantID, a.PackageID, a.StorageProfileID, a.Name, string(a.ArtifactType), a.MediaType,
		a.Repository, a.RegistryURL, a.Digest, a.SizeBytes, string(a.VerificationStatus),
		string(a.LifecycleState), metadata, a.CreatedAt)
	return err
}

func (r *ArtifactRepo) Get(id, tenantID string) (*appstore.ArtifactDescriptor, error) {
	return scanArtifact(r.db.QueryRow(artifactSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID))
}

func (r *ArtifactRepo) List(tenantID string, limit, offset int) ([]appstore.ArtifactDescriptor, error) {
	rows, err := r.db.Query(artifactSelect+` WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]appstore.ArtifactDescriptor, 0)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *artifact)
	}
	return result, rows.Err()
}

func (r *ArtifactRepo) ListUnassigned(tenantID string, limit, offset int) ([]appstore.ArtifactDescriptor, error) {
	rows, err := r.db.Query(artifactSelect+` a WHERE a.tenant_id=$1 AND NOT EXISTS (
		SELECT 1 FROM release_artifacts ra WHERE ra.artifact_id=a.id
	) ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifacts(rows)
}

func (r *ArtifactRepo) ListByRelease(releaseID, tenantID string) ([]appstore.ArtifactDescriptor, error) {
	rows, err := r.db.Query(`SELECT a.id, a.tenant_id, a.package_id, a.storage_profile_id, a.name, a.artifact_type,
		a.media_type, a.repository, a.registry_url, a.digest, a.size_bytes, a.verification_status,
		a.lifecycle_state, a.metadata, a.created_at, a.updated_at
		FROM release_artifacts ra
		JOIN artifacts a ON a.id=ra.artifact_id
		JOIN releases r ON r.id=ra.release_id
		JOIN products p ON p.id=r.product_id
		JOIN publishers pub ON pub.id=p.publisher_id
		WHERE ra.release_id=$1 AND pub.tenant_id=$2 AND a.tenant_id=$2
		ORDER BY ra.position, ra.created_at`, releaseID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifacts(rows)
}

func (r *ArtifactRepo) Update(a *appstore.ArtifactDescriptor) error {
	metadata, err := json.Marshal(a.Metadata)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(`UPDATE artifacts SET name=$3, artifact_type=$4, media_type=NULLIF($5,''), repository=$6,
		registry_url=NULLIF($7,''), digest=$8, size_bytes=$9, verification_status=$10, lifecycle_state=$11, metadata=$12,
		storage_profile_id=NULLIF($13,'')::uuid, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, a.ID, a.TenantID, a.Name, string(a.ArtifactType), a.MediaType, a.Repository,
		a.RegistryURL, a.Digest, a.SizeBytes, string(a.VerificationStatus), string(a.LifecycleState), metadata,
		a.StorageProfileID)
	return requireAffected(result, err)
}

// MarkPromoted updates the artifact's registry_url and verification_status after
// the artifact content has been promoted from staging to the OCI registry.
func (r *ArtifactRepo) MarkPromoted(artifactID, tenantID, registryURL string, status appstore.ArtifactVerificationStatus) error {
	result, err := r.db.Exec(`UPDATE artifacts SET registry_url=$3, verification_status=$4, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, artifactID, tenantID, registryURL, string(status))
	return requireAffected(result, err)
}

func (r *ArtifactRepo) AttachToRelease(releaseID, artifactID, tenantID string) (*appstore.ArtifactDescriptor, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	manifest, err := lockDraftRelease(tx, releaseID, tenantID)
	if err != nil {
		return nil, err
	}
	artifact, err := scanArtifact(tx.QueryRow(artifactSelect+` WHERE id=$1 AND tenant_id=$2 FOR SHARE`, artifactID, tenantID))
	if err != nil {
		return nil, err
	}
	if artifact.VerificationStatus != appstore.ArtifactVerified || artifact.LifecycleState != appstore.ArtifactActive {
		return nil, ErrArtifactNotAttachable
	}

	_, err = tx.Exec(`INSERT INTO release_artifacts (release_id, artifact_id, purpose, position, digest)
		VALUES ($1,$2,'runtime',COALESCE((SELECT MAX(position)+1 FROM release_artifacts WHERE release_id=$1),0),$3)
		ON CONFLICT (release_id, artifact_id, purpose) DO NOTHING`, releaseID, artifact.ID, artifact.Digest)
	if err != nil {
		return nil, err
	}
	if err := rebuildReleaseManifestArtifacts(tx, releaseID, tenantID, manifest); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return artifact, nil
}

func (r *ArtifactRepo) DetachFromRelease(releaseID, artifactID, tenantID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	manifest, err := lockDraftRelease(tx, releaseID, tenantID)
	if err != nil {
		return err
	}
	if _, err := scanArtifact(tx.QueryRow(artifactSelect+` WHERE id=$1 AND tenant_id=$2 FOR SHARE`, artifactID, tenantID)); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM release_artifacts WHERE release_id=$1 AND artifact_id=$2`, releaseID, artifactID)
	if err := requireAffected(result, err); err != nil {
		return err
	}
	if err := rebuildReleaseManifestArtifacts(tx, releaseID, tenantID, manifest); err != nil {
		return err
	}
	return tx.Commit()
}

// ConfirmUpload atomically records verified content and completes its session.
// Repeating confirmation returns the descriptor already linked to the session.
func (r *ArtifactRepo) ConfirmUpload(sessionID, tenantID string, artifact *appstore.ArtifactDescriptor) (*appstore.ArtifactDescriptor, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status appstore.UploadSessionStatus
	var existingID sql.NullString
	var releaseID sql.NullString
	var releaseManifest []byte
	var expiresAt time.Time
	err = tx.QueryRow(`SELECT status, artifact_id, release_id, expires_at FROM upload_sessions
		WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, sessionID, tenantID).Scan(&status, &existingID, &releaseID, &expiresAt)
	if err != nil {
		return nil, err
	}
	if status == appstore.SessionCompleted && existingID.Valid {
		existing, err := scanArtifact(tx.QueryRow(artifactSelect+` WHERE id=$1 AND tenant_id=$2`, existingID.String, tenantID))
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if status != appstore.SessionPending && status != appstore.SessionUploading {
		return nil, ErrUploadSessionState
	}
	if !expiresAt.After(time.Now()) {
		return nil, ErrUploadSessionExpired
	}
	if releaseID.Valid {
		var releaseStatus appstore.ReleaseStatus
		err := tx.QueryRow(`SELECT r.status, r.manifest FROM releases r
			JOIN products p ON p.id=r.product_id
			JOIN publishers pub ON pub.id=p.publisher_id
			WHERE r.id=$1 AND pub.tenant_id=$2 FOR UPDATE OF r`, releaseID.String, tenantID).Scan(&releaseStatus, &releaseManifest)
		if err != nil {
			return nil, err
		}
		if releaseStatus != appstore.RelDraft {
			return nil, ErrUploadReleaseState
		}
	}

	metadata, err := json.Marshal(artifact.Metadata)
	if err != nil {
		return nil, err
	}
	artifact.TenantID = tenantID
	artifact.CreatedAt = time.Now()
	stored, err := scanArtifact(tx.QueryRow(`INSERT INTO artifacts
		(id, tenant_id, package_id, storage_profile_id, name, artifact_type, media_type, repository, registry_url, digest, size_bytes, verification_status, lifecycle_state, metadata, created_at, updated_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,NULLIF($7,''),$8,NULLIF($9,''),$10,$11,$12,$13,$14,$15,$15)
		ON CONFLICT (tenant_id, digest) DO UPDATE SET digest=EXCLUDED.digest, updated_at=NOW()
		RETURNING id, tenant_id, package_id, storage_profile_id, name, artifact_type, media_type, repository, registry_url,
			digest, size_bytes, verification_status, lifecycle_state, metadata, created_at, updated_at`, artifact.ID, artifact.TenantID, artifact.PackageID, artifact.StorageProfileID, artifact.Name,
		string(artifact.ArtifactType), artifact.MediaType, artifact.Repository, artifact.RegistryURL,
		artifact.Digest, artifact.SizeBytes, string(artifact.VerificationStatus), string(artifact.LifecycleState),
		metadata, artifact.CreatedAt))
	if err != nil {
		return nil, err
	}
	artifact = stored

	if releaseID.Valid {
		manifestJSON, manifestDigest, err := appendReleaseManifestArtifact(releaseManifest, artifact.Name, artifact.Digest)
		if err != nil {
			return nil, err
		}
		result, err := tx.Exec(`UPDATE releases SET manifest=$2, manifest_digest=$3
			WHERE id=$1 AND status='draft'`, releaseID.String, manifestJSON, manifestDigest)
		if err := requireAffected(result, err); err != nil {
			return nil, err
		}
		_, err = tx.Exec(`INSERT INTO release_artifacts (release_id, artifact_id, purpose, position, digest)
			VALUES ($1,$2,'runtime',COALESCE((SELECT MAX(position)+1 FROM release_artifacts WHERE release_id=$1),0),$3)
			ON CONFLICT (release_id, artifact_id, purpose) DO NOTHING`, releaseID.String, artifact.ID, artifact.Digest)
		if err != nil {
			return nil, err
		}
	}

	result, err := tx.Exec(`UPDATE upload_sessions
		SET status='completed', artifact_id=$3, digest=$4, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, sessionID, tenantID, artifact.ID, artifact.Digest)
	if err := requireAffected(result, err); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return artifact, nil
}

const artifactSelect = `SELECT id, tenant_id, package_id, storage_profile_id, name, artifact_type, media_type,
	repository, registry_url, digest, size_bytes, verification_status, lifecycle_state, metadata,
	created_at, updated_at FROM artifacts`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanArtifact(row rowScanner) (*appstore.ArtifactDescriptor, error) {
	var artifact appstore.ArtifactDescriptor
	var packageID, storageProfileID, mediaType, registryURL sql.NullString
	var metadata []byte
	var updatedAt sql.NullTime
	err := row.Scan(&artifact.ID, &artifact.TenantID, &packageID, &storageProfileID, &artifact.Name, &artifact.ArtifactType,
		&mediaType, &artifact.Repository, &registryURL, &artifact.Digest, &artifact.SizeBytes,
		&artifact.VerificationStatus, &artifact.LifecycleState, &metadata, &artifact.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if packageID.Valid {
		artifact.PackageID = packageID.String
	}
	if storageProfileID.Valid {
		artifact.StorageProfileID = storageProfileID.String
	}
	if mediaType.Valid {
		artifact.MediaType = mediaType.String
	}
	if registryURL.Valid {
		artifact.RegistryURL = registryURL.String
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &artifact.Metadata); err != nil {
			return nil, err
		}
	}
	if updatedAt.Valid {
		t := updatedAt.Time
		artifact.UpdatedAt = &t
	}
	return &artifact, nil
}

func scanArtifacts(rows *sql.Rows) ([]appstore.ArtifactDescriptor, error) {
	result := make([]appstore.ArtifactDescriptor, 0)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *artifact)
	}
	return result, rows.Err()
}

func appendReleaseManifestArtifact(data []byte, name, digest string) ([]byte, string, error) {
	manifest, err := decodeReleaseManifest(data)
	if err != nil {
		return nil, "", err
	}

	artifacts := make([]any, 0)
	if value, ok := manifest["artifacts"]; ok {
		var valid bool
		artifacts, valid = value.([]any)
		if !valid {
			return nil, "", errors.New("release manifest artifacts must be an array")
		}
	}
	for _, value := range artifacts {
		if entry, ok := value.(map[string]any); ok && entry["digest"] == digest {
			return appstore.EncodeReleaseManifest(manifest)
		}
	}
	manifest["artifacts"] = append(artifacts, map[string]any{
		"name": name, "digest": digest, "purpose": "runtime",
	})
	return appstore.EncodeReleaseManifest(manifest)
}

func lockDraftRelease(tx *sql.Tx, releaseID, tenantID string) ([]byte, error) {
	var status appstore.ReleaseStatus
	var manifest []byte
	err := tx.QueryRow(`SELECT r.status, r.manifest FROM releases r
		JOIN products p ON p.id=r.product_id
		JOIN publishers pub ON pub.id=p.publisher_id
		WHERE r.id=$1 AND pub.tenant_id=$2 FOR UPDATE OF r`, releaseID, tenantID).Scan(&status, &manifest)
	if err != nil {
		return nil, err
	}
	if status != appstore.RelDraft {
		return nil, ErrUploadReleaseState
	}
	return manifest, nil
}

func rebuildReleaseManifestArtifacts(tx *sql.Tx, releaseID, tenantID string, data []byte) error {
	manifest, err := decodeReleaseManifest(data)
	if err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT a.name, ra.digest, ra.purpose FROM release_artifacts ra
		JOIN artifacts a ON a.id=ra.artifact_id
		WHERE ra.release_id=$1 AND a.tenant_id=$2
		ORDER BY ra.position, ra.created_at`, releaseID, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	artifacts := make([]any, 0)
	for rows.Next() {
		var name, digest, purpose string
		if err := rows.Scan(&name, &digest, &purpose); err != nil {
			return err
		}
		artifacts = append(artifacts, map[string]any{"name": name, "digest": digest, "purpose": purpose})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	manifest["artifacts"] = artifacts
	manifestJSON, manifestDigest, err := appstore.EncodeReleaseManifest(manifest)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE releases SET manifest=$2, manifest_digest=$3
		WHERE id=$1 AND status='draft'`, releaseID, manifestJSON, manifestDigest)
	return requireAffected(result, err)
}

func decodeReleaseManifest(data []byte) (map[string]any, error) {
	manifest := make(map[string]any)
	if len(data) == 0 || string(data) == "null" {
		return manifest, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode release manifest: %w", err)
	}
	return manifest, nil
}
