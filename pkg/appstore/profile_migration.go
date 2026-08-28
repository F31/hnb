package appstore

import "context"

type ProfileMigrationCommand struct {
	MigrationID     string         `json:"migration_id"`
	TenantID        string         `json:"tenant_id"`
	ArtifactID      string         `json:"artifact_id"`
	SourceProfileID string         `json:"source_profile_id"`
	TargetProfileID string         `json:"target_profile_id"`
	ArtifactDigest  string         `json:"artifact_digest"`
	Checkpoint      map[string]any `json:"checkpoint,omitempty"`
}

type ProfileMigrationProvider interface {
	MigrateProfile(ctx context.Context, cmd ProfileMigrationCommand) (map[string]any, error)
}
