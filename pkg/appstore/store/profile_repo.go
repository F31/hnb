package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

var ErrInvalidStorageProfile = errors.New("invalid storage profile")

type StorageProfileRepo struct{ db *sql.DB }

func NewStorageProfileRepo(db *sql.DB) *StorageProfileRepo { return &StorageProfileRepo{db: db} }

func (r *StorageProfileRepo) Create(profile *appstore.ArtifactStorageProfile) error {
	if err := ValidateStorageProfile(profile); err != nil {
		return err
	}
	metadata, err := json.Marshal(profile.Metadata)
	if err != nil {
		return err
	}
	profile.CreatedAt = time.Now()
	profile.UpdatedAt = profile.CreatedAt
	result, err := r.db.Exec(`INSERT INTO artifact_storage_profiles
		(id, tenant_id, name, backend, service_tier, authority_role, secret_reference, endpoint, region, rpo_seconds, rto_seconds, lifecycle_state, metadata, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13,$14,$15,$16)`,
		profile.ID, profile.TenantID, profile.Name, string(profile.Backend), string(profile.ServiceTier), string(profile.AuthorityRole),
		profile.SecretReference, profile.Endpoint, profile.Region, profile.RPOSeconds, profile.RTOSeconds, string(profile.LifecycleState),
		metadata, profile.CreatedBy, profile.CreatedAt, profile.UpdatedAt)
	return requireAffected(result, err)
}

func (r *StorageProfileRepo) Get(id, tenantID string) (*appstore.ArtifactStorageProfile, error) {
	return scanStorageProfile(r.db.QueryRow(storageProfileSelect+` WHERE id=$1 AND tenant_id=$2`, id, tenantID))
}

func (r *StorageProfileRepo) List(tenantID string, limit, offset int) ([]appstore.ArtifactStorageProfile, error) {
	rows, err := r.db.Query(storageProfileSelect+` WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]appstore.ArtifactStorageProfile, 0)
	for rows.Next() {
		profile, err := scanStorageProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *profile)
	}
	return profiles, rows.Err()
}

func (r *StorageProfileRepo) Update(profile *appstore.ArtifactStorageProfile) error {
	if err := ValidateStorageProfile(profile); err != nil {
		return err
	}
	metadata, err := json.Marshal(profile.Metadata)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(`UPDATE artifact_storage_profiles SET
		name=$3, backend=$4, service_tier=$5, authority_role=$6, secret_reference=NULLIF($7,''), endpoint=NULLIF($8,''),
		region=NULLIF($9,''), rpo_seconds=$10, rto_seconds=$11, lifecycle_state=$12, metadata=$13, updated_at=NOW()
		WHERE id=$1 AND tenant_id=$2`, profile.ID, profile.TenantID, profile.Name, string(profile.Backend), string(profile.ServiceTier),
		string(profile.AuthorityRole), profile.SecretReference, profile.Endpoint, profile.Region, profile.RPOSeconds, profile.RTOSeconds,
		string(profile.LifecycleState), metadata)
	return requireAffected(result, err)
}

func (r *StorageProfileRepo) Delete(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM artifact_storage_profiles WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return requireAffected(result, err)
}

func (r *StorageProfileRepo) RequestMigration(migration *appstore.ArtifactProfileMigration) error {
	checkpoint, err := json.Marshal(migration.Checkpoint)
	if err != nil {
		return err
	}
	migration.CreatedAt = time.Now()
	migration.UpdatedAt = migration.CreatedAt
	result, err := r.db.Exec(`INSERT INTO artifact_profile_migrations
		(id, tenant_id, artifact_id, source_profile_id, target_profile_id, artifact_digest, status, operation_id, checkpoint, idempotency_key, requested_by, created_at, updated_at)
		SELECT $1,$2,a.id,src.id,dst.id,a.digest,$7,NULLIF($8,'')::uuid,$9,$10,$11,$12,$13
		FROM artifacts a
		JOIN artifact_storage_profiles src ON src.id=$4 AND src.tenant_id=$2
		JOIN artifact_storage_profiles dst ON dst.id=$5 AND dst.tenant_id=$2
		WHERE a.id=$3 AND a.tenant_id=$2 AND a.digest=$6 AND a.lifecycle_state='active'`,
		migration.ID, migration.TenantID, migration.ArtifactID, migration.SourceProfileID, migration.TargetProfileID,
		migration.ArtifactDigest, migration.Status, migration.OperationID, checkpoint, migration.IdempotencyKey,
		migration.RequestedBy, migration.CreatedAt, migration.UpdatedAt)
	return requireAffected(result, err)
}

func ValidateStorageProfile(profile *appstore.ArtifactStorageProfile) error {
	if profile.ID == "" || profile.TenantID == "" || profile.Name == "" || profile.CreatedBy == "" {
		return fmt.Errorf("%w: id, tenant_id, name and created_by are required", ErrInvalidStorageProfile)
	}
	if profile.LifecycleState == "" {
		profile.LifecycleState = appstore.StorageProfileActive
	}
	if profile.AuthorityRole == "" {
		profile.AuthorityRole = appstore.StorageAuthoritative
	}
	if profile.ServiceTier == "" {
		profile.ServiceTier = appstore.StorageTierMinimal
	}
	if !validBackend(profile.Backend) || !validTier(profile.ServiceTier) || !validAuthority(profile.AuthorityRole) {
		return fmt.Errorf("%w: unsupported backend, tier or authority role", ErrInvalidStorageProfile)
	}
	if profile.RPOSeconds < 0 || profile.RTOSeconds < 0 {
		return fmt.Errorf("%w: rpo/rto must be non-negative", ErrInvalidStorageProfile)
	}
	if containsInlineSecret(profile.SecretReference) || containsInlineSecretMap(profile.Metadata) {
		return fmt.Errorf("%w: inline secret values are not accepted", ErrInvalidStorageProfile)
	}
	if containsWorkloadStorageSemantics(profile.Metadata) {
		return fmt.Errorf("%w: workload storage offering and StorageClass binding fields are not accepted", ErrInvalidStorageProfile)
	}
	if profile.ServiceTier != appstore.StorageTierMinimal {
		if profile.AuthorityRole != appstore.StorageAuthoritative || profile.Backend == appstore.StorageLocal {
			return fmt.Errorf("%w: lite_ha and above require a shared authoritative pvc/s3/oci backend", ErrInvalidStorageProfile)
		}
	}
	return nil
}

func validBackend(backend appstore.StorageBackend) bool {
	switch backend {
	case appstore.StorageLocal, appstore.StoragePVC, appstore.StorageS3, appstore.StorageOCI:
		return true
	default:
		return false
	}
}

func validTier(tier appstore.StorageServiceTier) bool {
	switch tier {
	case appstore.StorageTierMinimal, appstore.StorageTierLiteHA, appstore.StorageTierStandard, appstore.StorageTierEnterprise:
		return true
	default:
		return false
	}
}

func validAuthority(role appstore.StorageAuthorityRole) bool {
	switch role {
	case appstore.StorageAuthoritative, appstore.StorageMirror, appstore.StorageCache:
		return true
	default:
		return false
	}
}

func containsInlineSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.Contains(trimmed, "=") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func containsInlineSecretMap(values map[string]any) bool {
	for key, value := range values {
		if strings.Contains(strings.ToLower(key), "secret") || strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "token") {
			if s, ok := value.(string); ok && s != "" {
				return true
			}
		}
	}
	return false
}

func containsWorkloadStorageSemantics(values map[string]any) bool {
	for key := range values {
		switch strings.ToLower(key) {
		case "consumptionmodel", "bindingtarget", "storageclassbindingid", "workloadstorageofferingid":
			return true
		}
	}
	return false
}

const storageProfileSelect = `SELECT id, tenant_id, name, backend, service_tier, authority_role, secret_reference,
	endpoint, region, rpo_seconds, rto_seconds, lifecycle_state, metadata, created_by, created_at, updated_at FROM artifact_storage_profiles`

type scanner interface{ Scan(dest ...any) error }

func scanStorageProfile(row scanner) (*appstore.ArtifactStorageProfile, error) {
	var profile appstore.ArtifactStorageProfile
	var secretReference, endpoint, region sql.NullString
	var metadata []byte
	if err := row.Scan(&profile.ID, &profile.TenantID, &profile.Name, &profile.Backend, &profile.ServiceTier, &profile.AuthorityRole,
		&secretReference, &endpoint, &region, &profile.RPOSeconds, &profile.RTOSeconds, &profile.LifecycleState,
		&metadata, &profile.CreatedBy, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
		return nil, err
	}
	if secretReference.Valid {
		profile.SecretReference = secretReference.String
	}
	if endpoint.Valid {
		profile.Endpoint = endpoint.String
	}
	if region.Valid {
		profile.Region = region.String
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &profile.Metadata); err != nil {
			return nil, err
		}
	}
	return &profile, nil
}
