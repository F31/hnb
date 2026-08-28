package navigation

import (
	"context"

	navapp "github.com/F31/hnb/cmd/apiserver/internal/application/navigation"
)

// CapabilityWrappingRepository merges the staged cluster capability gates into
// the snapshot's capability map before the navigation service filters menus and
// routes. A disabled stage therefore removes its entries from the published
// navigation (KERNEL-016), and the cache key stays stable because the staged
// set is static for the lifetime of the deployment.
type CapabilityWrappingRepository struct {
	inner navapp.MetadataRepository
	staged map[string]bool
}

func NewCapabilityWrappingRepository(inner navapp.MetadataRepository, staged map[string]bool) *CapabilityWrappingRepository {
	return &CapabilityWrappingRepository{inner: inner, staged: staged}
}

func (w *CapabilityWrappingRepository) Snapshot(ctx context.Context, tenantID string, locale string) (navapp.Snapshot, error) {
	snapshot, err := w.inner.Snapshot(ctx, tenantID, locale)
	if err != nil {
		return navapp.Snapshot{}, err
	}
	if snapshot.Capabilities == nil {
		snapshot.Capabilities = make(map[string]bool)
	}
	for name, enabled := range w.staged {
		snapshot.Capabilities[name] = enabled
	}
	return snapshot, nil
}
