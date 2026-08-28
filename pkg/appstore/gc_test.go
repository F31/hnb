package appstore

import (
	"context"
	"testing"
)

type inMemoryGCProvider struct {
	seen GCSweepCommand
}

func (p *inMemoryGCProvider) SweepArtifact(_ context.Context, cmd GCSweepCommand) error {
	p.seen = cmd
	return nil
}

func TestGCProviderSweepCommandCarriesDigestOnly(t *testing.T) {
	provider := &inMemoryGCProvider{}
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	err := provider.SweepArtifact(context.Background(), GCSweepCommand{TombstoneID: "tombstone-a", TenantID: "tenant-a", ArtifactID: "artifact-a", ArtifactDigest: digest, OperationID: "operation-a"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.seen.ArtifactDigest != digest || provider.seen.ArtifactID == "" || provider.seen.TombstoneID == "" {
		t.Fatalf("invalid sweep command: %+v", provider.seen)
	}
}
