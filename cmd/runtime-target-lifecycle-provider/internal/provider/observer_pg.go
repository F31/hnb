package provider

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PGObserverRegistry struct {
	db *sql.DB
}

func NewPGObserverRegistry(db *sql.DB) *PGObserverRegistry {
	return &PGObserverRegistry{db: db}
}

func (r *PGObserverRegistry) Register(ctx context.Context, tenantID, targetID, targetKind, observerKind string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("observer registry database is not configured")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE runtime_targets
		SET observation_source = $4,
			observation_source_id = $5,
			observation_generation = CASE
				WHEN observation_source = $4 AND observation_source_id = $5 THEN COALESCE(observation_generation, 0)
				ELSE COALESCE(observation_generation, 0) + 1 END,
			observation_revision = CASE
				WHEN observation_source = $4 AND observation_source_id = $5 THEN observation_revision ELSE 0 END,
			updated_at = CASE
				WHEN observation_source = $4 AND observation_source_id = $5 THEN updated_at ELSE now() END
		WHERE id = $1::uuid AND tenant_id = $2 AND target_type = $3`,
		targetID, tenantID, targetTypeForKind(targetKind), observationSource(observerKind), observerKind+":"+targetID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("runtime target %s not found for observer registration", targetID)
	}
	return nil
}

func (r *PGObserverRegistry) Unregister(ctx context.Context, tenantID, targetID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("observer registry database is not configured")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE runtime_targets
		SET observation_source = NULL,
			observation_source_id = NULL,
			observation_generation = CASE
				WHEN observation_source IS NULL AND observation_source_id IS NULL THEN COALESCE(observation_generation, 0)
				ELSE COALESCE(observation_generation, 0) + 1 END,
			observation_revision = CASE
				WHEN observation_source IS NULL AND observation_source_id IS NULL THEN observation_revision ELSE 0 END,
			updated_at = CASE
				WHEN observation_source IS NULL AND observation_source_id IS NULL THEN updated_at ELSE now() END
		WHERE id = $1::uuid AND tenant_id = $2`,
		targetID, tenantID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("runtime target %s not found for observer unregistration", targetID)
	}
	return nil
}

func targetTypeForKind(targetKind string) string {
	switch targetKind {
	case "EdgeRuntimeTarget":
		return "edge_runtime"
	default:
		return "kubernetes"
	}
}

func observationSource(observerKind string) string {
	switch strings.ToLower(observerKind) {
	case "cloudcore":
		return "cloudcore"
	default:
		return "agent"
	}
}
