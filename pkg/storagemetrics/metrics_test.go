package storagemetrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func descriptor() Descriptor {
	return Descriptor{ProviderID: "ceph", Source: "ceph_exporter", FreshFor: 5 * time.Minute, Capabilities: map[Kind]Applicability{
		Capacity: Applicable, Usage: Applicable, IOPS: Unsupported, Throughput: Applicable, Latency: Unknown, Health: Applicable,
	}}
}

func TestNormalizePreservesUnavailableMetricsWithoutValues(t *testing.T) {
	observedAt := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	capacity := float64(1024)
	got, err := Normalize(descriptor(), Scope{TargetID: "target", ResourceKind: "StorageBackend", ResourceUID: "uid"},
		Snapshot{ObservedAt: observedAt, Values: []RawMeasurement{{Kind: Capacity, Status: Known, Value: &capacity}}}, observedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Metrics) != 6 || got.Metrics[0].Unit != "By" || got.Metrics[2].Applicability != Unsupported || got.Metrics[2].Value != nil || got.Metrics[4].Value != nil {
		t.Fatalf("unexpected normalized metrics: %+v", got.Metrics)
	}
}

func TestNormalizeRejectsInventedOrUndeclaredValues(t *testing.T) {
	now := time.Now()
	value := float64(1)
	_, err := Normalize(descriptor(), Scope{TargetID: "target", ResourceKind: "StorageBackend", ResourceUID: "uid"},
		Snapshot{ObservedAt: now, Values: []RawMeasurement{{Kind: IOPS, Status: Known, Value: &value}}}, now)
	if err == nil || !strings.Contains(err.Error(), "non-applicable") {
		t.Fatalf("expected non-applicable value rejection, got %v", err)
	}
	broken := descriptor()
	delete(broken.Capabilities, Latency)
	_, err = Normalize(broken, Scope{TargetID: "target", ResourceKind: "StorageBackend", ResourceUID: "uid"}, Snapshot{ObservedAt: now}, now)
	if err == nil || !strings.Contains(err.Error(), "capability applicability") {
		t.Fatalf("expected applicability rejection, got %v", err)
	}
}

func TestCollectorHasBoundedLabelsAndOmitsUnavailableValues(t *testing.T) {
	now := time.Now().UTC()
	value := float64(42)
	normalized, err := Normalize(descriptor(), Scope{TargetID: "tenant-target", ResourceKind: "PersistentVolumeClaim", ResourceUID: "pvc-secret"},
		Snapshot{ObservedAt: now, Values: []RawMeasurement{{Kind: Capacity, Status: Known, Value: &value}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewCollector(func() []NormalizedSnapshot { return []NormalizedSnapshot{normalized} }))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for _, family := range families {
		_, _ = expfmt.MetricFamilyToText(&output, family)
	}
	text := output.String()
	for _, forbidden := range []string{"tenant-target", "pvc-secret", "volumeHandle"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("telemetry contains forbidden identity %q: %s", forbidden, text)
		}
	}
	if strings.Count(text, `metric="`) != 2 || !strings.Contains(text, `metric="capacity"`) || strings.Contains(text, `metric="iops"`) {
		t.Fatalf("unexpected metrics: %s", text)
	}
}
