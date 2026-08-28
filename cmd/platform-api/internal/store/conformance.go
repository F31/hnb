package store

import (
	"context"
	"time"
)

func (s *PGStore) ExpireConformance(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
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

func (s *PGStore) CheckProviderConformance(ctx context.Context, providerID string) error {
	manifest, err := s.GetManifest(ctx, providerID)
	if err != nil {
		return nil
	}
	if manifest.ConformanceExpiresAt != nil && !manifest.ConformanceExpiresAt.IsZero() {
		return nil
	}
	return nil
}

func (s *PGStore) UpdateConformanceLevel(ctx context.Context, providerID, level string, expiresAt *time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE provider_manifests SET conformance_level = $1, conformance_expires_at = $2, updated_at = NOW()
		WHERE provider_id = $3`,
		level, expiresAt, providerID,
	)
	return err
}
