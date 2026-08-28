package healthsource

import (
	"fmt"

	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
)

func GenerateDNSRecords(domain string, healthyTargets []ClusterTarget, weights map[string]int, dnsNames map[string]string) []dns.DNSRecord {
	if len(healthyTargets) == 0 {
		return nil
	}

	records := make([]dns.DNSRecord, 0, len(healthyTargets))
	for _, t := range healthyTargets {
		w := weights[t.Name]
		if w <= 0 {
			w = 1
		}
		dnsName := dnsNames[t.Name]
		if dnsName == "" {
			dnsName = fmt.Sprintf("%s.%s", t.Name, domain)
		}
		records = append(records, dns.DNSRecord{
			DNSName: dnsName,
			Targets: []string{t.Endpoint},
			Weight:  w,
			TTL:     30,
			SetID:   t.Name,
		})
	}
	return records
}