package toolkit

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// Report fields are observations, never arbitrary logs. If a credential shape
// still reaches a descriptive field, discard the whole field, including multiline
// values. Removing only the word 'kubeconfig' or a token label would leak content.
var reportSensitiveText = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(password|passwd|pwd|secret|client[-_]?secret|token|access[-_]?token|refresh[-_]?token|api[-_]?token|api[-_]?key|master[-_]?key|private[-_]?key|[a-z0-9_-]+[-_](password|token|secret|master_key)|authorization|dsn|kubeconfig|kubeconfigFile|client-key-data|client-certificate-data|certificate-authority-data)["']?\s*[=:]|\bbearer\s+\S+|\b(postgres(ql)?|nats|tls)://|\b[a-z][a-z0-9+.-]*://[^\s]*[@?]|\beyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+|["']?(clusters|users)["']?\s*:)`)

func SafeReportText(value string) string {
	if reportSensitiveText.MatchString(value) {
		return redacted
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return '\uFFFD'
		}
		return r
	}, strings.ToValidUTF8(value, "\uFFFD"))
}

// WritePreflightReport validates first, sanitizes a copy and uses a single safe
// projection for JSON files/stdout and human output. Caller-owned evidence stays
// unchanged. Human string fields are quoted to prevent injected terminal lines.
func WritePreflightReport(w io.Writer, report *PreflightReport, format string) error {
	if format != "human" && format != "json" {
		return fmt.Errorf("report format must be human or json")
	}
	if err := report.Validate(); err != nil {
		return err
	}
	safe := *report
	safe.GeneratedAt = safe.GeneratedAt.UTC()
	safe.Checks = append([]PreflightCheck(nil), report.Checks...)
	for i := range safe.Checks {
		check := &safe.Checks[i]
		check.ObservedAt = check.ObservedAt.UTC()
		check.Expected = SafeReportText(check.Expected)
		check.Actual = SafeReportText(check.Actual)
		check.Remediation = SafeReportText(check.Remediation)
	}
	if format == "json" {
		data, err := json.MarshalIndent(safe, "", "  ")
		if err != nil {
			return err
		}
		n, err := w.Write(append(data, '\n'))
		if err == nil && n != len(data)+1 {
			return io.ErrShortWrite
		}
		return err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "preflight %s: %d blocked checks\nschema: %s\ngeneratedAt: %s\nconfigDigest: %s\nlockDigest: %s\ncheckSetComplete: %t\n", safe.RunID, safe.BlockingCount(), safe.SchemaVersion, safe.GeneratedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), safe.ConfigDigest, safe.LockDigest, safe.CheckSetComplete)
	for _, check := range safe.Checks {
		fmt.Fprintf(&out, "%s %s scope=%s node=%s reason=%s observedAt=%s\n  expected: %q\n  actual: %q\n", check.Status, check.CheckID, check.Scope, check.Node, check.ReasonCode, check.ObservedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), check.Expected, check.Actual)
		if check.Remediation != "" {
			fmt.Fprintf(&out, "  remediation: %q\n", check.Remediation)
		}
	}
	n, err := io.WriteString(w, out.String())
	if err == nil && n != out.Len() {
		return io.ErrShortWrite
	}
	return err
}
