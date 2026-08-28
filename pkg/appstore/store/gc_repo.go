package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

var ErrArtifactGCBlocked = errors.New("artifact has protected references")

type GCRepo struct{ db *sql.DB }

func NewGCRepo(db *sql.DB) *GCRepo { return &GCRepo{db: db} }

func (r *GCRepo) RegisterReference(ref *appstore.ArtifactReference) error {
	if ref.Purpose == "" {
		ref.Purpose = "runtime"
	}
	ref.CreatedAt = time.Now()
	result, err := r.db.Exec(`INSERT INTO artifact_references
		(id, tenant_id, artifact_id, owner_type, owner_id, purpose, expires_at, created_by, created_at)
		SELECT $1,$2,a.id,$4,$5,$6,$7,$8,$9 FROM artifacts a
		WHERE a.id=$3 AND a.tenant_id=$2 AND a.lifecycle_state='active'
		ON CONFLICT (tenant_id, artifact_id, owner_type, owner_id, purpose) DO UPDATE
		SET expires_at=EXCLUDED.expires_at`,
		ref.ID, ref.TenantID, ref.ArtifactID, ref.OwnerType, ref.OwnerID, ref.Purpose, ref.ExpiresAt, ref.CreatedBy, ref.CreatedAt)
	return requireAffected(result, err)
}

func (r *GCRepo) ListReferences(artifactID, tenantID string) ([]appstore.ArtifactReference, error) {
	rows, err := r.db.Query(referenceSelect+` WHERE artifact_id=$1 AND tenant_id=$2 AND (expires_at IS NULL OR expires_at > NOW()) ORDER BY created_at DESC`, artifactID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []appstore.ArtifactReference
	for rows.Next() {
		ref, err := scanReference(rows)
		if err != nil {
			return nil, err
		}
		refs = append(refs, *ref)
	}
	return refs, rows.Err()
}

func (r *GCRepo) Preview(artifactID, tenantID string, retention time.Duration) (*appstore.GCPreview, error) {
	artifact, err := NewArtifactRepo(r.db).Get(artifactID, tenantID)
	if err != nil {
		return nil, err
	}
	refs, err := r.ListReferences(artifactID, tenantID)
	if err != nil {
		return nil, err
	}
	return &appstore.GCPreview{ArtifactID: artifact.ID, Digest: artifact.Digest, Blocked: len(refs) > 0, Blockers: refs,
		EstimatedBytes: artifact.SizeBytes, EarliestDeleteAt: time.Now().Add(retention)}, nil
}

func (r *GCRepo) Execute(artifactID, tenantID, tombstoneID, operationID, actorID, reason string, retention time.Duration) (*appstore.ArtifactTombstone, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var digest string
	if err := tx.QueryRow(`SELECT digest FROM artifacts WHERE id=$1 AND tenant_id=$2 AND lifecycle_state='active' FOR UPDATE`, artifactID, tenantID).Scan(&digest); err != nil {
		return nil, err
	}
	var blockers int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM artifact_references WHERE artifact_id=$1 AND tenant_id=$2 AND (expires_at IS NULL OR expires_at > NOW())`, artifactID, tenantID).Scan(&blockers); err != nil {
		return nil, err
	}
	if blockers > 0 {
		return nil, ErrArtifactGCBlocked
	}
	leaseUntil := time.Now().Add(retention)
	result, err := tx.Exec(`INSERT INTO artifact_locks (artifact_id, tenant_id, lock_owner, lease_until, created_at, updated_at)
		VALUES ($1,$2,$3,$4,NOW(),NOW())
		ON CONFLICT (artifact_id) DO UPDATE SET lock_owner=EXCLUDED.lock_owner, lease_until=EXCLUDED.lease_until, updated_at=NOW()
		WHERE artifact_locks.tenant_id=$2 AND artifact_locks.lease_until < NOW()`, artifactID, tenantID, "gc:"+tombstoneID, leaseUntil)
	if err := requireAffected(result, err); err != nil {
		return nil, err
	}
	tombstone := &appstore.ArtifactTombstone{ID: tombstoneID, TenantID: tenantID, ArtifactID: artifactID, ArtifactDigest: digest, State: "retained", DeleteAfter: leaseUntil, OperationID: operationID, RequestedBy: actorID, Reason: reason, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	result, err = tx.Exec(`INSERT INTO artifact_tombstones
		(id, tenant_id, artifact_id, artifact_digest, state, delete_after, operation_id, requested_by, reason, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,NULLIF($9,''),$10,$11)`, tombstone.ID, tombstone.TenantID, tombstone.ArtifactID, tombstone.ArtifactDigest, tombstone.State, tombstone.DeleteAfter, tombstone.OperationID, tombstone.RequestedBy, tombstone.Reason, tombstone.CreatedAt, tombstone.UpdatedAt)
	if err := requireAffected(result, err); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE artifacts SET lifecycle_state='tombstoned' WHERE id=$1 AND tenant_id=$2`, artifactID, tenantID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tombstone, nil
}

func (r *GCRepo) PrepareSweep(tombstoneID, tenantID string) (*appstore.GCSweepCommand, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var cmd appstore.GCSweepCommand
	var blockers int
	err = tx.QueryRow(`SELECT t.id, t.tenant_id, t.artifact_id, t.artifact_digest, COALESCE(t.operation_id::text,''),
		(SELECT COUNT(*) FROM artifact_references r WHERE r.artifact_id=t.artifact_id AND r.tenant_id=t.tenant_id AND (r.expires_at IS NULL OR r.expires_at > NOW()))
		FROM artifact_tombstones t WHERE t.id=$1 AND t.tenant_id=$2 AND t.state='retained' AND t.delete_after <= NOW() FOR UPDATE`, tombstoneID, tenantID).
		Scan(&cmd.TombstoneID, &cmd.TenantID, &cmd.ArtifactID, &cmd.ArtifactDigest, &cmd.OperationID, &blockers)
	if err != nil {
		return nil, err
	}
	if blockers > 0 {
		_, err := tx.Exec(`UPDATE artifact_tombstones SET state='cancelled', updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, tombstoneID, tenantID)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(`UPDATE artifacts SET lifecycle_state='active' WHERE id=$1 AND tenant_id=$2`, cmd.ArtifactID, tenantID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrArtifactGCBlocked
	}
	if _, err := tx.Exec(`UPDATE artifact_tombstones SET state='deleting', updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, tombstoneID, tenantID); err != nil {
		return nil, err
	}
	return &cmd, tx.Commit()
}

func (r *GCRepo) CompleteSweep(tombstoneID, tenantID string) error {
	result, err := r.db.Exec(`UPDATE artifacts a SET lifecycle_state='deleted'
		FROM artifact_tombstones t WHERE t.artifact_id=a.id AND t.id=$1 AND t.tenant_id=$2 AND t.state='deleting'`, tombstoneID, tenantID)
	if err := requireAffected(result, err); err != nil {
		return err
	}
	_, err = r.db.Exec(`UPDATE artifact_tombstones SET state='deleted', updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, tombstoneID, tenantID)
	return err
}

const referenceSelect = `SELECT id, tenant_id, artifact_id, owner_type, owner_id, purpose, expires_at, created_by, created_at FROM artifact_references`

func scanReference(row scanner) (*appstore.ArtifactReference, error) {
	var ref appstore.ArtifactReference
	var expiresAt sql.NullTime
	if err := row.Scan(&ref.ID, &ref.TenantID, &ref.ArtifactID, &ref.OwnerType, &ref.OwnerID, &ref.Purpose, &expiresAt, &ref.CreatedBy, &ref.CreatedAt); err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		ref.ExpiresAt = &expiresAt.Time
	}
	return &ref, nil
}

func GCOperationCommand(tombstone *appstore.ArtifactTombstone) map[string]any {
	return map[string]any{"tombstone_id": tombstone.ID, "artifact_id": tombstone.ArtifactID, "digest": tombstone.ArtifactDigest, "operation_id": tombstone.OperationID}
}

func FormatGCBlocked(count int) error {
	return fmt.Errorf("%w: %d blockers", ErrArtifactGCBlocked, count)
}
