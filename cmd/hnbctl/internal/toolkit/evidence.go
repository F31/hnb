package toolkit

import (
	"regexp"
)

// EvidenceMatrix is the read-only, versioned capability evidence the installer
// consults. It is populated from the acceptance registry, not edited by clients.
type EvidenceMatrix struct {
	Version       string            `yaml:"version" json:"version"`
	CertifiedOS   []string          `yaml:"certifiedOS" json:"certifiedOS"`
	CertifiedArch []string          `yaml:"certifiedArch" json:"certifiedArch"`
	Kubernetes    []EvidenceVersion `yaml:"kubernetes" json:"kubernetes"`
	CNI           []EvidenceVersion `yaml:"cni" json:"cni"`
	Profiles      []EvidenceProfile `yaml:"profiles" json:"profiles"`
	Routes        []EvidenceProfile `yaml:"routes" json:"routes"`
	ProfilePaths  []EvidencePath    `yaml:"profilePaths,omitempty" json:"profilePaths,omitempty"`
	OfflineReady  bool              `yaml:"offlineReady" json:"offlineReady"`
	OSAdaptations []OSAdaptation    `yaml:"osAdaptations,omitempty" json:"osAdaptations,omitempty"`
}

// EvidencePath certifies one from->to profile transition. An empty
// ProfilePaths set means no upgrade path is certified. Backings, when set,
// restricts the transition to those data-plane backing kinds.
type EvidencePath struct {
	From     string   `yaml:"from" json:"from"`
	To       string   `yaml:"to" json:"to"`
	Status   string   `yaml:"status" json:"status"` // certified | blocked
	Backings []string `yaml:"backings,omitempty" json:"backings,omitempty"`
	Reasons  []string `yaml:"reasons,omitempty" json:"reasons,omitempty"`
	Report   string   `yaml:"report,omitempty" json:"report,omitempty"`
}

// UpgradePathCertified reports whether a from->to profile transition is certified.
func (m *EvidenceMatrix) UpgradePathCertified(from, to Profile) bool {
	return m.UpgradePathCertifiedFor(from, to, "")
}

// UpgradePathCertifiedFor reports whether a from->to transition is certified for
// a backing kind. An empty backing, or a path without a declared backing set,
// matches any backing.
func (m *EvidenceMatrix) UpgradePathCertifiedFor(from, to Profile, backing string) bool {
	if m == nil {
		return false
	}
	for _, p := range m.ProfilePaths {
		if p.From == string(from) && p.To == string(to) {
			if p.Status != "certified" {
				return false
			}
			return backing == "" || len(p.Backings) == 0 || containsStr(p.Backings, backing)
		}
	}
	return false
}

// OSAdaptation binds observed compatibility to a route and distribution. In
// particular, existing K3s deployment evidence cannot authorize kubeadm bootstrap.
type OSAdaptation struct {
	OS           string `yaml:"os" json:"os"`
	Arch         string `yaml:"arch" json:"arch"`
	Kernel       string `yaml:"kernel" json:"kernel"`
	Route        string `yaml:"route" json:"route"`
	Distribution string `yaml:"distribution" json:"distribution"`
	Status       string `yaml:"status" json:"status"`
	Report       string `yaml:"report" json:"report"`
}

func (m *EvidenceMatrix) ObservedOSKernelSupported(os, kernel, arch string) bool {
	if m == nil {
		return false
	}
	for _, evidence := range m.OSAdaptations {
		if evidence.OS == os && evidence.Kernel == kernel && evidence.Arch == arch &&
			(evidence.Status == "verified" || evidence.Status == "certified") {
			return true
		}
	}
	return false
}

type EvidenceVersion struct {
	Plugin  string `yaml:"plugin,omitempty" json:"plugin,omitempty"`
	Version string `yaml:"version" json:"version"`
	Status  string `yaml:"status" json:"status"` // certified | blocked
	Report  string `yaml:"report" json:"report"`
}

type EvidenceProfile struct {
	Name   string `yaml:"name" json:"name"`
	Status string `yaml:"status" json:"status"`
	// Backings restricts the certification to those data-plane backing kinds.
	// Empty means the entry certifies the profile regardless of backing.
	Backings []string `yaml:"backings,omitempty" json:"backings,omitempty"`
	Reasons  []string `yaml:"reasons" json:"reasons"`
}

func (m *EvidenceMatrix) VersionCertified(v string) bool {
	for _, k := range m.Kubernetes {
		if k.Version == v {
			return k.Status == "certified"
		}
	}
	return false
}

func (m *EvidenceMatrix) CNICertified(plugin, version string) bool {
	for _, c := range m.CNI {
		if c.Version == version && c.Plugin == plugin {
			return c.Status == "certified"
		}
	}
	return false
}

func (m *EvidenceMatrix) ProfileCertified(p Profile) bool {
	for _, pr := range m.Profiles {
		if pr.Name == string(p) {
			return pr.Status == "certified"
		}
	}
	return false
}

// ProfileCertifiedFor reports whether a profile is certified for a backing kind.
// An empty backing, or an entry without a declared backing set, matches any.
func (m *EvidenceMatrix) ProfileCertifiedFor(p Profile, backing string) bool {
	if m == nil {
		return false
	}
	for _, pr := range m.Profiles {
		if pr.Name == string(p) {
			if pr.Status != "certified" {
				return false
			}
			return backing == "" || len(pr.Backings) == 0 || containsStr(pr.Backings, backing)
		}
	}
	return false
}

// EvidenceGate applies the certified-capability gate to a config. This is
// separate from structural Validate so an operator can edit a draft, while
// install/resume refuses to act on combinations without certified evidence.
func EvidenceGate(cfg *DeploymentConfig, m *EvidenceMatrix) []string {
	if m == nil {
		return []string{"capability evidence matrix is unavailable"}
	}
	var problems []string
	if !m.ProfileCertified(cfg.Spec.Profile) {
		reason := "profile " + string(cfg.Spec.Profile) + " has no certified evidence"
		for _, pr := range m.Profiles {
			if pr.Name == string(cfg.Spec.Profile) {
				reason = "profile " + string(cfg.Spec.Profile) + " is not certified: " + joinReasons(pr.Reasons)
			}
		}
		problems = append(problems, reason)
	}
	if cfg.Spec.Cluster.Strategy == StrategyProvision {
		if !containsStr(m.CertifiedArch, "amd64") {
			problems = append(problems, "no certified architecture for provision route")
		}
	}
	certifiedRoute := false
	for _, route := range m.Routes {
		if route.Name == string(cfg.Spec.Mode) && route.Status == "certified" {
			certifiedRoute = true
		}
	}
	if !certifiedRoute {
		problems = append(problems, "route "+string(cfg.Spec.Mode)+" has no full platform delivery evidence")
	}
	if cfg.Spec.Artifacts.Mode == ArtifactsOffline && !m.OfflineReady {
		problems = append(problems, "offline delivery lacks dependency closure and no-public-network E2E evidence")
	}
	return problems
}

func joinReasons(reasons []string) string {
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Evidence for the current change: the method-level provider route and the
// existing-route Minimal full-platform delivery are certified. Bootstrap and
// LiteHA/StandardHA/Enterprise and offline remain blocked until their own
// evidence exists. Certifying Minimal applies at the platform level only; the
// bootstrap route stays blocked, so existing-route evidence is never promoted
// into a kubeadm bootstrap release.
var CurrentEvidence = &EvidenceMatrix{
	Version:       "2026-11-08",
	CertifiedOS:   []string{"ubuntu-24.04", "rocky-10.1"},
	CertifiedArch: []string{"amd64"},
	OSAdaptations: []OSAdaptation{
		{OS: "ubuntu-24.04", Arch: "amd64", Kernel: "6.8.0-31-generic", Route: "bootstrap", Distribution: "kubeadm", Status: "certified", Report: "docs/acceptance/reports/20261011-provider-ssh-ubuntu2404.md"},
		{OS: "rocky-10.1", Arch: "amd64", Kernel: "6.12.0-124.8.1.el10_1.x86_64", Route: "bootstrap", Distribution: "kubeadm", Status: "certified", Report: "docs/acceptance/reports/20261011-provider-ssh-rocky10.md"},
		{OS: "ubuntu-26.04", Arch: "amd64", Kernel: "7.0.0-30-generic", Route: "existing", Distribution: "k3s", Status: "certified", Report: "docs/acceptance/reports/20261016-existing-minimal-e2e.md"},
	},
	Kubernetes: []EvidenceVersion{
		{Version: "1.36.5", Status: "certified", Report: "docs/acceptance/reports/20261011-provider-ssh-ubuntu2404.md"},
		{Version: "1.36.4", Status: "certified", Report: "docs/acceptance/reports/20261108-s4-candidate-0.1.1-kind-install.md"},
	},
	CNI: []EvidenceVersion{
		{Plugin: "cilium", Version: "1.20.1", Status: "certified", Report: "docs/acceptance/reports/20261011-provider-ssh-ubuntu2404.md (cilium)"},
	},
	Profiles: []EvidenceProfile{
		{Name: "Minimal", Status: "certified", Backings: []string{"bundled"}, Reasons: []string{"existing-route bundled full platform delivery E2E passed; see docs/acceptance/reports/20261016-existing-minimal-e2e.md. Bootstrap route remains blocked by its own route gate."}},
		{Name: "LiteHA", Status: "blocked", Backings: []string{"in-cluster-ha", "external"}, Reasons: []string{"no HA failure-domain, PostgreSQL failover or JetStream node drills; no RPO/RTO", "in-cluster-ha needs >=3 control-plane domains plus a data-plane operator drill"}},
		{Name: "StandardHA", Status: "blocked", Backings: []string{"in-cluster-ha", "external"}, Reasons: []string{"no certified HA topology evidence"}},
		{Name: "Enterprise", Status: "blocked", Backings: []string{"in-cluster-ha", "external"}, Reasons: []string{"no capacity or recovery evidence"}},
	},
	ProfilePaths: []EvidencePath{
		{From: "Minimal", To: "LiteHA", Status: "blocked", Backings: []string{"in-cluster-ha", "external"}, Reasons: []string{"no HA failure-domain, PostgreSQL failover or JetStream node drills; no RPO/RTO"}},
		{From: "LiteHA", To: "StandardHA", Status: "blocked", Reasons: []string{"no HA topology or capacity evidence"}},
		{From: "StandardHA", To: "Enterprise", Status: "blocked", Reasons: []string{"no capacity or recovery evidence"}},
	},
	Routes: []EvidenceProfile{
		{Name: "existing", Status: "certified", Reasons: []string{"full platform Minimal delivery E2E passed on Ubuntu 26.04 / K3s 1.36.5"}},
		{Name: "bootstrap", Status: "blocked", Reasons: []string{"ownership boundary and full platform bootstrap E2E not yet verified"}},
	},
}

var profilePattern = regexp.MustCompile(`^Minimal|LiteHA|StandardHA|Enterprise$`)
