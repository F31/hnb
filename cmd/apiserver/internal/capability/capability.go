// Package capability implements the staged fail-closed deployment gates for
// the cluster-management console (KERNEL-016, RT-010, UX-021/022/023).
//
// Each stage corresponds to an independently deployable capability. A missing
// or incompatible capability removes the navigation entry, hides the PageSchema,
// and rejects direct route actions. Build-time feature flags are deployment
// overrides only and can never enable a server-disabled capability.
package capability

import (
	"sort"
	"strings"
)

// Stage identifiers (deployment phase order, independent gates).
const (
	// Contract means the OpenAPI/JSON Schema/generated types for the cluster
	// module are deployed and pinned.
	Contract = "cluster.contract"
	// Schema means the L2 PageSchema / dictionaries are published to the
	// console cohort.
	Schema = "cluster.schema"
	// Provider means a conformance lifecycle Provider HTTP v2 is deployed.
	Provider = "cluster.provider"
	// Projector means the observation projector / read model is running and
	// cutover has passed.
	Projector = "cluster.projector"
	// Read means read-only BFF routes (list/detail/nodes/dictionary) are gated
	// open.
	Read = "cluster.read"
	// Write means write routes (RuntimeIntent submission + Operation actions)
	// are gated open.
	Write = "cluster.write"
	// StorageSupply publishes the Resource storage supply view. Container
	// storage consumption remains independently available.
	StorageSupply = "storage.supply"
)

// StageOrder defines the fail-closed dependency order.
var StageOrder = []string{Contract, Schema, Provider, Projector, Read, Write, StorageSupply}

// Set is an immutable snapshot of enabled capabilities.
type Set struct {
	enabled map[string]bool
}

// AllStages returns a Set with every stage enabled.
func AllStages() Set {
	enabled := make(map[string]bool, len(StageOrder))
	for _, stage := range StageOrder {
		enabled[stage] = true
	}
	return Set{enabled: enabled}
}

// FromCSV parses a comma-separated list of enabled stages. Unknown names are
// ignored (never implicitly enabled). Empty input means "all stages enabled",
// which preserves the current default behavior for existing deployments.
func FromCSV(csv string) Set {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return AllStages()
	}
	enabled := make(map[string]bool, len(StageOrder))
	for _, part := range strings.Split(csv, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if isKnown(name) {
			enabled[name] = true
		}
	}
	return Set{enabled: enabled}
}

// Has reports whether the given stage is enabled. Unknown names are never
// enabled (fail closed).
func (s Set) Has(name string) bool {
	if !isKnown(name) {
		return false
	}
	return s.enabled[name]
}

// EnabledStages returns the sorted list of enabled stage names.
func (s Set) EnabledStages() []string {
	var names []string
	for _, stage := range StageOrder {
		if s.enabled[stage] {
			names = append(names, stage)
		}
	}
	sort.Strings(names)
	return names
}

// Snapshot returns a map suitable for the navigation capability filter.
func (s Set) Snapshot() map[string]bool {
	out := make(map[string]bool, len(s.enabled))
	for name, ok := range s.enabled {
		out[name] = ok
	}
	return out
}

func isKnown(name string) bool {
	for _, stage := range StageOrder {
		if stage == name {
			return true
		}
	}
	return false
}
