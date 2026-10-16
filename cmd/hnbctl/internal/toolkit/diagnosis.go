package toolkit

import (
	"regexp"
	"time"
)

// secretPattern matches common secret shapes so diagnostics can be redacted.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[-_]?key|private[-_]?key|authorization|bearer)\s*[=:]\s*\S+`),
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)kubeconfig`),
	regexp.MustCompile(`(?i)postgres(ql)?://[^\s"']+`),
	regexp.MustCompile(`(?i)nats://[^\s"']+`),
}

const redacted = "[REDACTED]"

// Redact removes common secret forms from diagnostics and normal terminal output.
func Redact(input string) string {
	out := input
	for _, pattern := range secretPatterns {
		out = pattern.ReplaceAllString(out, redacted)
	}
	return out
}

// DiagnosisBundle describes a collected support archive. It never contains
// plaintext credentials, kubeconfigs, tokens or customer data.
type DiagnosisBundle struct {
	SchemaVersion  string    `yaml:"schemaVersion" json:"schemaVersion"`
	BundleID       string    `yaml:"bundleId" json:"bundleId"`
	RunID          string    `yaml:"runId" json:"runId"`
	CollectedAt    time.Time `yaml:"collectedAt" json:"collectedAt"`
	Redacted       bool      `yaml:"redacted" json:"redacted"`
	EntryPaths     []string  `yaml:"entryPaths" json:"entryPaths"`
	ReasonCodes    []string  `yaml:"reasonCodes" json:"reasonCodes"`
	RemediationRef string    `yaml:"remediationRef" json:"remediationRef"`
}

// sensitiveKey matches diagnostic map keys whose values should be dropped.
var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[-_]?key|private[-_]?key|authorization|kubeconfig|dsn|url)`)

// RedactMap redacts sensitive values in a nested map (used for diagnostics).
// Sensitive keys have their entire value dropped regardless of shape.
func RedactMap(input map[string]any) {
	var walk func(key string, v any) any
	walk = func(key string, v any) any {
		if sensitiveKey.MatchString(key) {
			return redacted
		}
		switch typed := v.(type) {
		case string:
			return Redact(typed)
		case map[string]any:
			for k, child := range typed {
				typed[k] = walk(k, child)
			}
		case []any:
			for i, child := range typed {
				typed[i] = walk(key, child)
			}
		}
		return v
	}
	for k, child := range input {
		input[k] = walk(k, child)
	}
}
