package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func reportFixture() PreflightReport {
	observed := time.Date(2026, 10, 11, 10, 0, 0, 0, time.FixedZone("fixture", 3600))
	return PreflightReport{SchemaVersion: PreflightReportSchema, RunID: strings.Repeat("a", 32), ConfigDigest: "sha256:" + strings.Repeat("b", 64), LockDigest: "sha256:" + strings.Repeat("c", 64), GeneratedAt: observed.Add(time.Second), Checks: []PreflightCheck{{CheckID: "node.api-tcp", Scope: "node", Node: "worker", ObservedAt: observed, Status: StatusBlocked, Expected: "worker -> API TCP6443", Actual: "source path unreachable", ReasonCode: "NODE_API_TCP_UNREACHABLE", Remediation: "Check the worker route and API listener."}}}
}

func TestPreflightHumanAndJSONPreserveContract(t *testing.T) {
	report := reportFixture()
	var human, data bytes.Buffer
	if err := WritePreflightReport(&human, &report, "human"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{report.RunID, report.ConfigDigest, report.LockDigest, "checkSetComplete: false", "2026-10-11T09:00:01Z", "observedAt=2026-10-11T09:00:00Z", "expected:", "actual:", "remediation:", "NODE_API_TCP_UNREACHABLE"} {
		if !strings.Contains(human.String(), value) {
			t.Fatalf("human report omitted %s", value)
		}
	}
	if err := WritePreflightReport(&data, &report, "json"); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePreflightReport(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Checks[0].CheckID != report.Checks[0].CheckID || decoded.Checks[0].ReasonCode != report.Checks[0].ReasonCode || decoded.Checks[0].Expected != report.Checks[0].Expected || decoded.CheckSetComplete {
		t.Fatal("JSON report changed observations/completeness")
	}
	if report.GeneratedAt.Location().String() != "fixture" {
		t.Fatal("render mutated the evidence")
	}
}

func TestPreflightRejectsInvalidContractBeforeOutput(t *testing.T) {
	for name, mutate := range map[string]func(*PreflightReport){
		"schema":              func(r *PreflightReport) { r.SchemaVersion = "future/v9" },
		"identity":            func(r *PreflightReport) { r.RunID = "password=secret" },
		"digest":              func(r *PreflightReport) { r.LockDigest = placeholderDigest },
		"observed-time":       func(r *PreflightReport) { r.Checks[0].ObservedAt = r.GeneratedAt.Add(time.Second) },
		"missing-actual":      func(r *PreflightReport) { r.Checks[0].Actual = "" },
		"missing-remediation": func(r *PreflightReport) { r.Checks[0].Remediation = "" },
		"reason":              func(r *PreflightReport) { r.Checks[0].ReasonCode = "CHECK_PASSED" },
		"duplicate":           func(r *PreflightReport) { r.Checks = append(r.Checks, r.Checks[0]) },
		"scope":               func(r *PreflightReport) { r.Checks[0].Scope = "arbitrary" },
		"complete": func(r *PreflightReport) {
			r.CheckSetComplete = true
			r.Checks[0].ReasonCode = "PREFLIGHT_CHECK_SET_INCOMPLETE"
		},
	} {
		t.Run(name, func(t *testing.T) {
			report := reportFixture()
			mutate(&report)
			var out bytes.Buffer
			if err := WritePreflightReport(&out, &report, "json"); err == nil || out.Len() != 0 {
				t.Fatal("invalid evidence was emitted")
			}
		})
	}
	report := reportFixture()
	other := report.Checks[0]
	other.Node = "cp-1"
	report.Checks = append(report.Checks, other)
	if err := report.Validate(); err != nil {
		t.Fatal("same check type on distinct source nodes rejected:", err)
	}
}

func TestPreflightRedactsEntireSensitiveFieldsInBothFormats(t *testing.T) {
	for name, value := range map[string]string{
		"password":         "password=fixture-pass",
		"quoted-password":  `"password": "fixture secret with spaces"`,
		"token":            "Bearer fixture-token",
		"json-token":       `{"access_token":"fixture-token"}`,
		"env-token":        "API_TOKEN=fixture-token",
		"kms":              "HNB_MASTER_KEY=fixture-key-material",
		"signed-query":     "https://host/file?X-Amz-Signature=fixture-signed-url",
		"dsn":              "postgresql://user:fixture-pass@host/db",
		"nats":             "tls://user:fixture-pass@host:4222",
		"generic-userinfo": "https://user:fixture-pass@host/file",
		"pem":              "-----BEGIN PRIVATE KEY-----\nfixture-private-key\n-----END PRIVATE KEY-----",
		"partial-pem":      "-----BEGIN RSA PRIVATE KEY-----\nfixture-private-key",
		"kubeconfig":       "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: fixture-ca\nusers:\n- user:\n    token: fixture-token",
		"jwt":              "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.fixture-signature",
	} {
		t.Run(name, func(t *testing.T) {
			for _, format := range []string{"human", "json"} {
				report := reportFixture()
				report.Checks[0].Expected = value
				report.Checks[0].Actual = value
				report.Checks[0].Remediation = value
				var out bytes.Buffer
				if err := WritePreflightReport(&out, &report, format); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), redacted) || strings.Contains(out.String(), "fixture-pass") || strings.Contains(out.String(), "fixture-private-key") || strings.Contains(out.String(), "fixture-token") || strings.Contains(out.String(), "fixture-ca") || strings.Contains(out.String(), "fixture secret") || strings.Contains(out.String(), "eyJhb") {
					t.Fatal("sensitive field leaked:", name, format)
				}
				if strings.Count(out.String(), redacted) != 3 {
					t.Fatal("not all descriptive fields were fully redacted")
				}
				if report.Checks[0].Actual != value {
					t.Fatal("redaction mutated original evidence")
				}
			}
		})
	}
}

type failingReportWriter struct{}

func (failingReportWriter) Write([]byte) (int, error) { return 0, errors.New("writer unavailable") }

type shortReportWriter struct{}

func (shortReportWriter) Write(data []byte) (int, error) { return len(data) / 2, nil }

func TestReportOutputErrorsAndTerminalControls(t *testing.T) {
	report := reportFixture()
	report.Checks[0].Actual = "safe\nforged line\x1b[31m"
	for _, format := range []string{"human", "json"} {
		if err := WritePreflightReport(failingReportWriter{}, &report, format); err == nil {
			t.Fatal("output failure ignored")
		}
		if err := WritePreflightReport(shortReportWriter{}, &report, format); err == nil {
			t.Fatal("short report write ignored")
		}
	}
	var out bytes.Buffer
	if err := WritePreflightReport(&out, &report, "human"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "\nforged line") {
		t.Fatal("untrusted terminal lines/controls emitted")
	}
	out.Reset()
	if err := WritePreflightReport(&out, &report, "json"); err != nil {
		t.Fatal(err)
	}
	var parsed PreflightReport
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(parsed.Checks[0].Actual, "\x1b") {
		t.Fatal("control escaped JSON and leaked after decoding")
	}
}
