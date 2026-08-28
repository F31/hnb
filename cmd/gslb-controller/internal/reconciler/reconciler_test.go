package reconciler

import (
	"testing"

	"github.com/F31/hnb/cmd/gslb-controller/internal/healthsource"
)

func TestGenerateDNSRecordsWithClusterTarget(t *testing.T) {
	targets := []healthsource.ClusterTarget{
		{Name: "cluster-a", Endpoint: "https://10.0.0.1"},
		{Name: "cluster-b", Endpoint: "https://10.0.0.2"},
	}
	weights := map[string]int{"cluster-a": 70, "cluster-b": 30}
	dnsNames := map[string]string{"cluster-a": "app.hnb.cloud", "cluster-b": "app.hnb.cloud"}

	records := healthsource.GenerateDNSRecords("hnb.cloud", targets, weights, dnsNames)

	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
	for _, r := range records {
		if r.DNSName != "app.hnb.cloud" {
			t.Errorf("expected dnsName app.hnb.cloud, got %s", r.DNSName)
		}
		if r.TTL != 30 {
			t.Errorf("expected TTL 30, got %d", r.TTL)
		}
	}
}

func TestGenerateDNSRecordsEmpty(t *testing.T) {
	records := healthsource.GenerateDNSRecords("hnb.cloud", nil, nil, nil)
	if records != nil {
		t.Errorf("expected nil for empty targets")
	}
}

func TestGenerateDNSRecordsDefaultDNSName(t *testing.T) {
	targets := []healthsource.ClusterTarget{
		{Name: "cluster-a", Endpoint: "https://10.0.0.1"},
	}
	records := healthsource.GenerateDNSRecords("hnb.cloud", targets, nil, nil)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].DNSName != "cluster-a.hnb.cloud" {
		t.Errorf("expected cluster-a.hnb.cloud, got %s", records[0].DNSName)
	}
}

func TestGenerateDNSRecordsDefaultWeight(t *testing.T) {
	targets := []healthsource.ClusterTarget{
		{Name: "cluster-a", Endpoint: "https://10.0.0.1"},
	}
	records := healthsource.GenerateDNSRecords("hnb.cloud", targets, nil, nil)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Weight != 1 {
		t.Errorf("expected default weight 1, got %d", records[0].Weight)
	}
}