package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignedConformanceReport(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	digest := "sha256:" + strings.Repeat("a", 64)
	base := ConformanceReport{SchemaVersion: ConformanceReportSchema, RunID: "fixture-run",
		SourceDigest: digest, Environment: "isolated fixture only", ProviderID: "runtime-target.lifecycle.kubernetes",
		ProviderVersion: "test-version", ArtifactDigest: digest, ManifestDigest: digest,
		MatrixVersion: "fixture-matrix", TargetKind: "KubernetesTarget", Action: "import",
		StartedAt: now.Add(-time.Minute), FinishedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
	for _, category := range []string{"contract", "functional", "security", "fault", "idempotency", "fencing"} {
		base.Assertions = append(base.Assertions, ConformanceAssertion{Name: category, Category: category, Passed: true, EvidenceRef: digest})
	}
	for _, test := range []struct {
		name      string
		change    func(*ConformanceReport)
		tamper    bool
		untrusted bool
		valid     bool
	}{
		{name: "valid", valid: true},
		{name: "untrusted signer", untrusted: true},
		{name: "tampered", tamper: true},
		{name: "failed", change: func(r *ConformanceReport) { r.Assertions[0].Passed = false }},
		{name: "missing category", change: func(r *ConformanceReport) { r.Assertions = r.Assertions[1:] }},
		{name: "duplicate", change: func(r *ConformanceReport) { r.Assertions[1].Name = r.Assertions[0].Name }},
		{name: "unaddressed evidence", change: func(r *ConformanceReport) { r.Assertions[0].EvidenceRef = "reports/anything" }},
		{name: "expired at boundary", change: func(r *ConformanceReport) { r.ExpiresAt = now }},
		{name: "future finish", change: func(r *ConformanceReport) { r.FinishedAt = now.Add(time.Second) }},
		{name: "excess validity", change: func(r *ConformanceReport) { r.ExpiresAt = now.Add(31 * 24 * time.Hour) }},
		{name: "unsupported action", change: func(r *ConformanceReport) { r.Action = "create" }},
		{name: "missing source", change: func(r *ConformanceReport) { r.SourceDigest = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := base
			r.Assertions = append([]ConformanceAssertion(nil), base.Assertions...)
			if test.change != nil {
				test.change(&r)
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			signed := SignedConformanceReport{SignerID: "fixture", Report: r, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, data))}
			if test.tamper {
				signed.Report.ProviderVersion = "tampered"
			}
			keys := map[string]ed25519.PublicKey{"fixture": public}
			if test.untrusted {
				keys = nil
			}
			if err := signed.Verify(keys, now); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
}
