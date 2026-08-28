package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/appstore"
)

func TestDistributionTransitions(t *testing.T) {
	if !CanTransitionDistribution(appstore.DistributionPending, appstore.DistributionSyncing) {
		t.Fatal("pending should transition to syncing")
	}
	if !CanTransitionDistribution(appstore.DistributionReady, appstore.DistributionStale) {
		t.Fatal("ready should transition to stale after cache loss")
	}
	if CanTransitionDistribution(appstore.DistributionPending, appstore.DistributionReady) {
		t.Fatal("pending must not skip syncing and become ready")
	}
}

func TestDistributionRepoCreateValidatesTenantAuthorityAndDigest(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	target := &appstore.ArtifactDistributionTarget{
		ID: "target-a", TenantID: "tenant-a", ArtifactID: "artifact-a", AuthorityProfileID: "profile-authority", TargetProfileID: "profile-edge",
		TargetRole: appstore.DistributionEdgeCache, DesiredDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		HighWatermarkBytes: 100, CurrentBytes: 120, IdempotencyKey: "target-a",
	}
	if err := NewDistributionRepo(db).Create(target); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("create error = %v", err)
	}
	for _, fragment := range []string{"artifact_distribution_targets", "authority.authority_role='authoritative'", "a.digest=$7", "a.lifecycle_state='active'"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("create query missing %q: %s", fragment, capture.query)
		}
	}
}

func TestDistributionEvictionCandidatesExcludeAuthorityAndLockedCopies(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	_, err := NewDistributionRepo(db).EvictionCandidates("tenant-a", 10)
	if err != nil {
		t.Fatalf("eviction candidates error = %v", err)
	}
	for _, fragment := range []string{"target_role='edge_cache'", "local_lock=false", "current_bytes > high_watermark_bytes", "target_profile_id <> authority_profile_id"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("eviction query missing %q: %s", fragment, capture.query)
		}
	}
}
