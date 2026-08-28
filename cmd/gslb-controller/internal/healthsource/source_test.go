package healthsource

import (
	"context"
	"testing"
	"time"
)

type mockSource struct {
	name   string
	status map[string]string
}

func (m *mockSource) Name() string { return m.name }

func (m *mockSource) Probe(ctx context.Context, targets []ClusterTarget) (map[string]HealthResult, error) {
	results := make(map[string]HealthResult, len(targets))
	for _, t := range targets {
		s := "healthy"
		if m.status != nil {
			if v, ok := m.status[t.Name]; ok {
				s = v
			}
		}
		results[t.Name] = HealthResult{
			Status:    s,
			Source:    m.name,
			Timestamp: time.Now(),
		}
	}
	return results, nil
}

func TestMergeResults_AllHealthy(t *testing.T) {
	results := map[string]map[string]HealthResult{
		"cluster-a": {
			"http":    {Status: "healthy", Source: "http"},
			"karmada": {Status: "healthy", Source: "karmada"},
		},
		"cluster-b": {
			"http":    {Status: "healthy", Source: "http"},
			"karmada": {Status: "unreachable", Source: "karmada"},
		},
	}

	merged := MergeResults(results, MergeAllHealthy)

	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected cluster-a healthy, got %s", merged["cluster-a"].Status)
	}
	if merged["cluster-b"].Status != "unreachable" {
		t.Errorf("expected cluster-b unreachable, got %s", merged["cluster-b"].Status)
	}
}

func TestMergeResults_AnyHealthy(t *testing.T) {
	results := map[string]map[string]HealthResult{
		"cluster-a": {
			"http":    {Status: "healthy", Source: "http"},
			"karmada": {Status: "unreachable", Source: "karmada"},
		},
		"cluster-b": {
			"http":    {Status: "unreachable", Source: "http"},
			"karmada": {Status: "unreachable", Source: "karmada"},
		},
	}

	merged := MergeResults(results, MergeAnyHealthy)

	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected cluster-a healthy, got %s", merged["cluster-a"].Status)
	}
	if merged["cluster-b"].Status != "unreachable" {
		t.Errorf("expected cluster-b unreachable, got %s", merged["cluster-b"].Status)
	}
}

func TestMergeResults_PrimaryFallback(t *testing.T) {
	results := map[string]map[string]HealthResult{
		"cluster-a": {
			"http":    {Status: "healthy", Source: "http"},
			"karmada": {Status: "unreachable", Source: "karmada"},
		},
		"cluster-b": {
			"http":    {Status: "unreachable", Source: "http"},
			"karmada": {Status: "healthy", Source: "karmada"},
		},
		"cluster-c": {
			"http":    {Status: "unreachable", Source: "http"},
			"karmada": {Status: "unreachable", Source: "karmada"},
		},
	}

	merged := MergeResults(results, MergePrimaryFallback)

	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected cluster-a healthy, got %s", merged["cluster-a"].Status)
	}
	if merged["cluster-b"].Status != "healthy" {
		t.Errorf("expected cluster-b healthy (fallback), got %s", merged["cluster-b"].Status)
	}
	if merged["cluster-c"].Status != "unreachable" {
		t.Errorf("expected cluster-c unreachable, got %s", merged["cluster-c"].Status)
	}
}

func TestMergeResults_SingleSource(t *testing.T) {
	results := map[string]map[string]HealthResult{
		"cluster-a": {
			"http": {Status: "healthy", Source: "http"},
		},
	}

	merged := MergeResults(results, MergeAllHealthy)

	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected healthy, got %s", merged["cluster-a"].Status)
	}
}

func TestMergeResults_Empty(t *testing.T) {
	merged := MergeResults(nil, MergeAllHealthy)
	if len(merged) != 0 {
		t.Errorf("expected empty, got %d", len(merged))
	}
}

func TestParseMergePolicy(t *testing.T) {
	tests := []struct {
		input string
		want  MergePolicy
	}{
		{"all-healthy", MergeAllHealthy},
		{"any-healthy", MergeAnyHealthy},
		{"primary-fallback", MergePrimaryFallback},
		{"unknown", MergeAllHealthy},
		{"", MergeAllHealthy},
	}

	for _, tt := range tests {
		got := ParseMergePolicy(tt.input)
		if got != tt.want {
			t.Errorf("ParseMergePolicy(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestHealthManager_Basic(t *testing.T) {
	httpSrc := &mockSource{name: "http", status: map[string]string{"cluster-a": "healthy"}}
	mgr := NewHealthManager([]HealthSource{httpSrc}, MergeAllHealthy)

	ctx := context.Background()
	targets := []ClusterTarget{{Name: "cluster-a", Endpoint: "https://10.0.0.1"}}

	merged := mgr.ProbeAll(ctx, targets)

	if len(merged) != 1 {
		t.Fatalf("expected 1 merged result, got %d", len(merged))
	}
	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected healthy, got %s", merged["cluster-a"].Status)
	}
}

func TestHealthManager_MultiSource(t *testing.T) {
	httpSrc := &mockSource{name: "http", status: map[string]string{"cluster-a": "healthy"}}
	karmadaSrc := &mockSource{name: "karmada", status: map[string]string{"cluster-a": "healthy"}}
	mgr := NewHealthManager([]HealthSource{httpSrc, karmadaSrc}, MergeAllHealthy)

	ctx := context.Background()
	targets := []ClusterTarget{{Name: "cluster-a", Endpoint: "https://10.0.0.1"}}

	merged := mgr.ProbeAll(ctx, targets)

	if merged["cluster-a"].Status != "healthy" {
		t.Errorf("expected healthy from both sources, got %s", merged["cluster-a"].Status)
	}

	allStatuses := mgr.GetAllStatuses()
	if len(allStatuses) != 1 {
		t.Fatalf("expected 1 cluster in statuses, got %d", len(allStatuses))
	}
	if allStatuses["cluster-a"]["http"] != "healthy" {
		t.Errorf("expected http healthy, got %s", allStatuses["cluster-a"]["http"])
	}
	if allStatuses["cluster-a"]["karmada"] != "healthy" {
		t.Errorf("expected karmada healthy, got %s", allStatuses["cluster-a"]["karmada"])
	}
}

func TestHealthManager_HealthyTargets(t *testing.T) {
	httpSrc := &mockSource{name: "http", status: map[string]string{
		"cluster-a": "healthy",
		"cluster-b": "unreachable",
	}}
	mgr := NewHealthManager([]HealthSource{httpSrc}, MergeAllHealthy)

	ctx := context.Background()
	targets := []ClusterTarget{
		{Name: "cluster-a", Endpoint: "https://10.0.0.1"},
		{Name: "cluster-b", Endpoint: "https://10.0.0.2"},
	}

	mgr.ProbeAll(ctx, targets)
	healthy := mgr.HealthyTargets(targets)

	if len(healthy) != 1 {
		t.Fatalf("expected 1 healthy target, got %d", len(healthy))
	}
	if healthy[0].Name != "cluster-a" {
		t.Errorf("expected cluster-a, got %s", healthy[0].Name)
	}
}

func TestHealthManager_GetMergedStatus(t *testing.T) {
	httpSrc := &mockSource{name: "http", status: map[string]string{"cluster-a": "healthy"}}
	mgr := NewHealthManager([]HealthSource{httpSrc}, MergeAllHealthy)

	ctx := context.Background()
	mgr.ProbeAll(ctx, []ClusterTarget{{Name: "cluster-a", Endpoint: "https://10.0.0.1"}})

	status := mgr.GetMergedStatus("cluster-a")
	if status != "healthy" {
		t.Errorf("expected healthy, got %s", status)
	}

	status = mgr.GetMergedStatus("nonexistent")
	if status != "unknown" {
		t.Errorf("expected unknown for nonexistent, got %s", status)
	}
}

func TestHealthManager_GetSourceStatus(t *testing.T) {
	httpSrc := &mockSource{name: "http", status: map[string]string{"cluster-a": "healthy"}}
	mgr := NewHealthManager([]HealthSource{httpSrc}, MergeAllHealthy)

	ctx := context.Background()
	mgr.ProbeAll(ctx, []ClusterTarget{{Name: "cluster-a", Endpoint: "https://10.0.0.1"}})

	status := mgr.GetSourceStatus("cluster-a", "http")
	if status != "healthy" {
		t.Errorf("expected healthy, got %s", status)
	}

	status = mgr.GetSourceStatus("cluster-a", "nonexistent")
	if status != "unknown" {
		t.Errorf("expected unknown for nonexistent source, got %s", status)
	}
}

func TestParseSources_Default(t *testing.T) {
	sources, err := ParseSources("", 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("ParseSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 default source, got %d", len(sources))
	}
	if sources[0].Name() != "http" {
		t.Errorf("expected http source, got %s", sources[0].Name())
	}
}

func TestParseSources_HTTP(t *testing.T) {
	sources, err := ParseSources("http", 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("ParseSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Name() != "http" {
		t.Errorf("expected http, got %s", sources[0].Name())
	}
}

func TestParseSources_Unknown(t *testing.T) {
	sources, err := ParseSources("unknown", 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("ParseSources: %v", err)
	}
	// Should fall back to http
	if len(sources) != 1 {
		t.Fatalf("expected 1 fallback source, got %d", len(sources))
	}
	if sources[0].Name() != "http" {
		t.Errorf("expected http fallback, got %s", sources[0].Name())
	}
}

func TestGenerateDNSRecords(t *testing.T) {
	targets := []ClusterTarget{
		{Name: "cluster-a", Endpoint: "https://10.0.0.1"},
		{Name: "cluster-b", Endpoint: "https://10.0.0.2"},
	}
	weights := map[string]int{"cluster-a": 70, "cluster-b": 30}
	dnsNames := map[string]string{"cluster-a": "app.hnb.cloud"}

	records := GenerateDNSRecords("hnb.cloud", targets, weights, dnsNames)

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	for _, r := range records {
		if r.SetID == "cluster-a" && r.DNSName != "app.hnb.cloud" {
			t.Errorf("expected cluster-a dnsName app.hnb.cloud, got %s", r.DNSName)
		}
		if r.SetID == "cluster-b" && r.DNSName != "cluster-b.hnb.cloud" {
			t.Errorf("expected cluster-b default dnsName, got %s", r.DNSName)
		}
		if r.Weight <= 0 {
			t.Errorf("expected positive weight for %s, got %d", r.SetID, r.Weight)
		}
	}
}

func TestGenerateDNSRecordsEmpty(t *testing.T) {
	records := GenerateDNSRecords("hnb.cloud", nil, nil, nil)
	if records != nil {
		t.Errorf("expected nil for empty targets")
	}
}