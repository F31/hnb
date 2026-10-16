package toolkit

import (
	"fmt"
	"sort"
)

// ScalePlanSchema is the schema id of a computed scale plan.
const ScalePlanSchema = "scale-plan/v1"

// ScaleInput describes a requested scaling operation against a shape.
type ScaleInput struct {
	Shape     ProfileSpec
	Replicas  map[string]int             // component -> desired replicas
	StorageGi map[string]int             // component -> desired PVC Gi
	Resources map[string]ResourceProfile // component -> desired requests
	// Observed overrides the shape baseline with the live replica count so a
	// scale is planned relative to reality, not the profile default.
	Observed          map[string]int
	ObservedStorage   map[string]int             // component -> live PVC Gi
	ObservedResources map[string]ResourceProfile // component -> live requests
	InFlightUpgrade   bool
	// ControlPlaneDomains is the observed schedulable control-plane domain count
	// (0 = not observed). OperatorReady is the observed data-plane operator
	// readiness (nil = not observed); both gate in-cluster-ha scaling.
	ControlPlaneDomains int
	OperatorReady       *bool
}

// ScalePlan is the side-effect-free result of planning a scale operation.
type ScalePlan struct {
	SchemaVersion string        `json:"schemaVersion"`
	Steps         []UpgradeStep `json:"steps"`
	Blocked       []string      `json:"blocked"`
	Authorized    bool          `json:"authorized"`
}

// haCriticalComponents are the components that must keep quorum in an HA shape.
var haCriticalComponents = map[string]bool{"apiserver": true, "operation-worker": true, "platform-api": true}

// dataPlaneComponents maps a replicated in-cluster backing component to the
// field it scales: PostgreSQL scales the CloudNativePG Cluster instance count,
// JetStream the StatefulSet replica count. Only valid with in-cluster-ha.
var dataPlaneComponents = map[string]string{
	"postgresql": "CloudNativePG Cluster.spec.instances",
	"nats":       "StatefulSet.spec.replicas",
}

// inClusterDataPlaneReplicas is the minimum (and default) quorum size of the
// in-cluster data plane.
const inClusterDataPlaneReplicas = 3

// maxReplicasPerDomain caps how many replicas of one component may run per
// independent failure domain (co-location guardrail).
const maxReplicasPerDomain = 5

// PlanScale validates a scale request against the shape's guardrails and returns
// a reviewable plan. It never mutates anything.
func PlanScale(in ScaleInput) (ScalePlan, error) {
	plan := ScalePlan{SchemaVersion: ScalePlanSchema}
	if in.InFlightUpgrade {
		plan.Blocked = append(plan.Blocked, "SCALE_UPGRADE_IN_FLIGHT: a migration or upgrade run is not terminal")
	}
	plan.Blocked = append(plan.Blocked, dataPlanePreconditions(in.Shape.Backings, in.ControlPlaneDomains, in.OperatorReady)...)

	names := make([]string, 0, len(in.Replicas))
	for name := range in.Replicas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want := in.Replicas[name]
		if want < 0 {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_INVALID: %s replicas must not be negative", name))
			continue
		}
		current, known := in.Shape.Components[name]
		if observed, ok := in.Observed[name]; ok {
			current, known = observed, true
		}
		if field, isDataPlane := dataPlaneComponents[name]; isDataPlane && in.Shape.Backings == BackingInClusterHA {
			if !known {
				current, known = inClusterDataPlaneReplicas, true
			}
			if current == want {
				continue
			}
			if want < inClusterDataPlaneReplicas {
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_BELOW_QUORUM: data plane %s replicas %d < %d", name, want, inClusterDataPlaneReplicas))
				continue
			}
			if name == "nats" && want%2 == 0 {
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_EVEN_QUORUM: JetStream requires an odd replica count, got %d", want))
				continue
			}
			plan.Steps = append(plan.Steps, UpgradeStep{Kind: "components", Target: name, Detail: "scale data-plane replicas",
				From: fmt.Sprintf("%d", current), To: fmt.Sprintf("%d", want), Reversible: false,
				Requires: []string{field, "quorum maintained"}})
			continue
		}
		if !known && want > 0 {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_UNKNOWN_COMPONENT: %s is not part of profile %s", name, in.Shape.Name))
			continue
		}
		if current == want {
			continue
		}
		if haCriticalComponents[name] && in.Shape.HA.IndependentDomains >= 3 && want > 0 && want < in.Shape.HA.Quorum {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_BELOW_QUORUM: %s replicas %d < quorum %d", name, want, in.Shape.HA.Quorum))
			continue
		}
		if in.Shape.HA.IndependentDomains > 0 && want > in.Shape.HA.IndependentDomains*maxReplicasPerDomain {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_COLOCATION_LIMIT: %s replicas %d exceed %d per domain", name, want, maxReplicasPerDomain))
			continue
		}
		plan.Steps = append(plan.Steps, UpgradeStep{Kind: "components", Target: name, Detail: "scale replicas",
			From: fmt.Sprintf("%d", current), To: fmt.Sprintf("%d", want), Reversible: true})
	}

	storageNames := make([]string, 0, len(in.StorageGi))
	for name := range in.StorageGi {
		storageNames = append(storageNames, name)
	}
	sort.Strings(storageNames)
	for _, name := range storageNames {
		want := in.StorageGi[name]
		current, known := in.Shape.Storage[name]
		if observed, ok := in.ObservedStorage[name]; ok {
			current, known = StorageProfile{SizeGi: observed}, true
		}
		if !known {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_UNKNOWN_STORAGE: %s has no managed claim in profile %s", name, in.Shape.Name))
			continue
		}
		if want < current.SizeGi {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("STORAGE_SHRINK_REFUSED: %s %dGi < %dGi", name, want, current.SizeGi))
			continue
		}
		if want == current.SizeGi {
			continue
		}
		plan.Steps = append(plan.Steps, UpgradeStep{Kind: "storage", Target: name, Detail: "expand managed claim",
			From: fmt.Sprintf("%dGi", current.SizeGi), To: fmt.Sprintf("%dGi", want), Reversible: false,
			Requires: []string{"storage class allows volume expansion"}})
	}

	resNames := make([]string, 0, len(in.Resources))
	for name := range in.Resources {
		resNames = append(resNames, name)
	}
	sort.Strings(resNames)
	for _, name := range resNames {
		want := in.Resources[name]
		if want.CPU == "" && want.Memory == "" {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_INVALID_RESOURCE: %s requires cpu or memory", name))
			continue
		}
		shapeRes := in.Shape.Resources[name]
		current, known := shapeRes, false
		if _, ok := in.Shape.Resources[name]; ok {
			known = true
		}
		if observed, ok := in.ObservedResources[name]; ok {
			current, known = observed, true
			if current.CPU == "" {
				current.CPU = shapeRes.CPU
			}
			if current.Memory == "" {
				current.Memory = shapeRes.Memory
			}
		}
		if !known {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("SCALE_UNKNOWN_COMPONENT: %s has no managed resources in profile %s", name, in.Shape.Name))
			continue
		}
		if want.CPU == "" {
			want.CPU = current.CPU
		}
		if want.Memory == "" {
			want.Memory = current.Memory
		}
		if want.CPU == current.CPU && want.Memory == current.Memory {
			continue
		}
		plan.Steps = append(plan.Steps, UpgradeStep{Kind: "resources", Target: name, Detail: "reconcile requests",
			From: current.CPU + "," + current.Memory, To: want.CPU + "," + want.Memory, Reversible: true,
			Requires: []string{"rolling restart"}})
	}

	plan.Authorized = len(plan.Blocked) == 0 && len(plan.Steps) > 0
	return plan, nil
}
