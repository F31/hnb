package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

var ErrInvalidDistributionState = errors.New("invalid distribution state transition")

type DistributionRepo struct{ db *sql.DB }

func NewDistributionRepo(db *sql.DB) *DistributionRepo { return &DistributionRepo{db: db} }

func (r *DistributionRepo) Create(target *appstore.ArtifactDistributionTarget) error {
	if target.State == "" {
		target.State = appstore.DistributionPending
	}
	if target.Health == "" {
		target.Health = appstore.DistributionHealthUnknown
	}
	if target.CreatedAt.IsZero() {
		target.CreatedAt = time.Now()
		target.UpdatedAt = target.CreatedAt
	}
	result, err := r.db.Exec(`INSERT INTO artifact_distribution_targets
		(id, tenant_id, artifact_id, authority_profile_id, target_profile_id, target_role, desired_digest, observed_digest, state, health,
		 low_watermark_bytes, high_watermark_bytes, current_bytes, local_lock, rebuild_operation_id, last_error, idempotency_key, created_at, updated_at)
		SELECT $1,$2,a.id,authority.id,target.id,$6,a.digest,NULLIF($8,''),$9,$10,$11,$12,$13,$14,NULLIF($15,'')::uuid,NULLIF($16,''),$17,$18,$19
		FROM artifacts a
		JOIN artifact_storage_profiles authority ON authority.id=$4 AND authority.tenant_id=$2 AND authority.authority_role='authoritative'
		JOIN artifact_storage_profiles target ON target.id=$5 AND target.tenant_id=$2
		WHERE a.id=$3 AND a.tenant_id=$2 AND a.digest=$7 AND a.lifecycle_state='active'`,
		target.ID, target.TenantID, target.ArtifactID, target.AuthorityProfileID, target.TargetProfileID, string(target.TargetRole),
		target.DesiredDigest, target.ObservedDigest, string(target.State), string(target.Health), target.LowWatermarkBytes,
		target.HighWatermarkBytes, target.CurrentBytes, target.LocalLock, target.RebuildOperationID, target.LastError,
		target.IdempotencyKey, target.CreatedAt, target.UpdatedAt)
	return requireAffected(result, err)
}

func (r *DistributionRepo) Get(id, tenantID string) (*appstore.ArtifactDistributionTarget, error) {
	return scanDistributionTarget(r.db.QueryRow(distributionSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID))
}

func (r *DistributionRepo) List(tenantID string, limit, offset int) ([]appstore.ArtifactDistributionTarget, error) {
	rows, err := r.db.Query(distributionSelect+` WHERE tenant_id=$1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appstore.ArtifactDistributionTarget
	for rows.Next() {
		target, err := scanDistributionTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, rows.Err()
}

func (r *DistributionRepo) Transition(id, tenantID string, next appstore.DistributionState, observedDigest, health, lastError string) error {
	current, err := r.Get(id, tenantID)
	if err != nil {
		return err
	}
	if !CanTransitionDistribution(current.State, next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidDistributionState, current.State, next)
	}
	if health == "" {
		health = string(current.Health)
	}
	result, err := r.db.Exec(`UPDATE artifact_distribution_targets
		SET state=$3, observed_digest=NULLIF($4,''), health=$5, last_error=NULLIF($6,''), updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, id, tenantID, string(next), observedDigest, health, lastError)
	return requireAffected(result, err)
}

func (r *DistributionRepo) RequestRebuild(id, tenantID, operationID, idempotencyKey string) (*appstore.DistributionRebuildCommand, error) {
	target, err := r.Get(id, tenantID)
	if err != nil {
		return nil, err
	}
	if idempotencyKey == "" {
		idempotencyKey = target.IdempotencyKey + ":rebuild"
	}
	result, err := r.db.Exec(`UPDATE artifact_distribution_targets
		SET state='syncing', health='unknown', rebuild_operation_id=NULLIF($3,'')::uuid, idempotency_key=$4, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2 AND state IN ('pending','ready','stale','failed','syncing')`, id, tenantID, operationID, idempotencyKey)
	if err := requireAffected(result, err); err != nil {
		return nil, err
	}
	return &appstore.DistributionRebuildCommand{TargetID: target.ID, TenantID: target.TenantID, ArtifactID: target.ArtifactID,
		AuthorityProfileID: target.AuthorityProfileID, TargetProfileID: target.TargetProfileID, DesiredDigest: target.DesiredDigest,
		OperationID: operationID, IdempotencyKey: idempotencyKey}, nil
}

func (r *DistributionRepo) EvictionCandidates(tenantID string, limit int) ([]appstore.ArtifactDistributionTarget, error) {
	rows, err := r.db.Query(distributionSelect+` WHERE tenant_id=$1 AND target_role='edge_cache'
		AND local_lock=false AND high_watermark_bytes > 0 AND current_bytes > high_watermark_bytes
		AND target_profile_id <> authority_profile_id ORDER BY current_bytes - high_watermark_bytes DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appstore.ArtifactDistributionTarget
	for rows.Next() {
		target, err := scanDistributionTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, rows.Err()
}

func CanTransitionDistribution(from, to appstore.DistributionState) bool {
	allowed := map[appstore.DistributionState][]appstore.DistributionState{
		appstore.DistributionPending: {appstore.DistributionSyncing, appstore.DistributionFailed},
		appstore.DistributionSyncing: {appstore.DistributionReady, appstore.DistributionFailed, appstore.DistributionStale},
		appstore.DistributionReady:   {appstore.DistributionStale, appstore.DistributionSyncing, appstore.DistributionFailed},
		appstore.DistributionStale:   {appstore.DistributionSyncing, appstore.DistributionFailed},
		appstore.DistributionFailed:  {appstore.DistributionSyncing},
	}
	for _, state := range allowed[from] {
		if state == to {
			return true
		}
	}
	return false
}

const distributionSelect = `SELECT id, tenant_id, artifact_id, authority_profile_id, target_profile_id, target_role,
	desired_digest, observed_digest, state, health, low_watermark_bytes, high_watermark_bytes, current_bytes,
	local_lock, rebuild_operation_id, last_error, idempotency_key, created_at, updated_at FROM artifact_distribution_targets`

func scanDistributionTarget(row scanner) (*appstore.ArtifactDistributionTarget, error) {
	var target appstore.ArtifactDistributionTarget
	var observedDigest, rebuildOperationID, lastError sql.NullString
	err := row.Scan(&target.ID, &target.TenantID, &target.ArtifactID, &target.AuthorityProfileID, &target.TargetProfileID,
		&target.TargetRole, &target.DesiredDigest, &observedDigest, &target.State, &target.Health,
		&target.LowWatermarkBytes, &target.HighWatermarkBytes, &target.CurrentBytes, &target.LocalLock,
		&rebuildOperationID, &lastError, &target.IdempotencyKey, &target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if observedDigest.Valid {
		target.ObservedDigest = observedDigest.String
	}
	if rebuildOperationID.Valid {
		target.RebuildOperationID = rebuildOperationID.String
	}
	if lastError.Valid {
		target.LastError = lastError.String
	}
	return &target, nil
}
