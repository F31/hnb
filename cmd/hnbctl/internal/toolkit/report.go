package toolkit

import (
	"fmt"
	"regexp"
	"time"
)

const PreflightReportSchema = "preflight-report/v1"

var checkIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*([._-][a-z0-9]+)*$`)
var reasonCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)
var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// CheckStatus is the verdict of a single preflight check.
type CheckStatus string

const (
	StatusPass    CheckStatus = "pass"
	StatusWarn    CheckStatus = "warn"
	StatusBlocked CheckStatus = "blocked"
)

// PreflightCheck is one side-effect-free observation.
type PreflightCheck struct {
	CheckID     string      `yaml:"checkId" json:"checkId"`
	Scope       string      `yaml:"scope" json:"scope"` // node | cluster | platform | artifacts
	Node        string      `yaml:"node" json:"node,omitempty"`
	ObservedAt  time.Time   `yaml:"observedAt" json:"observedAt"`
	Status      CheckStatus `yaml:"status" json:"status"`
	Expected    string      `yaml:"expected" json:"expected"`
	Actual      string      `yaml:"actual" json:"actual"`
	ReasonCode  string      `yaml:"reasonCode" json:"reasonCode"`
	Remediation string      `yaml:"remediation" json:"remediation"`
}

// PreflightReport is the immutable result of the read-only preflight stage.
type PreflightReport struct {
	SchemaVersion    string           `yaml:"schemaVersion" json:"schemaVersion"`
	CheckSetComplete bool             `yaml:"checkSetComplete" json:"checkSetComplete"`
	RunID            string           `yaml:"runId" json:"runId"`
	ConfigDigest     string           `yaml:"configDigest" json:"configDigest"`
	LockDigest       string           `yaml:"lockDigest" json:"lockDigest"`
	GeneratedAt      time.Time        `yaml:"generatedAt" json:"generatedAt"`
	Checks           []PreflightCheck `yaml:"checks" json:"checks"`
}

// Validate checks identities and observations before any report bytes are
// emitted. Duplicate IDs are disallowed within the same scope/source node.
func (r *PreflightReport) Validate() error {
	if r.SchemaVersion != PreflightReportSchema || !runIDPattern.MatchString(r.RunID) || !digestPattern.MatchString(r.ConfigDigest) || !digestPattern.MatchString(r.LockDigest) || r.ConfigDigest == placeholderDigest || r.LockDigest == placeholderDigest {
		return fmt.Errorf("invalid preflight report schema or identity")
	}
	if r.GeneratedAt.IsZero() || len(r.Checks) == 0 {
		return fmt.Errorf("preflight report requires generation time and observations")
	}
	seen := map[string]bool{}
	for i, check := range r.Checks {
		if !checkIDPattern.MatchString(check.CheckID) || !reasonCodePattern.MatchString(check.ReasonCode) || (check.Node != "" && !validName(check.Node)) {
			return fmt.Errorf("invalid check identity at index %d", i)
		}
		switch check.Scope {
		case "node", "cluster", "platform", "artifacts":
		default:
			return fmt.Errorf("invalid check scope at index %d", i)
		}
		key := check.Scope + "/" + check.Node + "/" + check.CheckID
		if seen[key] {
			return fmt.Errorf("duplicate check identity at index %d", i)
		}
		seen[key] = true
		if check.ObservedAt.IsZero() || check.ObservedAt.After(r.GeneratedAt) || check.Expected == "" || check.Actual == "" {
			return fmt.Errorf("missing or invalid observation at index %d", i)
		}
		switch check.Status {
		case StatusPass, StatusWarn, StatusBlocked:
		default:
			return fmt.Errorf("invalid verdict at index %d", i)
		}
		if check.Status != StatusPass && check.Remediation == "" {
			return fmt.Errorf("missing remediation at index %d", i)
		}
		if check.Status == StatusPass && check.ReasonCode != "CHECK_PASSED" {
			return fmt.Errorf("inconsistent passing reason at index %d", i)
		}
		if check.Status != StatusPass && check.ReasonCode == "CHECK_PASSED" {
			return fmt.Errorf("inconsistent blocked/warn reason at index %d", i)
		}
		if r.CheckSetComplete && check.ReasonCode == "PREFLIGHT_CHECK_SET_INCOMPLETE" {
			return fmt.Errorf("report cannot declare an incomplete check set complete")
		}
	}
	return nil
}

// DecodePreflightReport parses a PreflightReport with strict unknown-field rejection.
func DecodePreflightReport(data []byte) (*PreflightReport, error) {
	var report PreflightReport
	if err := decodeStrict(data, &report); err != nil {
		return nil, err
	}
	if err := report.Validate(); err != nil {
		return nil, err
	}
	return &report, nil
}

// BlockingCount returns the number of blocked checks.
func (r *PreflightReport) BlockingCount() int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == StatusBlocked {
			n++
		}
	}
	return n
}
