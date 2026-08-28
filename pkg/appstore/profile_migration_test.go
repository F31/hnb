package appstore

import (
	"context"
	"testing"
)

type inMemoryProfileMigrationProvider struct {
	seen ProfileMigrationCommand
}

func (p *inMemoryProfileMigrationProvider) MigrateProfile(_ context.Context, cmd ProfileMigrationCommand) (map[string]any, error) {
	p.seen = cmd
	return map[string]any{"artifact_digest": cmd.ArtifactDigest, "copied": true}, nil
}

func TestProfileMigrationProviderContractCarriesIDsAndDigestOnly(t *testing.T) {
	provider := &inMemoryProfileMigrationProvider{}
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	checkpoint, err := provider.MigrateProfile(context.Background(), ProfileMigrationCommand{
		MigrationID: "migration-a", TenantID: "tenant-a", ArtifactID: "artifact-a",
		SourceProfileID: "profile-source", TargetProfileID: "profile-target", ArtifactDigest: digest,
		Checkpoint: map[string]any{"step": "verify"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.seen.ArtifactDigest != digest || checkpoint["artifact_digest"] != digest {
		t.Fatalf("digest was not preserved: seen=%+v checkpoint=%+v", provider.seen, checkpoint)
	}
	if provider.seen.SourceProfileID == "" || provider.seen.TargetProfileID == "" {
		t.Fatalf("profile IDs missing from command: %+v", provider.seen)
	}
}
