package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/F31/hnb/pkg/core"
)

type ProviderRegistryStore struct {
	db *sql.DB
}

func NewProviderRegistryStore(db *sql.DB) *ProviderRegistryStore {
	return &ProviderRegistryStore{db: db}
}

func (s *ProviderRegistryStore) Save(entry *core.ProviderEntry) error {
	configJSON, err := json.Marshal(entry.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	targetID := ""
	if entry.RuntimeTarget != nil {
		targetID = entry.RuntimeTarget.ID
	}

	_, err = s.db.Exec(`
		INSERT INTO provider_registry (provider_id, provider_type, target_id, name, version, config, capability_pack, is_default, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9)
		ON CONFLICT (provider_id) DO UPDATE SET
			version = EXCLUDED.version,
			config = EXCLUDED.config,
			capability_pack = EXCLUDED.capability_pack,
			is_default = EXCLUDED.is_default,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()`,
		entry.ProviderID, string(entry.ProviderType), targetID,
		entry.Name, entry.Version, string(configJSON),
		entry.CapabilityPack, entry.IsDefault, entry.IsActive)
	return err
}

func (s *ProviderRegistryStore) Delete(providerID string) error {
	_, err := s.db.Exec(`DELETE FROM provider_registry WHERE provider_id = $1`, providerID)
	return err
}

func (s *ProviderRegistryStore) List() ([]core.ProviderEntry, error) {
	rows, err := s.db.Query(`
		SELECT provider_id, provider_type, target_id, name, version, config, capability_pack, is_default, is_active
		FROM provider_registry ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	var result []core.ProviderEntry
	for rows.Next() {
		var entry core.ProviderEntry
		var configStr string
		var capPack, targetID sql.NullString
		err := rows.Scan(&entry.ProviderID, &entry.ProviderType, &targetID,
			&entry.Name, &entry.Version, &configStr, &capPack, &entry.IsDefault, &entry.IsActive)
		if err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		if targetID.Valid {
			entry.RuntimeTarget = &core.RuntimeTarget{ID: targetID.String}
		}
		if capPack.Valid {
			entry.CapabilityPack = capPack.String
		}
		if configStr != "" {
			json.Unmarshal([]byte(configStr), &entry.Config)
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *ProviderRegistryStore) Get(providerID string) (*core.ProviderEntry, error) {
	var entry core.ProviderEntry
	var configStr string
	var capPack, targetID sql.NullString
	err := s.db.QueryRow(`
		SELECT provider_id, provider_type, target_id, name, version, config, capability_pack, is_default, is_active
		FROM provider_registry WHERE provider_id = $1`, providerID).
		Scan(&entry.ProviderID, &entry.ProviderType, &targetID,
			&entry.Name, &entry.Version, &configStr, &capPack, &entry.IsDefault, &entry.IsActive)
	if err != nil {
		return nil, err
	}
	if targetID.Valid {
		entry.RuntimeTarget = &core.RuntimeTarget{ID: targetID.String}
	}
	if capPack.Valid {
		entry.CapabilityPack = capPack.String
	}
	if configStr != "" {
		json.Unmarshal([]byte(configStr), &entry.Config)
	}
	return &entry, nil
}