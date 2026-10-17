package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
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
	return getManifest(ctx, s.db, providerID)
}

type manifestQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getManifest(ctx context.Context, query manifestQuerier, providerID string) (*ProviderManifest, error) {
	manifest := &ProviderManifest{}
	var capabilities, actions []string
	var permissionsJSON, resourceJSON, depsJSON, compatJSON, evidenceJSON, storageDriverPackageJSON string
	var expiresAt sql.NullTime

	err := query.QueryRowContext(ctx, `
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
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

// ManifestIdentityDigest ignores qualification metadata, so a report cannot
// qualify a different base manifest and renewals remain bound to the same input.
func ManifestIdentityDigest(manifest *ProviderManifest) string {
	copy := *manifest
	copy.ConformanceLevel = "none"
	copy.ConformanceEvidence = nil
	copy.ConformanceExpiresAt = nil
	copy.ResourceRequirements = nil
	for key, value := range manifest.ResourceRequirements {
		if key == "conformanceReport" {
			continue
		}
		if copy.ResourceRequirements == nil {
			copy.ResourceRequirements = make(map[string]any)
		}
		copy.ResourceRequirements[key] = value
	}
	data, _ := json.Marshal(copy)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
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
	// The shipped columns are NOT NULL with a '{}' default, so a nil Go slice
	// must be sent as an empty array rather than SQL NULL.
	capabilities, actions := pq.Array(manifest.Capabilities), pq.Array(manifest.Actions)
	if manifest.Capabilities == nil {
		capabilities = pq.Array([]string{})
	}
	if manifest.Actions == nil {
		actions = pq.Array([]string{})
	}

	// provider_id carries no unique constraint, so ON CONFLICT cannot express
	// the upsert. Lock the existing rows instead: exactly one row is updated,
	// none inserts, and duplicates are reported rather than silently merged.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM provider_manifests WHERE provider_id = $1 FOR UPDATE`, manifest.ProviderID)
	if err != nil {
		return err
	}
	locked := 0
	for rows.Next() {
		locked++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if locked > 1 {
		return fmt.Errorf("provider %q has %d manifest rows; refusing an ambiguous upsert", manifest.ProviderID, locked)
	}
	if locked == 1 {
		_, err = tx.ExecContext(ctx, `
			UPDATE provider_manifests SET
				name = $2, version = $3, protocol_version = $4, capabilities = $5, actions = $6,
				permissions = $7, resource_requirements = $8, dependencies = $9, compatibility = $10,
				conformance_level = $11, conformance_evidence = $12, conformance_expires_at = $13,
				storage_driver_package = $14, updated_at = NOW()
			WHERE provider_id = $1`,
			manifest.ProviderID, manifest.Name, manifest.Version, manifest.ProtocolVersion,
			capabilities, actions, string(permissionsJSON), string(resourceJSON),
			string(depsJSON), string(compatJSON),
			manifest.ConformanceLevel, string(evidenceJSON),
			manifest.ConformanceExpiresAt, string(storageDriverPackageJSON))
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO provider_manifests (
				provider_id, name, version, protocol_version,
				capabilities, actions, permissions, resource_requirements,
				dependencies, compatibility, conformance_level,
				conformance_evidence, conformance_expires_at, storage_driver_package
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			manifest.ProviderID, manifest.Name, manifest.Version, manifest.ProtocolVersion,
			capabilities, actions, string(permissionsJSON), string(resourceJSON),
			string(depsJSON), string(compatJSON),
			manifest.ConformanceLevel, string(evidenceJSON),
			manifest.ConformanceExpiresAt, string(storageDriverPackageJSON))
	}
	if err != nil {
		return err
	}
	return tx.Commit()
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
