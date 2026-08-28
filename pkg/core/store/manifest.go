package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/lib/pq"
)

type ManifestStore struct {
	db *sql.DB
}

func NewManifestStore(db *sql.DB) *ManifestStore {
	return &ManifestStore{db: db}
}

func (s *ManifestStore) Save(manifest *core.ProviderManifest) error {
	if manifest.StorageDriverPackage != nil {
		if err := manifest.StorageDriverPackage.Validate(manifest.Version, time.Now()); err != nil {
			return fmt.Errorf("validate storage driver package: %w", err)
		}
	}
	permissionsJSON, _ := json.Marshal(manifest.Permissions)
	resourceJSON, _ := json.Marshal(manifest.ResourceRequirements)
	depsJSON, _ := json.Marshal(manifest.Dependencies)
	compatJSON, _ := json.Marshal(manifest.Compatibility)
	evidenceJSON, _ := json.Marshal(manifest.ConformanceEvidence)
	storageDriverPackageJSON, _ := json.Marshal(manifest.StorageDriverPackage)

	_, err := s.db.Exec(`
		INSERT INTO provider_manifests (
			provider_id, name, version, protocol_version,
			capabilities, actions, permissions, resource_requirements,
			dependencies, compatibility, conformance_level,
			conformance_evidence, conformance_expires_at, storage_driver_package
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (provider_id) DO UPDATE SET
			name = EXCLUDED.name,
			version = EXCLUDED.version,
			protocol_version = EXCLUDED.protocol_version,
			capabilities = EXCLUDED.capabilities,
			actions = EXCLUDED.actions,
			permissions = EXCLUDED.permissions,
			resource_requirements = EXCLUDED.resource_requirements,
			dependencies = EXCLUDED.dependencies,
			compatibility = EXCLUDED.compatibility,
			conformance_level = EXCLUDED.conformance_level,
			conformance_evidence = EXCLUDED.conformance_evidence,
			conformance_expires_at = EXCLUDED.conformance_expires_at,
			storage_driver_package = EXCLUDED.storage_driver_package,
			updated_at = NOW()`,
		manifest.ProviderID, manifest.Name, manifest.Version, manifest.ProtocolVersion,
		pq.Array(manifest.Capabilities), pq.Array(manifest.Actions),
		string(permissionsJSON), string(resourceJSON),
		string(depsJSON), string(compatJSON),
		string(manifest.ConformanceLevel), string(evidenceJSON),
		manifest.ConformanceExpiresAt, string(storageDriverPackageJSON),
	)
	return err
}

func (s *ManifestStore) Get(providerID string) (*core.ProviderManifest, error) {
	manifest := &core.ProviderManifest{}
	var capabilities, actions []string
	var permissionsJSON, resourceJSON, depsJSON, compatJSON, evidenceJSON, storageDriverPackageJSON string
	var expiresAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT provider_id, name, version, protocol_version,
			capabilities, actions, permissions, resource_requirements,
			dependencies, compatibility, conformance_level,
			conformance_evidence, conformance_expires_at, storage_driver_package
		FROM provider_manifests WHERE provider_id = $1`, providerID,
	).Scan(
		&manifest.ProviderID, &manifest.Name, &manifest.Version, &manifest.ProtocolVersion,
		pq.Array(&capabilities), pq.Array(&actions),
		&permissionsJSON, &resourceJSON, &depsJSON, &compatJSON,
		&manifest.ConformanceLevel, &evidenceJSON, &expiresAt, &storageDriverPackageJSON,
	)
	if err != nil {
		return nil, err
	}

	manifest.Capabilities = capabilities
	manifest.Actions = actions
	json.Unmarshal([]byte(permissionsJSON), &manifest.Permissions)
	json.Unmarshal([]byte(resourceJSON), &manifest.ResourceRequirements)
	json.Unmarshal([]byte(depsJSON), &manifest.Dependencies)
	json.Unmarshal([]byte(compatJSON), &manifest.Compatibility)
	json.Unmarshal([]byte(evidenceJSON), &manifest.ConformanceEvidence)
	if storageDriverPackageJSON != "null" && storageDriverPackageJSON != "{}" {
		json.Unmarshal([]byte(storageDriverPackageJSON), &manifest.StorageDriverPackage)
	}
	if expiresAt.Valid {
		manifest.ConformanceExpiresAt = &expiresAt.Time
	}
	return manifest, nil
}

func (s *ManifestStore) Delete(providerID string) error {
	_, err := s.db.Exec(`DELETE FROM provider_manifests WHERE provider_id = $1`, providerID)
	return err
}

type CompatibilityEntry struct {
	ID                string
	CoreVersion       string
	ProviderID        string
	ProviderVersion   string
	RuntimeTargetType string
	Compatible        bool
	ConstraintReason  string
	CreatedAt         time.Time
}

func (s *ManifestStore) SaveCompatibility(entry *CompatibilityEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO provider_compatibility_matrix (core_version, provider_id, provider_version, runtime_target_type, compatible, constraint_reason)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		ON CONFLICT (core_version, provider_id, provider_version, runtime_target_type) DO UPDATE SET
			compatible = EXCLUDED.compatible,
			constraint_reason = EXCLUDED.constraint_reason`,
		entry.CoreVersion, entry.ProviderID, entry.ProviderVersion,
		entry.RuntimeTargetType, entry.Compatible, entry.ConstraintReason,
	)
	return err
}

func (s *ManifestStore) CheckCompatibility(coreVersion, providerID, providerVersion, targetType string) (*CompatibilityEntry, error) {
	entry := &CompatibilityEntry{}
	err := s.db.QueryRow(`
		SELECT id, core_version, provider_id, provider_version, runtime_target_type, compatible, COALESCE(constraint_reason, ''), created_at
		FROM provider_compatibility_matrix
		WHERE core_version = $1 AND provider_id = $2 AND provider_version = $3 AND runtime_target_type = $4`,
		coreVersion, providerID, providerVersion, targetType,
	).Scan(&entry.ID, &entry.CoreVersion, &entry.ProviderID, &entry.ProviderVersion,
		&entry.RuntimeTargetType, &entry.Compatible, &entry.ConstraintReason, &entry.CreatedAt)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *ManifestStore) ExpireConformance() ([]string, error) {
	rows, err := s.db.Query(`
		UPDATE provider_manifests SET conformance_level = 'none', updated_at = NOW()
		WHERE conformance_expires_at IS NOT NULL AND conformance_expires_at < NOW()
		AND conformance_level != 'none'
		RETURNING provider_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expired []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		expired = append(expired, id)
	}
	return expired, nil
}
