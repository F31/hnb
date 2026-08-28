package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/F31/hnb/pkg/core"
)

type ProviderBindingStore struct {
	db *sql.DB
}

func NewProviderBindingStore(db *sql.DB) *ProviderBindingStore {
	return &ProviderBindingStore{db: db}
}

func (s *ProviderBindingStore) List() ([]core.ProviderBinding, error) {
	rows, err := s.db.Query(`
		SELECT id, cluster_id, provider, version, phase, ref_count,
		       health_failures, last_health_at, last_error, created_at, updated_at
		FROM provider_bindings
		ORDER BY cluster_id, provider`)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	var result []core.ProviderBinding
	for rows.Next() {
		var b core.ProviderBinding
		var lastHealthAt sql.NullTime
		var lastError sql.NullString
		err := rows.Scan(&b.ID, &b.ClusterID, &b.Provider, &b.Version, &b.Phase,
			&b.RefCount, &b.HealthFailures, &lastHealthAt, &lastError,
			&b.CreatedAt, &b.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		if lastHealthAt.Valid {
			b.LastHealthAt = &lastHealthAt.Time
		}
		if lastError.Valid {
			b.LastError = lastError.String
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (s *ProviderBindingStore) GetByClusterAndProvider(clusterID, provider string) (*core.ProviderBinding, error) {
	var b core.ProviderBinding
	var lastHealthAt sql.NullTime
	var lastError sql.NullString
	err := s.db.QueryRow(`
		SELECT id, cluster_id, provider, version, phase, ref_count,
		       health_failures, last_health_at, last_error, created_at, updated_at
		FROM provider_bindings
		WHERE cluster_id = $1 AND provider = $2`, clusterID, provider).
		Scan(&b.ID, &b.ClusterID, &b.Provider, &b.Version, &b.Phase,
			&b.RefCount, &b.HealthFailures, &lastHealthAt, &lastError,
			&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lastHealthAt.Valid {
		b.LastHealthAt = &lastHealthAt.Time
	}
	if lastError.Valid {
		b.LastError = lastError.String
	}
	return &b, nil
}

func (s *ProviderBindingStore) Upsert(b *core.ProviderBinding) error {
	_, err := s.db.Exec(`
		INSERT INTO provider_bindings (id, cluster_id, provider, version, phase, ref_count,
		                               health_failures, last_health_at, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (cluster_id, provider) DO UPDATE SET
			version = EXCLUDED.version,
			phase = EXCLUDED.phase,
			ref_count = EXCLUDED.ref_count,
			health_failures = EXCLUDED.health_failures,
			last_health_at = EXCLUDED.last_health_at,
			last_error = EXCLUDED.last_error,
			updated_at = EXCLUDED.updated_at`,
		b.ID, b.ClusterID, b.Provider, b.Version, string(b.Phase),
		b.RefCount, b.HealthFailures, b.LastHealthAt, nullIfEmpty(b.LastError),
		b.CreatedAt, time.Now())
	return err
}

func (s *ProviderBindingStore) UpdatePhase(id string, phase core.ProviderPhase, healthFailures int, lastError string) error {
	_, err := s.db.Exec(`
		UPDATE provider_bindings
		SET phase = $2, health_failures = $3, last_error = $4, updated_at = NOW()
		WHERE id = $1`, id, string(phase), healthFailures, nullIfEmpty(lastError))
	return err
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}