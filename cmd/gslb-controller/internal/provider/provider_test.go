package provider

import (
	"context"
	"testing"

	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
)

// fakeDNS 记录 Apply/Delete 调用，验证 SPI 适配语义。
type fakeDNS struct {
	ensured  []string
	deleted  bool
	failOn   error
}

func (f *fakeDNS) EnsureEndpoint(_ context.Context, name string, _ []dns.DNSRecord) error {
	f.ensured = append(f.ensured, name)
	return f.failOn
}

func (f *fakeDNS) CleanupOrphaned(_ context.Context, _ map[string]bool) error {
	f.deleted = true
	return f.failOn
}

func externalDNSWith(fake *fakeDNS) *ExternalDNS {
	return &ExternalDNS{manager: fake}
}

// TestExternalDNSDeleteUsesCleanup 验证 DeleteRecords 走 CleanupOrphaned。
func TestExternalDNSDeleteUsesCleanup(t *testing.T) {
	fake := &fakeDNS{}
	adapter := externalDNSWith(fake)
	if err := adapter.DeleteRecords(context.Background(), "app.hnb.cloud"); err != nil {
		t.Fatal(err)
	}
	if !fake.deleted {
		t.Fatal("DeleteRecords must invoke CleanupOrphaned")
	}
}

func TestVerifyTargetsRejectsEmpty(t *testing.T) {
	adapter := externalDNSWith(&fakeDNS{})
	if err := adapter.VerifyTargets(context.Background(), "app.hnb.cloud", nil); err != ErrNotVerified {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyTargetsReplaysApply(t *testing.T) {
	fake := &fakeDNS{}
	adapter := externalDNSWith(fake)
	if err := adapter.VerifyTargets(context.Background(), "app.hnb.cloud", []string{"10.0.1.10"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.ensured) != 1 {
		t.Fatalf("ensured = %v", fake.ensured)
	}
}
