package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/F31/hnb/pkg/iam"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type staticClusterProjectionComparator struct {
	diff clusterProjectionDiff
	err  error
}

func (c staticClusterProjectionComparator) Compare(context.Context, string) (clusterProjectionDiff, error) {
	return c.diff, c.err
}

type recordingClusterProjectionRecorder struct {
	recorded bool
	blocked  string
}

func (r *recordingClusterProjectionRecorder) Record(clusterProjectionMode, clusterProjectionDiff, error) {
	r.recorded = true
}

func (r *recordingClusterProjectionRecorder) Blocked(reason string) {
	r.blocked = reason
}

func TestClusterListWhereUsesIdenticalBoundedPredicates(t *testing.T) {
	where, args := clusterListWhere("tenant-a", "edge_runtime", "prod", "ACTIVE", "HEALTHY", "CONNECTED", "FRESH")
	for _, want := range []string{
		"rt.target_type IN ('kubernetes','edge_runtime')",
		"rt.target_type = $2",
		"rt.name ILIKE $3",
		"rt.lifecycle_state = $4",
		"rt.health_state = $5",
		"rt.connectivity_state = $6",
		"rt.freshness_state = $7",
	} {
		if !strings.Contains(where, want) {
			t.Fatalf("where clause missing %q: %s", want, where)
		}
	}
	if len(args) != 7 || args[2] != "%prod%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestClusterListWhereAppliesCombinedStatusWithBadgePrecedence(t *testing.T) {
	where, args := clusterListWhere("tenant-a", "", "", "", "", "", "", "STALE")
	if !strings.Contains(where, "rt.freshness_state = 'STALE'") {
		t.Fatalf("STALE predicate missing: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("combined status must not interpolate values: %#v", args)
	}

	where, _ = clusterListWhere("tenant-a", "", "", "", "", "", "", "RUNNING")
	for _, want := range []string{"rt.lifecycle_state = 'ACTIVE'", "rt.health_state = 'HEALTHY'", "rt.freshness_state <> 'STALE'"} {
		if !strings.Contains(where, want) {
			t.Fatalf("RUNNING predicate missing %q: %s", want, where)
		}
	}
}

func TestClusterProjectionShadowRecordsMismatchWithoutBlocking(t *testing.T) {
	recorder := &recordingClusterProjectionRecorder{}
	h := &ResourceClusterHandler{
		projectionMode:       clusterProjectionShadow,
		projectionComparator: staticClusterProjectionComparator{diff: clusterProjectionDiff{LegacyCount: 2, CanonicalCount: 1}},
		projectionRecorder:   recorder,
	}
	if !h.allowClusterProjectionRead(context.Background(), "tenant-a") {
		t.Fatal("shadow mode blocked the read")
	}
	if !recorder.recorded || recorder.blocked != "" {
		t.Fatalf("unexpected recorder state: %+v", recorder)
	}
}

func TestClusterProjectionCutoverBlocksMismatchBeforeQuery(t *testing.T) {
	recorder := &recordingClusterProjectionRecorder{}
	h := &ResourceClusterHandler{
		projectionMode:       clusterProjectionCutover,
		projectionComparator: staticClusterProjectionComparator{diff: clusterProjectionDiff{StatusMismatch: 1}},
		projectionRecorder:   recorder,
	}
	ctx := iam.WithTrustedContext(context.Background(), iam.TrustedContext{TenantID: "tenant-a"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources/clusters", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ListClusters(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if recorder.blocked != "projection_mismatch" {
		t.Fatalf("blocked reason = %q", recorder.blocked)
	}
}

func TestClusterProjectionCutoverBlocksComparisonError(t *testing.T) {
	recorder := &recordingClusterProjectionRecorder{}
	h := &ResourceClusterHandler{
		projectionMode:       clusterProjectionCutover,
		projectionComparator: staticClusterProjectionComparator{err: errors.New("database unavailable")},
		projectionRecorder:   recorder,
	}
	if h.allowClusterProjectionRead(context.Background(), "tenant-a") {
		t.Fatal("cutover mode allowed a read after comparison failure")
	}
	if recorder.blocked != "comparison_error" {
		t.Fatalf("blocked reason = %q", recorder.blocked)
	}
}

func TestClusterProjectionMetricsRecordDimensions(t *testing.T) {
	countBefore := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("count"))
	statusBefore := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("status"))
	tenantBefore := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("tenant"))

	prometheusClusterProjectionRecorder{}.Record(clusterProjectionShadow, clusterProjectionDiff{
		LegacyCount: 3, CanonicalCount: 2, StatusMismatch: 2, TenantMismatch: 1,
	}, nil)

	if got := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("count")); got != countBefore+1 {
		t.Fatalf("count mismatch metric = %v, want %v", got, countBefore+1)
	}
	if got := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("status")); got != statusBefore+2 {
		t.Fatalf("status mismatch metric = %v, want %v", got, statusBefore+2)
	}
	if got := testutil.ToFloat64(clusterProjectionMismatches.WithLabelValues("tenant")); got != tenantBefore+1 {
		t.Fatalf("tenant mismatch metric = %v, want %v", got, tenantBefore+1)
	}
}
