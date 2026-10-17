package core

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const ConformanceReportSchema = "provider-conformance/v1"

type ConformanceAssertion struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Passed      bool   `json:"passed"`
	EvidenceRef string `json:"evidenceRef"`
}

// ConformanceReport is an attestation by an independently trusted test runner.
// Its exact cell scope must not be interpreted as whole-platform certification.
type ConformanceReport struct {
	SchemaVersion   string                 `json:"schemaVersion"`
	RunID           string                 `json:"runId"`
	SourceDigest    string                 `json:"sourceDigest"`
	Environment     string                 `json:"environment"`
	ProviderID      string                 `json:"providerId"`
	ProviderVersion string                 `json:"providerVersion"`
	ArtifactDigest  string                 `json:"artifactDigest"`
	ManifestDigest  string                 `json:"manifestDigest"`
	MatrixVersion   string                 `json:"matrixVersion"`
	TargetKind      string                 `json:"targetKind"`
	Action          string                 `json:"action"`
	StartedAt       time.Time              `json:"startedAt"`
	FinishedAt      time.Time              `json:"finishedAt"`
	ExpiresAt       time.Time              `json:"expiresAt"`
	Assertions      []ConformanceAssertion `json:"assertions"`
}

type SignedConformanceReport struct {
	SignerID  string            `json:"signerId"`
	Report    ConformanceReport `json:"report"`
	Signature string            `json:"signature"`
}

var conformanceDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func IsConformanceDigest(value string) bool { return conformanceDigestPattern.MatchString(value) }

func (s SignedConformanceReport) Verify(keys map[string]ed25519.PublicKey, now time.Time) error {
	key := keys[s.SignerID]
	if len(key) != ed25519.PublicKeySize {
		return errors.New("untrusted conformance signer")
	}
	data, err := json.Marshal(s.Report)
	if err != nil {
		return errors.New("invalid conformance report")
	}
	signature, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil || !ed25519.Verify(key, data, signature) {
		return errors.New("invalid conformance signature")
	}
	if err := validateReportContents(s.Report, now); err != nil {
		return err
	}
	return nil
}

func validateReportContents(r ConformanceReport, now time.Time) error {
	if r.SchemaVersion != ConformanceReportSchema || strings.TrimSpace(r.RunID) == "" ||
		strings.TrimSpace(r.Environment) == "" || strings.TrimSpace(r.ProviderID) == "" ||
		strings.TrimSpace(r.ProviderVersion) == "" || strings.TrimSpace(r.MatrixVersion) == "" ||
		r.TargetKind != "KubernetesTarget" || r.Action != "import" ||
		!IsConformanceDigest(r.SourceDigest) || !IsConformanceDigest(r.ArtifactDigest) || !IsConformanceDigest(r.ManifestDigest) {
		return errors.New("invalid conformance identity or unsupported cell")
	}
	if r.StartedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) || r.FinishedAt.After(now) ||
		!now.Before(r.ExpiresAt) || r.ExpiresAt.After(r.FinishedAt.Add(30*24*time.Hour)) {
		return errors.New("invalid or expired conformance window")
	}
	required := map[string]bool{"contract": false, "functional": false, "security": false,
		"fault": false, "idempotency": false, "fencing": false}
	names := make(map[string]bool)
	for _, assertion := range r.Assertions {
		if !assertion.Passed || strings.TrimSpace(assertion.Name) == "" || names[assertion.Name] ||
			!IsConformanceDigest(assertion.EvidenceRef) {
			return errors.New("failed, duplicate or unaddressed conformance assertion")
		}
		names[assertion.Name] = true
		if _, ok := required[assertion.Category]; !ok {
			return errors.New("unknown conformance assertion category")
		}
		required[assertion.Category] = true
	}
	for _, passed := range required {
		if !passed {
			return errors.New("incomplete conformance assertions")
		}
	}
	return nil
}

func (s SignedConformanceReport) EvidenceDigest() string {
	data, _ := json.Marshal(s)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
