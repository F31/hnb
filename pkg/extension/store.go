package extension

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
)

type ExtensionDBStore struct {
	db *sql.DB
}

func NewExtensionDBStore(db *sql.DB) *ExtensionDBStore {
	return &ExtensionDBStore{db: db}
}

func (s *ExtensionDBStore) Get(id string) (*core.Extension, error) {
	var ext core.Extension
	var manifestJSON, labelsJSON []byte
	var lastError, workspaceID, targetID sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, version, provider_type, workspace_id, target_id,
		       phase, manifest, labels, health_failures, last_error, created_at, updated_at
		FROM extensions WHERE id = $1`, id).
		Scan(&ext.ID, &ext.Name, &ext.Version, &ext.ProviderType,
			&workspaceID, &targetID,
			&ext.Phase, &manifestJSON, &labelsJSON,
			&ext.HealthFailures, &lastError, &ext.CreatedAt, &ext.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if workspaceID.Valid {
		ext.WorkspaceID = workspaceID.String
	}
	if targetID.Valid {
		ext.TargetID = targetID.String
	}
	if lastError.Valid {
		ext.LastError = lastError.String
	}
	if err := json.Unmarshal(manifestJSON, &ext.Manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if labelsJSON != nil {
		json.Unmarshal(labelsJSON, &ext.Labels)
	}

	return &ext, nil
}

func (s *ExtensionDBStore) List() ([]core.Extension, error) {
	rows, err := s.db.Query(`
		SELECT id, name, version, provider_type, workspace_id, target_id,
		       phase, manifest, labels, health_failures, last_error, created_at, updated_at
		FROM extensions ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list extensions: %w", err)
	}
	defer rows.Close()

	var result []core.Extension
	for rows.Next() {
		var ext core.Extension
		var manifestJSON, labelsJSON []byte
		var lastError, workspaceID, targetID sql.NullString

		err := rows.Scan(&ext.ID, &ext.Name, &ext.Version, &ext.ProviderType,
			&workspaceID, &targetID,
			&ext.Phase, &manifestJSON, &labelsJSON,
			&ext.HealthFailures, &lastError, &ext.CreatedAt, &ext.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan extension: %w", err)
		}

		if workspaceID.Valid {
			ext.WorkspaceID = workspaceID.String
		}
		if targetID.Valid {
			ext.TargetID = targetID.String
		}
		if lastError.Valid {
			ext.LastError = lastError.String
		}
		if err := json.Unmarshal(manifestJSON, &ext.Manifest); err != nil {
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		if labelsJSON != nil {
			json.Unmarshal(labelsJSON, &ext.Labels)
		}

		result = append(result, ext)
	}
	return result, rows.Err()
}

func (s *ExtensionDBStore) Create(ext *core.Extension) error {
	if err := ext.Manifest.Validate(); err != nil {
		return fmt.Errorf("validate extension manifest: %w", err)
	}
	manifestJSON, err := json.Marshal(ext.Manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	labelsJSON, err := json.Marshal(ext.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO extensions (id, name, version, provider_type, workspace_id, target_id,
		                        phase, manifest, labels, health_failures, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
		        $7, $8, $9, $10, NULLIF($11, ''), $12, $13)`,
		ext.ID, ext.Name, ext.Version, string(ext.ProviderType),
		nullIfEmpty(ext.WorkspaceID), nullIfEmpty(ext.TargetID),
		string(ext.Phase), string(manifestJSON), string(labelsJSON),
		ext.HealthFailures, nullIfEmpty(ext.LastError),
		time.Now(), time.Now())
	return err
}

func (s *ExtensionDBStore) UpdatePhase(id string, phase core.ExtensionPhase, healthFailures int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE extensions
		SET phase = $2, health_failures = $3, last_error = NULLIF($4, ''), updated_at = NOW()
		WHERE id = $1`, id, string(phase), healthFailures, nullIfEmpty(lastError))
	return err
}

func (s *ExtensionDBStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM extensions WHERE id = $1`, id)
	return err
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
