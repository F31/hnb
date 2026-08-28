package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ConformanceLevel string

const (
	ConformanceNone            ConformanceLevel = "none"
	ConformanceBasic           ConformanceLevel = "basic"
	ConformanceProductionReady ConformanceLevel = "production_ready"
)

type ProviderManifest struct {
	ProviderID           string                `json:"provider_id"`
	Name                 string                `json:"name"`
	Version              string                `json:"version"`
	ProtocolVersion      string                `json:"protocol_version"`
	Capabilities         []string              `json:"capabilities"`
	Actions              []string              `json:"actions"`
	Permissions          map[string]any        `json:"permissions"`
	ResourceRequirements map[string]any        `json:"resource_requirements"`
	Dependencies         []ProviderDependency  `json:"dependencies"`
	Compatibility        map[string]any        `json:"compatibility"`
	ConformanceLevel     ConformanceLevel      `json:"conformance_level"`
	ConformanceEvidence  []ConformanceEvidence `json:"conformance_evidence"`
	ConformanceExpiresAt *time.Time            `json:"conformance_expires_at,omitempty"`
	StorageDriverPackage *StorageDriverPackage `json:"storage_driver_package,omitempty"`
}

type ProviderDependency struct {
	ProviderID string `json:"provider_id"`
	Version    string `json:"version"`
	Required   bool   `json:"required"`
}

type ConformanceEvidence struct {
	TestName    string `json:"test_name"`
	Category    string `json:"category"`
	Passed      bool   `json:"passed"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

type StorageDriverPackage struct {
	SchemaVersion       string                       `json:"schemaVersion"`
	PackageID           string                       `json:"packageId"`
	PackageVersion      string                       `json:"packageVersion"`
	Provisioners        []string                     `json:"provisioners"`
	Compatibility       StorageDriverCompatibility   `json:"compatibility"`
	Capabilities        StorageDriverCapabilities    `json:"capabilities"`
	RequiredComponents  StorageRequiredComponents    `json:"requiredComponents"`
	PackageDigest       string                       `json:"packageDigest"`
	Signature           PackageSignature             `json:"signature"`
	ConformanceEvidence []StorageConformanceEvidence `json:"conformanceEvidence"`
}

type StorageDriverCompatibility struct {
	KubernetesVersions  []VersionRange `json:"kubernetesVersions"`
	UpgradeFromVersions []string       `json:"upgradeFromVersions"`
	RollbackToVersions  []string       `json:"rollbackToVersions"`
}

type VersionRange struct {
	MinInclusive string `json:"minInclusive"`
	MaxExclusive string `json:"maxExclusive"`
}

type StorageDriverCapabilities struct {
	VolumeModes      []string `json:"volumeModes"`
	AccessModes      []string `json:"accessModes"`
	Topology         string   `json:"topology"`
	CapacityTracking []string `json:"capacityTracking"`
	Expansion        string   `json:"expansion"`
	Clone            string   `json:"clone"`
	Snapshot         string   `json:"snapshot"`
	Ephemeral        string   `json:"ephemeral"`
	Health           string   `json:"health"`
}

type StorageRequiredComponents struct {
	CRDs        []ComponentRequirement `json:"crds"`
	Controllers []ComponentRequirement `json:"controllers"`
}

type ComponentRequirement struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type PackageSignature struct {
	Format       string `json:"format"`
	KeyID        string `json:"keyId"`
	SignedDigest string `json:"signedDigest"`
	EvidenceRef  string `json:"evidenceRef"`
}

type StorageConformanceEvidence struct {
	PackageVersion    string    `json:"packageVersion"`
	KubernetesVersion string    `json:"kubernetesVersion"`
	SuiteVersion      string    `json:"suiteVersion"`
	PassedAt          time.Time `json:"passedAt"`
	ExpiresAt         time.Time `json:"expiresAt"`
	EvidenceRef       string    `json:"evidenceRef"`
	EvidenceDigest    string    `json:"evidenceDigest"`
}

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func (m *ProviderManifest) Validate() error {
	if m.ProviderID == "" {
		return fmt.Errorf("provider_id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if !semverPattern.MatchString(m.Version) {
		return fmt.Errorf("version must be semver (e.g. 1.0.0)")
	}
	if m.ProtocolVersion == "" {
		return fmt.Errorf("protocol_version is required")
	}
	if len(m.Actions) == 0 {
		return fmt.Errorf("at least one action is required")
	}
	for _, action := range m.Actions {
		if !isValidAction(action) {
			return fmt.Errorf("unsupported action: %s", action)
		}
	}
	if m.ConformanceLevel != "" {
		switch m.ConformanceLevel {
		case ConformanceNone, ConformanceBasic, ConformanceProductionReady:
		default:
			return fmt.Errorf("invalid conformance_level: %s", m.ConformanceLevel)
		}
	}
	if m.StorageDriverPackage != nil {
		if err := m.StorageDriverPackage.Validate(m.Version, time.Now()); err != nil {
			return fmt.Errorf("storage_driver_package: %w", err)
		}
	}
	return nil
}

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var provisionerPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func (p *StorageDriverPackage) Validate(manifestVersion string, now time.Time) error {
	if p.SchemaVersion != "1.0.0" {
		return fmt.Errorf("schemaVersion must be 1.0.0")
	}
	if p.PackageID == "" {
		return fmt.Errorf("packageId is required")
	}
	if !semverPattern.MatchString(p.PackageVersion) || p.PackageVersion != manifestVersion {
		return fmt.Errorf("packageVersion must match manifest version %q", manifestVersion)
	}
	if err := validateUniqueValues("provisioners", p.Provisioners, nil, provisionerPattern); err != nil {
		return err
	}
	if len(p.Compatibility.KubernetesVersions) == 0 {
		return fmt.Errorf("at least one supported Kubernetes version range is required")
	}
	for _, versionRange := range p.Compatibility.KubernetesVersions {
		if !semverPattern.MatchString(versionRange.MinInclusive) || !semverPattern.MatchString(versionRange.MaxExclusive) || compareSemver(versionRange.MinInclusive, versionRange.MaxExclusive) >= 0 {
			return fmt.Errorf("invalid Kubernetes version range %q to %q", versionRange.MinInclusive, versionRange.MaxExclusive)
		}
	}
	for name, versions := range map[string][]string{
		"upgradeFromVersions": p.Compatibility.UpgradeFromVersions,
		"rollbackToVersions":  p.Compatibility.RollbackToVersions,
	} {
		if len(versions) > 0 {
			if err := validateUniqueValues(name, versions, nil, semverPattern); err != nil {
				return err
			}
		}
	}
	if err := validateUniqueValues("volumeModes", p.Capabilities.VolumeModes, map[string]bool{"Block": true, "File": true}, nil); err != nil {
		return err
	}
	if err := validateUniqueValues("accessModes", p.Capabilities.AccessModes, map[string]bool{"ReadWriteOnce": true, "ReadOnlyMany": true, "ReadWriteMany": true, "ReadWriteOncePod": true}, nil); err != nil {
		return err
	}
	if err := validateUniqueValues("capacityTracking", p.Capabilities.CapacityTracking, map[string]bool{"CSIStorageCapacity": true, "Provider": true, "NodeInventory": true, "None": true}, nil); err != nil {
		return err
	}
	for name, claim := range map[string]string{
		"topology": p.Capabilities.Topology, "expansion": p.Capabilities.Expansion,
		"clone": p.Capabilities.Clone, "snapshot": p.Capabilities.Snapshot,
		"ephemeral": p.Capabilities.Ephemeral, "health": p.Capabilities.Health,
	} {
		if claim != "Supported" && claim != "Unsupported" {
			return fmt.Errorf("%s must be Supported or Unsupported", name)
		}
	}
	if contains(p.Capabilities.CapacityTracking, "None") && len(p.Capabilities.CapacityTracking) != 1 {
		return fmt.Errorf("capacityTracking None cannot be combined with another source")
	}
	if err := validateComponents("requiredComponents.crds", p.RequiredComponents.CRDs); err != nil {
		return err
	}
	if err := validateComponents("requiredComponents.controllers", p.RequiredComponents.Controllers); err != nil {
		return err
	}
	if !digestPattern.MatchString(p.PackageDigest) {
		return fmt.Errorf("packageDigest must be a sha256 digest")
	}
	if p.Signature.Format != "Cosign" && p.Signature.Format != "Notation" && p.Signature.Format != "X509" {
		return fmt.Errorf("unsupported signature format %q", p.Signature.Format)
	}
	if p.Signature.KeyID == "" || p.Signature.EvidenceRef == "" || p.Signature.SignedDigest != p.PackageDigest {
		return fmt.Errorf("signature must identify its key and evidence and bind packageDigest")
	}
	if len(p.ConformanceEvidence) == 0 {
		return fmt.Errorf("version-bound conformanceEvidence is required")
	}
	for _, evidence := range p.ConformanceEvidence {
		if evidence.PackageVersion != p.PackageVersion {
			return fmt.Errorf("conformance evidence packageVersion %q does not match packageVersion", evidence.PackageVersion)
		}
		if !semverPattern.MatchString(evidence.SuiteVersion) || !semverPattern.MatchString(evidence.KubernetesVersion) || !supportsKubernetesVersion(p.Compatibility.KubernetesVersions, evidence.KubernetesVersion) {
			return fmt.Errorf("conformance evidence is not bound to a supported Kubernetes and suite version")
		}
		if evidence.PassedAt.IsZero() || evidence.PassedAt.After(now) || !evidence.ExpiresAt.After(evidence.PassedAt) || !evidence.ExpiresAt.After(now) {
			return fmt.Errorf("conformance evidence is expired or has an invalid validity interval")
		}
		if evidence.EvidenceRef == "" || !digestPattern.MatchString(evidence.EvidenceDigest) {
			return fmt.Errorf("conformance evidence reference and digest are required")
		}
	}
	return nil
}

func validateUniqueValues(name string, values []string, allowed map[string]bool, pattern *regexp.Regexp) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || (allowed != nil && !allowed[value]) || (pattern != nil && !pattern.MatchString(value)) {
			return fmt.Errorf("invalid %s value %q", name, value)
		}
		if seen[value] {
			return fmt.Errorf("duplicate %s value %q", name, value)
		}
		seen[value] = true
	}
	return nil
}

func validateComponents(name string, components []ComponentRequirement) error {
	seen := make(map[string]bool, len(components))
	for _, component := range components {
		if component.Name == "" || seen[component.Name] {
			return fmt.Errorf("%s contains an empty or duplicate name %q", name, component.Name)
		}
		seen[component.Name] = true
		if err := validateUniqueValues(name+".versions", component.Versions, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func supportsKubernetesVersion(ranges []VersionRange, version string) bool {
	for _, versionRange := range ranges {
		if compareSemver(version, versionRange.MinInclusive) >= 0 && compareSemver(version, versionRange.MaxExclusive) < 0 {
			return true
		}
	}
	return false
}

func compareSemver(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for i := range leftParts {
		leftValue, _ := strconv.Atoi(leftParts[i])
		rightValue, _ := strconv.Atoi(rightParts[i])
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var validActions = map[string]bool{
	"validate": true, "plan": true, "provision": true, "observe": true,
	"update": true, "scale": true, "backup": true, "restore": true, "delete": true,
}

func isValidAction(action string) bool {
	return validActions[action]
}

func (m *ProviderManifest) SupportsAction(action string) bool {
	for _, a := range m.Actions {
		if a == action {
			return true
		}
	}
	return false
}

func (m *ProviderManifest) IsConformanceValid() bool {
	if m.ConformanceLevel == ConformanceNone {
		return true
	}
	if m.ConformanceExpiresAt == nil {
		return true
	}
	return time.Now().Before(*m.ConformanceExpiresAt)
}

func (m *ProviderManifest) ConformanceSummary() string {
	level := string(m.ConformanceLevel)
	if m.ConformanceLevel == ConformanceNone {
		return "not certified"
	}
	if !m.IsConformanceValid() {
		return level + " (expired)"
	}
	return level
}
