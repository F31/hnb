package appstore

import "context"

type GCProvider interface {
	SweepArtifact(ctx context.Context, cmd GCSweepCommand) error
}
