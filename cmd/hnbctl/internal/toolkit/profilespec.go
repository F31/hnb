package toolkit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ProfileLevel orders the four deployment tiers. A strictly higher level is a
// capability superset of every lower level; the ordering is what makes
// "minimal -> lite -> standard -> pro" a legal, monotonic upgrade path.
type ProfileLevel int

const (
	LevelMinimal ProfileLevel = iota + 1
	LevelLite
	LevelStandard
	LevelPro
)

// ProfileLevels is the ordered tier ladder used by monotonic validation.
var ProfileLevels = []ProfileLevel{LevelMinimal, LevelLite, LevelStandard, LevelPro}

func (l ProfileLevel) String() string {
	switch l {
	case LevelMinimal:
		return "minimal"
	case LevelLite:
		return "lite"
	case LevelStandard:
		return "standard"
	case LevelPro:
		return "pro"
	default:
		return "unknown"
	}
}

// Backing ranks the data-plane backing kind. External backings are an HA
// upgrade of bundled single-instance backings, never a downgrade.
const (
	BackingBundled  = "bundled"
	BackingExternal = "external"
)

// Profile capability flags (feature-level, not the HA/resource dimensions which
// have their own fields). The four current profiles share the T0/T1 set; a
// larger profile does not implicitly enable optional Packs.
const (
	CapCore            = "core"
	CapPortal          = "portal"
	CapMarket          = "market"
	CapTunnel          = "tunnel"
	CapRouting         = "routing"
	CapExtension       = "extension"
	CapPlugins         = "plugins"
	CapRuntime         = "runtime"
	CapGateway         = "gateway"
	CapKubernetes      = "kubernetes"
	CapRBAC            = "rbac"
	CapAlerts          = "alerts"
	CapAgent           = "agent"
	CapNetworkRegistry = "network-registry"
	CapCNI             = "cni"
)

// HAProfile is the failure-domain contract of a shape.
type HAProfile struct {
	IndependentDomains int `json:"independentDomains"`
	Quorum             int `json:"quorum"`
}

// StorageProfile is the managed persistent footprint of one component.
type StorageProfile struct {
	SizeGi    int    `json:"sizeGi"`
	ClassName string `json:"className,omitempty"`
}

// ResourceProfile is one component's starting CPU/memory requests.
type ResourceProfile struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// ProfileSpec is the comparable capability vector of a deployment shape. It is
// derived from deploy/charts/hnb/overlays (guarded by a parity test) so the
// Chart stays the single source of truth. It carries no image identities and no
// credentials.
type ProfileSpec struct {
	Name         Profile                    `json:"name"`
	Level        ProfileLevel               `json:"level"`
	Capabilities []string                   `json:"capabilities"`
	Components   map[string]int             `json:"components"`
	HA           HAProfile                  `json:"ha"`
	Backings     string                     `json:"backings"`
	Storage      map[string]StorageProfile  `json:"storage"`
	Resources    map[string]ResourceProfile `json:"resources"`
	ResourceTier string                     `json:"resourceTier"`
}

// backingRank orders backing kinds for superset comparison.
func backingRank(kind string) int {
	if kind == BackingExternal {
		return 1
	}
	return 0
}

func cloneInts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStorage(in map[string]StorageProfile) map[string]StorageProfile {
	out := make(map[string]StorageProfile, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneResources(in map[string]ResourceProfile) map[string]ResourceProfile {
	out := make(map[string]ResourceProfile, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// minimalShape is the base vector every other shape extends, matching
// deploy/charts/hnb/overlays/minimal.yaml.
func minimalShape() ProfileSpec {
	return ProfileSpec{
		Name:  ProfileMinimal,
		Level: LevelMinimal,
		Capabilities: []string{
			CapAgent, CapAlerts, CapCNI, CapCore, CapExtension, CapGateway,
			CapKubernetes, CapMarket, CapNetworkRegistry, CapPlugins, CapPortal,
			CapRBAC, CapRouting, CapRuntime, CapTunnel,
		},
		Components: map[string]int{
			"alert-manager": 1, "apiserver": 1, "app-market": 1, "cilium-provider": 1,
			"cluster-agent": 1, "cluster-router": 1, "extension-controller": 1,
			"gateway-provider": 1, "kubernetes-provider": 1, "network-registry": 1,
			"operation-worker": 1, "platform-api": 1, "plugin-worker": 1, "portal": 1,
			"rbac-syncer": 1, "runtime-target-lifecycle-provider": 1, "tunnel-server": 1,
		},
		HA:           HAProfile{IndependentDomains: 1, Quorum: 1},
		Backings:     BackingBundled,
		Storage:      map[string]StorageProfile{"postgresql": {SizeGi: 2}, "nats": {SizeGi: 1}},
		ResourceTier: "small",
		Resources: map[string]ResourceProfile{
			"apiserver": {CPU: "100m", Memory: "192Mi"}, "operation-worker": {CPU: "100m", Memory: "192Mi"},
			"platform-api": {CPU: "100m", Memory: "192Mi"}, "tunnel-server": {CPU: "75m", Memory: "96Mi"},
			"cluster-router": {CPU: "75m", Memory: "96Mi"}, "extension-controller": {CPU: "75m", Memory: "96Mi"},
			"app-market": {CPU: "75m", Memory: "96Mi"}, "plugin-worker": {CPU: "75m", Memory: "96Mi"},
			"runtime-target-lifecycle-provider": {CPU: "75m", Memory: "96Mi"}, "gateway-provider": {CPU: "75m", Memory: "96Mi"},
			"kubernetes-provider": {CPU: "75m", Memory: "96Mi"}, "rbac-syncer": {CPU: "25m", Memory: "48Mi"},
			"alert-manager": {CPU: "25m", Memory: "48Mi"}, "cluster-agent": {CPU: "25m", Memory: "48Mi"},
			"network-registry": {CPU: "25m", Memory: "48Mi"}, "cilium-provider": {CPU: "25m", Memory: "48Mi"},
			"portal": {CPU: "25m", Memory: "64Mi"},
		},
	}
}

// ProfileShapeData returns the capability vector of every known shape keyed by
// profile. The parity test guards it against the Chart overlays.
func ProfileShapeData() map[Profile]ProfileSpec {
	minimal := minimalShape()

	// LiteHA: merge minimal.yaml + lite-ha.yaml (external backings, 3 domains).
	lite := minimal
	lite.Name, lite.Level = ProfileLiteHA, LevelLite
	lite.Components = cloneInts(minimal.Components)
	for _, c := range []string{"apiserver", "cluster-router", "operation-worker", "platform-api", "portal", "tunnel-server"} {
		lite.Components[c] = 3
	}
	lite.HA = HAProfile{IndependentDomains: 3, Quorum: 2}
	lite.Backings = BackingExternal
	lite.Storage = map[string]StorageProfile{}

	// StandardHA: merge + standard-ha.yaml (larger cores, plugin-worker replicated).
	standard := lite
	standard.Name, standard.Level = ProfileStandardHA, LevelStandard
	standard.Components = cloneInts(lite.Components)
	standard.Components["plugin-worker"] = 3
	standard.ResourceTier = "standard"
	standard.Resources = cloneResources(minimal.Resources)
	for _, c := range []string{"apiserver", "cluster-router", "operation-worker", "platform-api", "tunnel-server"} {
		standard.Resources[c] = ResourceProfile{CPU: "1", Memory: "1Gi"}
	}
	standard.Resources["plugin-worker"] = ResourceProfile{CPU: "500m", Memory: "512Mi"}

	// Enterprise: merge + enterprise.yaml (largest cores).
	pro := standard
	pro.Name, pro.Level = ProfileEnterprise, LevelPro
	pro.Components = cloneInts(standard.Components)
	for _, c := range []string{"apiserver", "operation-worker", "platform-api"} {
		pro.Components[c] = 5
	}
	pro.Components["plugin-worker"] = 5
	pro.ResourceTier = "large"
	pro.Resources = cloneResources(standard.Resources)
	for _, c := range []string{"apiserver", "operation-worker", "platform-api", "plugin-worker"} {
		pro.Resources[c] = ResourceProfile{CPU: "2", Memory: "2Gi"}
	}

	return map[Profile]ProfileSpec{
		ProfileMinimal:    minimal,
		ProfileLiteHA:     lite,
		ProfileStandardHA: standard,
		ProfileEnterprise: pro,
	}
}

// ProfileShape returns the capability vector for a profile.
func ProfileShape(p Profile) (ProfileSpec, bool) {
	spec, ok := ProfileShapeData()[p]
	return spec, ok
}

// Digest returns a stable content identity over the canonical vector, for use
// in upgrade plans and run records.
func (s ProfileSpec) Digest() string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
}

// SupersetOf reports the reasons (empty means satisfied) a target spec is NOT a
// valid superset of a source spec. It enforces capability inclusion, component
// replica monotonicity, HA-domain/quorum monotonicity, backing-kind monotonicity
// and storage non-shrink.
func (s ProfileSpec) SupersetOf(source ProfileSpec) []string {
	var problems []string
	have := map[string]bool{}
	for _, c := range s.Capabilities {
		have[c] = true
	}
	for _, c := range source.Capabilities {
		if !have[c] {
			problems = append(problems, "PROFILE_NOT_SUPERSET: capability "+c+" is missing")
		}
	}
	for component, want := range source.Components {
		got, ok := s.Components[component]
		if !ok {
			problems = append(problems, "PROFILE_NOT_SUPERSET: component "+component+" is missing")
			continue
		}
		if got < want {
			problems = append(problems, fmt.Sprintf("PROFILE_NOT_SUPERSET: component %s replicas %d < %d", component, got, want))
		}
	}
	if s.HA.IndependentDomains < source.HA.IndependentDomains {
		problems = append(problems, "PROFILE_NOT_SUPERSET: independent domains decreased")
	}
	if s.HA.Quorum < source.HA.Quorum {
		problems = append(problems, "PROFILE_NOT_SUPERSET: quorum decreased")
	}
	if backingRank(s.Backings) < backingRank(source.Backings) {
		problems = append(problems, "PROFILE_NOT_SUPERSET: backing kind regressed")
	}
	for component, want := range source.Storage {
		got, ok := s.Storage[component]
		if !ok {
			// Externalising a bundled claim is an upgrade, not a shrink.
			if s.Backings != BackingExternal {
				problems = append(problems, "PROFILE_NOT_SUPERSET: storage "+component+" is missing")
			}
			continue
		}
		if got.SizeGi < want.SizeGi {
			problems = append(problems, fmt.Sprintf("PROFILE_NOT_SUPERSET: storage %s %dGi < %dGi", component, got.SizeGi, want.SizeGi))
		}
	}
	for component, want := range source.Resources {
		got, ok := s.Resources[component]
		if !ok {
			continue
		}
		if parseCPU(got.CPU) < parseCPU(want.CPU) || parseMemory(got.Memory) < parseMemory(want.Memory) {
			problems = append(problems, "PROFILE_NOT_SUPERSET: resources for "+component+" decreased")
		}
	}
	return problems
}

// parseCPU normalises a CPU quantity to millicores. Unknown forms parse to 0.
func parseCPU(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.HasSuffix(value, "m") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "m"))
		if err != nil {
			return 0
		}
		return n
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return int(n * 1000)
}

// parseMemory normalises a memory quantity to MiB. Unknown forms parse to 0.
func parseMemory(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	switch {
	case strings.HasSuffix(value, "Gi"):
		n, err := strconv.Atoi(strings.TrimSuffix(value, "Gi"))
		if err != nil {
			return 0
		}
		return n * 1024
	case strings.HasSuffix(value, "Mi"):
		n, err := strconv.Atoi(strings.TrimSuffix(value, "Mi"))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// SortedComponents returns the enabled components in a stable order (useful for
// plans and human output).
func (s ProfileSpec) SortedComponents() []string {
	out := make([]string, 0, len(s.Components))
	for name := range s.Components {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
