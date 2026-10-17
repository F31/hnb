package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/core"
)

func (f *fakeStore) RegisterConformance(_ context.Context, signed core.SignedConformanceReport) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	manifest := f.manifests[signed.Report.ProviderID]
	if manifest == nil {
		return store.ErrNotFound
	}
	if !store.ConformanceMatchesManifest(manifest, signed.Report) {
		return store.ErrConformanceIdentityChanged
	}
	manifest.ConformanceLevel = "production_ready"
	manifest.ConformanceExpiresAt = &signed.Report.ExpiresAt
	manifest.ConformanceEvidence = []store.ConformanceEvidence{{Category: "runtime-target-lifecycle",
		TestName: signed.Report.MatrixVersion + "/KubernetesTarget/import", Passed: true, EvidenceRef: signed.EvidenceDigest()}}
	return nil
}

func TestConformanceRegistration(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := engine.DefaultCompatibilityMatrix().Evaluate(engine.IntentImportRuntimeTarget, "KubernetesTarget")
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("b", 64)
	for _, test := range []struct {
		name   string
		trust  bool
		permit bool
		change func(*core.ConformanceReport)
		status int
	}{
		{name: "passed", trust: true, permit: true, status: 201},
		{name: "untrusted signer", trust: true, permit: true, status: 400},
		{name: "tampered signature", trust: true, permit: true, status: 400},
		{name: "trust not configured", permit: true, status: 503},
		{name: "permission denied", trust: true, status: 403},
		{name: "artifact mismatch", trust: true, permit: true, change: func(r *core.ConformanceReport) { r.ArtifactDigest = "sha256:" + strings.Repeat("c", 64) }, status: 409},
		{name: "manifest changed", trust: true, permit: true, change: func(r *core.ConformanceReport) { r.ManifestDigest = digest }, status: 409},
		{name: "matrix mismatch", trust: true, permit: true, change: func(r *core.ConformanceReport) { r.MatrixVersion = "unknown" }, status: 409},
		{name: "failed assertion", trust: true, permit: true, change: func(r *core.ConformanceReport) { r.Assertions[0].Passed = false }, status: 400},
		{name: "wrong provider", trust: true, permit: true, change: func(r *core.ConformanceReport) { r.ProviderID = "other" }, status: 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := newFakeStore()
			manifest := &store.ProviderManifest{ProviderID: decision.ProviderID, Version: "test-version", ProtocolVersion: "2.0.0", Actions: []string{"import"}, ConformanceLevel: "none", Compatibility: map[string]any{"artifactDigest": digest}}
			st.manifests[manifest.ProviderID] = manifest
			var srv *Server
			if test.permit {
				srv = NewServer(st, testAuthenticator{})
			} else {
				srv = NewServer(st, noPermissionAuthenticator{})
			}
			if test.trust {
				srv.ConfigureConformanceKeys(map[string]ed25519.PublicKey{"fixture": public})
			}
			now := time.Now().UTC()
			r := core.ConformanceReport{SchemaVersion: core.ConformanceReportSchema, RunID: "fixture", Environment: "isolated fixture", SourceDigest: digest,
				ProviderID: manifest.ProviderID, ProviderVersion: manifest.Version, ArtifactDigest: digest, ManifestDigest: store.ManifestIdentityDigest(manifest),
				MatrixVersion: decision.MatrixVersion, TargetKind: "KubernetesTarget", Action: "import", StartedAt: now.Add(-time.Minute), FinishedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}
			for _, category := range []string{"contract", "functional", "security", "fault", "idempotency", "fencing"} {
				r.Assertions = append(r.Assertions, core.ConformanceAssertion{Name: category, Category: category, Passed: true, EvidenceRef: digest})
			}
			if test.change != nil {
				test.change(&r)
			}
			data, _ := json.Marshal(r)
			signed := core.SignedConformanceReport{SignerID: "fixture", Report: r, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, data))}
			if test.name == "untrusted signer" {
				signed.SignerID = "unknown"
			}
			if test.name == "tampered signature" {
				signed.Report.RunID = "tampered"
			}
			body, _ := json.Marshal(signed)
			response := doRequest(t, srv, http.MethodPost, "/v1/providers/"+manifest.ProviderID+"/conformance", string(body))
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body)
			}
			if test.status != 201 && manifest.ConformanceLevel != "none" {
				t.Fatal("rejected report promoted provider")
			}
			if test.status == 201 {
				if _, err := (lifecycleProviderResolver{store: st}).ResolveLifecycleProvider(context.Background(), decision); err != nil {
					t.Fatalf("registered cell not eligible: %v", err)
				}
			}
		})
	}
}
