package toolkit

import (
	"context"
	"strings"
	"testing"
)

func TestUbuntu2604ScopedAdaptationAndOwnershipFence(t *testing.T) {
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.NewReplacer("osVersion=24.04", "osVersion=26.04", "kernel=6.8.0-31-generic", "kernel=7.0.0-30-generic", "owner=none", "owner=k3s").Replace(nodeProbeFixture)
	checks := ProbeNodes(context.Background(), cfg, &fixtureProbe{response: raw})
	var observed, fenced bool
	for _, check := range checks {
		if check.CheckID == "node.os-kernel" && check.Status == StatusPass {
			observed = true
		}
		if check.CheckID == "node.owner-markers" && check.ReasonCode == "NODE_OWNER_PRESENT" {
			fenced = true
		}
	}
	if !observed || !fenced {
		t.Fatalf("OS recognition lost the existing owner's fence: %+v", checks)
	}
	if containsStr(CurrentEvidence.CertifiedOS, "ubuntu-26.04") {
		t.Fatal("existing K3s verification was added to the bootstrap-certified OS list")
	}
	for _, route := range CurrentEvidence.Routes {
		if route.Name == "bootstrap" && route.Status == "certified" {
			t.Fatal("existing K3s verification was promoted into kubeadm bootstrap certification")
		}
	}
	for _, tc := range [][3]string{
		{"ubuntu-26.04", "7.0.0-unverified", "amd64"},
		{"ubuntu-26.04", "7.0.0-30-generic", "arm64"},
		{"ubuntu-26.10", "7.0.0-30-generic", "amd64"},
	} {
		if CurrentEvidence.ObservedOSKernelSupported(tc[0], tc[1], tc[2]) {
			t.Fatalf("unobserved platform promoted: %v", tc)
		}
	}
	lock := &ReleaseLock{Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
	checks = ProbeLockedNodes(context.Background(), cfg, lock, &fixtureProbe{response: raw})
	for _, check := range checks {
		if check.ReasonCode == "NODE_OS_RECIPE_MISMATCH" {
			return
		}
	}
	t.Fatal("26.04 silently reused a 24.04 release lock")
}

func TestHostOSInspectionPreservesScopedEvidenceAndFences(t *testing.T) {
	raw := strings.NewReplacer("osVersion=24.04", "osVersion=26.04", "kernel=6.8.0-31-generic", "kernel=7.0.0-30-generic", "owner=none", "owner=k3s", "swapEntries=0", "swapEntries=1").Replace(nodeProbeFixture)
	report, err := hostOSReport(raw, CurrentEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != "host-os/v1" || !report.Adapted || !report.OSKernelVerified || report.BootstrapCertified || report.Owner != "k3s" || report.ActiveSwapEntries != 1 || len(report.Evidence) != 1 || report.Evidence[0].Route != "existing" {
		t.Fatalf("local inspection changed compatibility/ownership: %+v", report)
	}
	if _, err := hostOSReport(raw+"secret=do-not-report\n", CurrentEvidence); err == nil {
		t.Fatal("inspection accepted unbounded extra probe fields")
	}
	unknown, err := hostOSReport(raw, nil)
	if err != nil || unknown.OSKernelVerified || unknown.BootstrapCertified {
		t.Fatal("missing evidence granted compatibility")
	}
}
