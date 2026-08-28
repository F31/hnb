package navigation

import (
	"context"
	"testing"

	navapp "github.com/F31/hnb/cmd/apiserver/internal/application/navigation"
)

type stubNavRepo struct {
	snapshot navapp.Snapshot
}

func (s stubNavRepo) Snapshot(context.Context, string, string) (navapp.Snapshot, error) {
	return s.snapshot, nil
}

func TestCapabilityWrappingRepositoryMergesStagedGates(t *testing.T) {
	inner := stubNavRepo{snapshot: navapp.Snapshot{Capabilities: map[string]bool{"resource": true}}}
	wrap := NewCapabilityWrappingRepository(inner, map[string]bool{"cluster.read": true, "cluster.write": false})

	snap, err := wrap.Snapshot(context.Background(), "tenant-a", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Capabilities["cluster.read"] {
		t.Fatal("cluster.read must be merged as enabled")
	}
	if snap.Capabilities["cluster.write"] {
		t.Fatal("cluster.write must be merged as disabled")
	}
	if !snap.Capabilities["resource"] {
		t.Fatal("inner capabilities must be preserved")
	}
}
