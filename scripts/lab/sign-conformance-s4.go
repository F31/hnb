// Lab-only conformance report signer. The private key is never committed.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/core"
)

func main() {
	keyPath := flag.String("key", "", "Ed25519 PKCS8 private key")
	out := flag.String("out", "", "signed report output")
	manifestDigest := flag.String("manifest-digest", "", "manifest identity digest")
	artifactDigest := flag.String("artifact-digest", "", "provider image digest")
	sourceDigest := flag.String("source-digest", "", "source tree digest")
	evidenceDir := flag.String("evidence-dir", "", "directory of category logs")
	flag.Parse()
	if *keyPath == "" || *out == "" || *evidenceDir == "" ||
		!core.IsConformanceDigest(*manifestDigest) || !core.IsConformanceDigest(*artifactDigest) || !core.IsConformanceDigest(*sourceDigest) {
		fmt.Fprintln(os.Stderr, "valid key/out/digests/evidence-dir are required")
		os.Exit(2)
	}
	pemData, err := os.ReadFile(*keyPath)
	if err != nil {
		panic(err)
	}
	block, _ := pem.Decode(pemData)
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		panic("not an Ed25519 private key")
	}
	assertions := make([]core.ConformanceAssertion, 0, 6)
	for _, category := range []string{"contract", "functional", "security", "fault", "idempotency", "fencing"} {
		data, err := os.ReadFile(filepath.Join(*evidenceDir, category+".log"))
		if err != nil || !strings.Contains(string(data), "PASS") {
			panic("missing or failed evidence for " + category)
		}
		sum := sha256.Sum256(data)
		assertions = append(assertions, core.ConformanceAssertion{
			Name: "lab-" + category, Category: category, Passed: true,
			EvidenceRef: fmt.Sprintf("sha256:%x", sum),
		})
	}
	now := time.Now().UTC().Truncate(time.Second)
	report := core.ConformanceReport{
		SchemaVersion: core.ConformanceReportSchema, RunID: "s4-import-conformance-20261028",
		SourceDigest: *sourceDigest, Environment: "s4-clean-kind/provider-unit",
		ProviderID: "runtime-target.lifecycle.kubernetes", ProviderVersion: "1.0.0",
		ArtifactDigest: *artifactDigest, ManifestDigest: *manifestDigest, MatrixVersion: "1.0.0",
		TargetKind: "KubernetesTarget", Action: "import",
		StartedAt: now.Add(-5 * time.Minute), FinishedAt: now, ExpiresAt: now.Add(96 * time.Hour),
		Assertions: assertions,
	}
	canonical, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	signed := core.SignedConformanceReport{
		SignerID: "lab-conformance", Report: report,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, canonical)),
	}
	output, err := json.Marshal(signed)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, output, 0o600); err != nil {
		panic(err)
	}
	fmt.Println("signed", report.RunID, "report", *out)
}
