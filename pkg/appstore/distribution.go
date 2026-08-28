package appstore

import "context"

type DistributionProvider interface {
	RebuildDistribution(ctx context.Context, cmd DistributionRebuildCommand) error
}
