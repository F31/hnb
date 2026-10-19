package observer

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// RunFreshnessSweeper keeps the stored freshness projection aligned with the
// observation window without changing lifecycle/health/connectivity state.
func RunFreshnessSweeper(ctx context.Context, db *sql.DB, interval time.Duration) {
	if db == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := db.ExecContext(ctx, `
				UPDATE runtime_targets
				SET freshness_state = CASE
					WHEN observed_at IS NULL THEN 'UNKNOWN'
					WHEN observed_at + COALESCE(NULLIF(stale_threshold_seconds, 0), 300) * interval '1 second' >= now() THEN 'FRESH'
					ELSE 'STALE'
				END
				WHERE observed_at IS NOT NULL
				  AND freshness_state IS DISTINCT FROM CASE
					WHEN observed_at + COALESCE(NULLIF(stale_threshold_seconds, 0), 300) * interval '1 second' >= now() THEN 'FRESH'
					ELSE 'STALE'
				  END`); err != nil {
				log.Printf("[freshness] sweep failed: %v", err)
			}
		}
	}
}
