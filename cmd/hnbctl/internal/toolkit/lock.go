package toolkit

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	LockSchemaVersion = "release-lock/v1"
	placeholderDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ReleaseLock pins every install artifact to a real content identity.
type ReleaseLock struct {
	Status                string            `yaml:"status,omitempty" json:"status,omitempty"`
	CLI                   []PinnedCLI       `yaml:"cli,omitempty" json:"cli,omitempty"`
	SourceRedirectOrigins []string          `yaml:"sourceRedirectOrigins,omitempty" json:"sourceRedirectOrigins,omitempty"`
	SchemaVersion         string            `yaml:"schemaVersion" json:"schemaVersion"`
	ReleaseID             string            `yaml:"releaseId" json:"releaseId"`
	ReleaseVersion        string            `yaml:"releaseVersion" json:"releaseVersion"`
	Images                []PinnedImage     `yaml:"images" json:"images"`
	Charts                []PinnedChart     `yaml:"charts" json:"charts"`
	NodeArtifacts         []NodeArtifact    `yaml:"nodeArtifacts" json:"nodeArtifacts"`
	Migrations            []Migration       `yaml:"migrations" json:"migrations"`
	Compatibility         LockCompatibility `yaml:"compatibility" json:"compatibility"`
	// Release-train (release-lock/v2) fields. They are optional so a v1 lock
	// remains fully readable; upgrade paths exist only when declared here.
	Profiles          []LockProfileContract `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	SupportedUpgrades []LockUpgradePath     `yaml:"supportedUpgrades,omitempty" json:"supportedUpgrades,omitempty"`
	MigrationChain    []MigrationStep       `yaml:"migrationChain,omitempty" json:"migrationChain,omitempty"`
}

// LockProfileContract declares the requirements of one profile inside the train.
type LockProfileContract struct {
	Name                 string   `yaml:"name" json:"name"`
	RequiredCapabilities []string `yaml:"requiredCapabilities,omitempty" json:"requiredCapabilities,omitempty"`
	MinKubernetesVersion string   `yaml:"minKubernetesVersion,omitempty" json:"minKubernetesVersion,omitempty"`
}

// LockUpgradePath is one allowed from->to transition (profile name or version).
type LockUpgradePath struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

// MigrationStep is one ordered, owner-scoped schema transition.
type MigrationStep struct {
	Owner      string `yaml:"owner" json:"owner"`
	Name       string `yaml:"name" json:"name"`
	FromSchema string `yaml:"fromSchema" json:"fromSchema"`
	ToSchema   string `yaml:"toSchema" json:"toSchema"`
	Checksum   string `yaml:"checksum" json:"checksum"`
	Reversible bool   `yaml:"reversible,omitempty" json:"reversible,omitempty"`
}

type PinnedImage struct {
	Name           string `yaml:"name" json:"name"`
	Ref            string `yaml:"ref" json:"ref"` // fully qualified repository:tag
	Digest         string `yaml:"digest" json:"digest"`
	ManifestDigest string `yaml:"manifestDigest,omitempty" json:"manifestDigest,omitempty"`
	Platform       string `yaml:"platform,omitempty" json:"platform,omitempty"`
}

type PinnedCLI struct {
	OS     string `yaml:"os" json:"os"`
	Arch   string `yaml:"arch" json:"arch"`
	Ref    string `yaml:"ref" json:"ref"`
	Digest string `yaml:"digest" json:"digest"`
}

type PinnedChart struct {
	Ref     string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	Digest  string `yaml:"digest" json:"digest"`
}

type NodeArtifact struct {
	OS     string `yaml:"os" json:"os"`
	Arch   string `yaml:"arch" json:"arch"`
	Kind   string `yaml:"kind" json:"kind"` // binary | package | container
	Ref    string `yaml:"ref" json:"ref"`
	Digest string `yaml:"digest" json:"digest"`
}

type Migration struct {
	Owner    string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Name     string `yaml:"name" json:"name"`
	Checksum string `yaml:"checksum" json:"checksum"`
}

type LockCompatibility struct {
	OS                 []string `yaml:"os" json:"os"`
	Arch               []string `yaml:"arch" json:"arch"`
	KubernetesVersions []string `yaml:"kubernetesVersions" json:"kubernetesVersions"`
	Profiles           []string `yaml:"profiles" json:"profiles"`
	Reports            []string `yaml:"reports" json:"reports"`
}

// DecodeReleaseLock parses a ReleaseLock with strict unknown-field rejection.
func DecodeReleaseLock(data []byte) (*ReleaseLock, error) {
	var lock ReleaseLock
	if err := decodeStrict(data, &lock); err != nil {
		return nil, err
	}
	return &lock, nil
}

// Validate rejects placeholders, latest tags, missing digests and unsupported
// combinations before any install side effect.
func (l *ReleaseLock) Validate() []string {
	var problems []string
	if l.Status != "" && l.Status != "candidate" {
		problems = append(problems, "unsupported lock status; generation cannot certify a release")
	}
	if l.Status == "candidate" && (len(l.CLI) == 0 || len(l.Charts) == 0 || len(l.Migrations) == 0) {
		problems = append(problems, "candidate lock requires CLI, Chart and owner migrations")
	}
	for i, cli := range l.CLI {
		if cli.OS != "linux" || cli.Arch != "amd64" || !validName(cli.Ref) || !digestPattern.MatchString(cli.Digest) || cli.Digest == placeholderDigest {
			problems = append(problems, fmt.Sprintf("invalid CLI identity at index %d", i))
		}
	}
	if l.SchemaVersion != LockSchemaVersion {
		problems = append(problems, fmt.Sprintf("unsupported lock schemaVersion %q (want %q)", l.SchemaVersion, LockSchemaVersion))
	}
	if strings.TrimSpace(l.ReleaseID) == "" {
		problems = append(problems, "releaseId is required")
	}
	if strings.TrimSpace(l.ReleaseVersion) == "" {
		problems = append(problems, "releaseVersion is required")
	}
	if len(l.Images) == 0 {
		problems = append(problems, "lock contains no images")
	}
	for i, img := range l.Images {
		if img.ManifestDigest != "" && (!digestPattern.MatchString(img.ManifestDigest) || img.ManifestDigest == placeholderDigest || img.Platform != "linux/amd64") {
			problems = append(problems, fmt.Sprintf("invalid platform manifest identity at image index %d", i))
		}
		if l.Status == "candidate" && img.ManifestDigest == "" {
			problems = append(problems, fmt.Sprintf("missing platform identity at image index %d", i))
		}
		if strings.TrimSpace(img.Name) == "" {
			problems = append(problems, fmt.Sprintf("images[%d].name is required", i))
		}
		if strings.TrimSpace(img.Digest) == "" || img.Digest == placeholderDigest || !digestPattern.MatchString(img.Digest) {
			problems = append(problems, fmt.Sprintf("images[%d] (%s) has no real digest", i, img.Name))
		}
		refTag := img.Ref[strings.LastIndex(img.Ref, ":")+1:]
		if refTag == "latest" {
			problems = append(problems, fmt.Sprintf("images[%d] (%s) must not use the latest tag", i, img.Name))
		}
	}
	for i, chart := range l.Charts {
		if l.Status == "candidate" && (!validName(chart.Name) || !tagPattern.MatchString(chart.Version) || strings.EqualFold(chart.Version, "latest") || chart.Ref == "") {
			problems = append(problems, fmt.Sprintf("invalid chart identity at index %d", i))
		}
		if strings.TrimSpace(chart.Digest) == "" || chart.Digest == placeholderDigest || !digestPattern.MatchString(chart.Digest) {
			problems = append(problems, fmt.Sprintf("charts[%d] (%s) has no real digest", i, chart.Name))
		}
	}
	for i, node := range l.NodeArtifacts {
		if strings.TrimSpace(node.Digest) == "" || node.Digest == placeholderDigest || node.Digest == strings.TrimPrefix(placeholderDigest, "sha256:") || !digestPattern.MatchString(node.Digest) && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(node.Digest) {
			problems = append(problems, fmt.Sprintf("nodeArtifacts[%d] (%s) has no real digest", i, node.Ref))
		}
	}
	for i, m := range l.Migrations {
		if l.Status == "candidate" && (m.Owner != "platform" && m.Owner != "market" && m.Owner != "artifact") {
			problems = append(problems, fmt.Sprintf("invalid migration owner at index %d", i))
		}
		if m.Checksum == strings.TrimPrefix(placeholderDigest, "sha256:") || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(m.Checksum) {
			problems = append(problems, fmt.Sprintf("migrations[%d] (%s) has no real checksum", i, m.Name))
		}
	}
	if len(l.Compatibility.OS) == 0 || len(l.Compatibility.Arch) == 0 {
		problems = append(problems, "lock compatibility must declare at least one OS and architecture")
	}
	for i, p := range l.Profiles {
		if !knownProfileName(p.Name) {
			problems = append(problems, fmt.Sprintf("profiles[%d] (%s) is not a known profile", i, p.Name))
		}
		if p.MinKubernetesVersion != "" && !tagPattern.MatchString(p.MinKubernetesVersion) {
			problems = append(problems, fmt.Sprintf("profiles[%d] (%s) has an invalid minimum Kubernetes version", i, p.Name))
		}
	}
	for i, path := range l.SupportedUpgrades {
		if strings.TrimSpace(path.From) == "" || strings.TrimSpace(path.To) == "" {
			problems = append(problems, fmt.Sprintf("supportedUpgrades[%d] requires both from and to", i))
		}
	}
	for i, step := range l.MigrationChain {
		if step.Owner != "platform" && step.Owner != "market" && step.Owner != "artifact" {
			problems = append(problems, fmt.Sprintf("migrationChain[%d] (%s) has an invalid owner", i, step.Name))
		}
		if strings.TrimSpace(step.Name) == "" || strings.TrimSpace(step.FromSchema) == "" || strings.TrimSpace(step.ToSchema) == "" {
			problems = append(problems, fmt.Sprintf("migrationChain[%d] requires name, fromSchema and toSchema", i))
		}
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(step.Checksum) {
			problems = append(problems, fmt.Sprintf("migrationChain[%d] (%s) has no real checksum", i, step.Name))
		}
	}
	return problems
}

// knownProfileName reports whether a name is one of the certified profiles.
func knownProfileName(name string) bool {
	switch Profile(name) {
	case ProfileMinimal, ProfileLiteHA, ProfileStandardHA, ProfileEnterprise:
		return true
	default:
		return false
	}
}

// ValidateCompatibility checks the declared candidate matrix, not actual node
// observations or artifact content. Those require preflight/prepare evidence.
func (l *ReleaseLock) ValidateCompatibility(c *DeploymentConfig, ev *EvidenceMatrix) []string {
	var problems []string
	if ev == nil {
		return []string{"capability evidence matrix is unavailable"}
	}
	if !containsStr(l.Compatibility.Profiles, string(c.Spec.Profile)) {
		problems = append(problems, "ReleaseLock compatibility does not include the requested profile")
	}
	if c.Spec.Cluster.Strategy == StrategyProvision && !containsStr(l.Compatibility.KubernetesVersions, c.Spec.Cluster.KubernetesVersion) {
		problems = append(problems, "ReleaseLock compatibility does not include the requested Kubernetes version")
	}
	for _, os := range l.Compatibility.OS {
		if !containsStr(ev.CertifiedOS, os) {
			problems = append(problems, "ReleaseLock declares an OS without candidate evidence: "+os)
		}
	}
	for _, arch := range l.Compatibility.Arch {
		if !containsStr(ev.CertifiedArch, arch) {
			problems = append(problems, "ReleaseLock declares an architecture without candidate evidence: "+arch)
		}
	}
	for _, version := range l.Compatibility.KubernetesVersions {
		if !ev.VersionCertified(version) {
			problems = append(problems, "ReleaseLock declares a Kubernetes version without candidate evidence: "+version)
		}
	}
	problems = append(problems, l.CheckProfileContract(c.Spec.Profile, c.Spec.Cluster.KubernetesVersion)...)
	return problems
}
