package appstore

import (
	"context"
	"testing"
)

type inMemoryDistributionProvider struct {
	seen DistributionRebuildCommand
}

func (p *inMemoryDistributionProvider) RebuildDistribution(_ context.Context, cmd DistributionRebuildCommand) error {
	p.seen = cmd
	return nil
}

func TestDistributionProviderCommandCarriesIDsAndDigestOnly(t *testing.T) {
	provider := &inMemoryDistributionProvider{}
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	err := provider.RebuildDistribution(context.Background(), DistributionRebuildCommand{
		TargetID: "target-a", TenantID: "tenant-a", ArtifactID: "artifact-a", AuthorityProfileID: "profile-authority",
		TargetProfileID: "profile-edge", DesiredDigest: digest, OperationID: "operation-a", IdempotencyKey: "rebuild-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.seen.DesiredDigest != digest || provider.seen.AuthorityProfileID == "" || provider.seen.TargetProfileID == "" {
		t.Fatalf("invalid rebuild command: %+v", provider.seen)
	}
}
