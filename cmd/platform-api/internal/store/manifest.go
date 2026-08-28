package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
	"github.com/lib/pq"
)

type ProviderManifest struct {
	ProviderID           string                     `json:"provider_id"`
	Name                 string                     `json:"name"`
	Version              string                     `json:"version"`
	ProtocolVersion      string                     `json:"protocol_version"`
	Capabilities         []string                   `json:"capabilities"`
	Actions              []string                   `json:"actions"`
	Permissions          map[string]any             `json:"permissions"`
	ResourceRequirements map[string]any             `json:"resource_requirements"`
	Dependencies         []ProviderDependency       `json:"dependencies"`
	Compatibility        map[string]any             `json:"compatibility"`
	ConformanceLevel     string                     `json:"conformance_level"`
	ConformanceEvidence  []ConformanceEvidence      `json:"conformance_evidence"`
	ConformanceExpiresAt *time.Time                 `json:"conformance_expires_at,omitempty"`
	StorageDriverPackage *core.StorageDriverPackage `json:"storage_driver_package,omitempty"`
}

type ProviderDependency struct {
	ProviderID string `json:"provider_id"`
	Version    string `json:"version"`
	Required   bool   `json:"required"`
}

type ConformanceEvidence struct {
	TestName    string `json:"test_name"`
	Category    string `json:"category"`
	Passed      bool   `json:"passed"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
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

func (s *PGStore) GetManifest(ctx context.Context, providerID string) (*ProviderManifest, error) {
	manifest := &ProviderManifest{}
	var capabilities, actions []string
	var permissionsJSON, resourceJSON, depsJSON, compatJSON, evidenceJSON, storageDriverPackageJSON string
	var expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT provider_id, name, version, protocol_version,
			capabilities, actions, permissions, resource_requirements,
			dependencies, compatibility, conformance_level,
			conformance_evidence, conformance_expires_at, storage_driver_package
		FROM provider_manifests
		WHERE provider_id = $1
		  AND (SELECT count(*) FROM provider_manifests WHERE provider_id = $1) = 1`, providerID,
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

func (s *PGStore) SaveManifest(ctx context.Context, manifest *ProviderManifest) error {
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

	_, err := s.db.ExecContext(ctx, `
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
		manifest.ConformanceLevel, string(evidenceJSON),
		manifest.ConformanceExpiresAt, string(storageDriverPackageJSON),
	)
	return err
}

func (s *PGStore) DeleteManifest(ctx context.Context, providerID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM provider_manifests WHERE provider_id = $1`, providerID)
	return err
}

func (s *PGStore) CheckCompatibility(ctx context.Context, coreVersion, providerID, providerVersion, targetType string) (*CompatibilityEntry, error) {
	entry := &CompatibilityEntry{}
	err := s.db.QueryRowContext(ctx, `
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

func (s *PGStore) SaveCompatibility(ctx context.Context, entry *CompatibilityEntry) error {
	_, err := s.db.ExecContext(ctx, `
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
