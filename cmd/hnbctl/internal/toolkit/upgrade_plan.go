package toolkit

import (
	"fmt"
	"sort"
)

// UpgradePlanSchema is the schema id of a computed upgrade plan.
const ProfileUpgradePlanSchema = "profile-upgrade-plan/v1"

// UpgradeStep is one reviewable action in an upgrade plan.
type UpgradeStep struct {
	Kind       string   `json:"kind"` // capabilities|components|storage|backings|migration|release|verify
	Target     string   `json:"target"`
	Detail     string   `json:"detail"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Reversible bool     `json:"reversible"`
	Requires   []string `json:"requires,omitempty"`
}

// UpgradePlan is the side-effect-free result of planning a profile transition.
type UpgradePlan struct {
	SchemaVersion string        `json:"schemaVersion"`
	From          string        `json:"from"`
	To            string        `json:"to"`
	FromDigest    string        `json:"fromDigest"`
	ToDigest      string        `json:"toDigest"`
	Steps         []UpgradeStep `json:"steps"`
	Blocked       []string      `json:"blocked"`
	Authorized    bool          `json:"authorized"`
}

// UpgradeInput captures everything the planner reads. It never mutates state.
type UpgradeInput struct {
	From              Profile
	To                Profile
	AllowDowngrade    bool
	Lock              *ReleaseLock
	Evidence          *EvidenceMatrix
	KubernetesVersion string
	// ControlPlaneDomains is the observed schedulable control-plane domain count.
	// 0 means "not observed" and skips the in-cluster-ha minimum check.
	ControlPlaneDomains int
	// OperatorReady is the observed data-plane operator readiness. nil means
	// "not observed" and skips the check.
	OperatorReady *bool
	// BackingOverride replaces the target profile's default backing kind
	// (bundled|in-cluster-ha|external). Empty keeps the profile default.
	BackingOverride string
}

// dataPlanePreconditions returns the prerequisite problems for an in-cluster-ha
// backing that must be satisfied before planning executes.
func dataPlanePreconditions(backings string, controlPlaneDomains int, operatorReady *bool) []string {
	if backings != BackingInClusterHA {
		return nil
	}
	var problems []string
	if controlPlaneDomains > 0 && controlPlaneDomains < 3 {
		problems = append(problems, "HA_CONTROL_NODES_INSUFFICIENT: in-cluster-ha requires at least 3 schedulable control-plane domains")
	}
	if operatorReady != nil && !*operatorReady {
		problems = append(problems, "OPERATOR_NOT_INSTALLED: the data-plane operator is not ready")
	}
	return problems
}

// PlanUpgrade computes the diff between the current and target profile vectors,
// enforces monotonicity/downgrade rules and the lock's declared upgrade path,
// and reports whether the transition is authorised. It has no side effects.
func PlanUpgrade(in UpgradeInput) (UpgradePlan, error) {
	from, okFrom := ProfileShape(in.From)
	to, okTo := ProfileShape(in.To)
	if !okFrom {
		return UpgradePlan{}, fmt.Errorf("unknown source profile %q", in.From)
	}
	if !okTo {
		return UpgradePlan{}, fmt.Errorf("unknown target profile %q", in.To)
	}
	if in.BackingOverride != "" {
		if !knownBacking(in.BackingOverride) {
			return UpgradePlan{}, fmt.Errorf("unknown backing override %q", in.BackingOverride)
		}
		to.Backings = in.BackingOverride
		if to.Backings == BackingExternal {
			to.Storage = nil
		}
	}
	plan := UpgradePlan{
		SchemaVersion: ProfileUpgradePlanSchema, From: string(in.From), To: string(in.To),
		FromDigest: from.Digest(), ToDigest: to.Digest(),
	}

	downgrade := to.Level < from.Level
	if downgrade && !in.AllowDowngrade {
		plan.Blocked = append(plan.Blocked, fmt.Sprintf("DOWNGRADE_NOT_ACKNOWLEDGED: %s -> %s lowers the profile level", in.From, in.To))
	} else if !downgrade {
		plan.Blocked = append(plan.Blocked, to.SupersetOf(from)...)
	}

	if in.Lock != nil && !downgrade {
		plan.Blocked = append(plan.Blocked, in.Lock.ValidateUpgrade(in.From, in.To)...)
		plan.Blocked = append(plan.Blocked, in.Lock.CheckProfileContract(in.To, in.KubernetesVersion)...)
	}
	if in.Evidence != nil {
		if !in.Evidence.ProfileCertifiedFor(in.To, to.Backings) {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("TARGET_PROFILE_NOT_CERTIFIED: %s has no full delivery evidence for backing %s", in.To, to.Backings))
		}
		if in.From != in.To && len(in.Evidence.ProfilePaths) > 0 && !in.Evidence.UpgradePathCertifiedFor(in.From, in.To, to.Backings) {
			plan.Blocked = append(plan.Blocked, fmt.Sprintf("UPGRADE_PATH_NOT_CERTIFIED: %s -> %s has no acceptance evidence", in.From, in.To))
		}
	}

	plan.Steps = planUpgradeSteps(from, to, in.Lock)
	plan.Blocked = append(plan.Blocked, dataPlanePreconditions(to.Backings, in.ControlPlaneDomains, in.OperatorReady)...)
	plan.Authorized = len(plan.Blocked) == 0
	return plan, nil
}

// planUpgradeSteps builds the ordered diff steps. Reading order is human; the
// executor maps kinds onto the run-store phases.
func planUpgradeSteps(from, to ProfileSpec, lock *ReleaseLock) []UpgradeStep {
	var steps []UpgradeStep

	// Capability enablement (feature flags that appear in the target).
	have := map[string]bool{}
	for _, c := range from.Capabilities {
		have[c] = true
	}
	for _, c := range to.Capabilities {
		if !have[c] {
			steps = append(steps, UpgradeStep{Kind: "capabilities", Target: c, Detail: "enable capability", Reversible: true})
		}
	}

	// Component/backing changes.
	names := map[string]bool{}
	for n := range from.Components {
		names[n] = true
	}
	for n := range to.Components {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		was, now := from.Components[name], to.Components[name]
		if was == now {
			continue
		}
		steps = append(steps, UpgradeStep{Kind: "components", Target: name, Detail: "reconcile replicas",
			From: fmt.Sprintf("%d", was), To: fmt.Sprintf("%d", now), Reversible: true})
	}

	// Storage (managed PVCs). Growth is irreversible: PVCs cannot shrink.
	storages := map[string]bool{}
	for c := range from.Storage {
		storages[c] = true
	}
	for c := range to.Storage {
		storages[c] = true
	}
	storageNames := make([]string, 0, len(storages))
	for c := range storages {
		storageNames = append(storageNames, c)
	}
	sort.Strings(storageNames)
	for _, c := range storageNames {
		was, now := from.Storage[c], to.Storage[c]
		if _, inFrom := from.Storage[c]; inFrom {
			if _, inTo := to.Storage[c]; !inTo {
				// Externalising a managed claim is recorded by the backings step.
				continue
			}
		}
		if was.SizeGi == now.SizeGi {
			continue
		}
		steps = append(steps, UpgradeStep{Kind: "storage", Target: c, Detail: "expand managed claim",
			From: fmt.Sprintf("%dGi", was.SizeGi), To: fmt.Sprintf("%dGi", now.SizeGi), Reversible: false,
			Requires: []string{"storage class allows volume expansion"}})
	}

	if from.Backings != to.Backings {
		requires := []string{"data migration", "backup verified"}
		switch to.Backings {
		case BackingExternal:
			requires = append([]string{"external backings reachable"}, requires...)
		case BackingInClusterHA:
			requires = append([]string{"at least 3 schedulable control-plane domains", "data-plane operator ready"}, requires...)
		}
		steps = append(steps, UpgradeStep{Kind: "backings", Target: "postgresql+nats", Detail: "re-backing (data-plane-rebacking)",
			From: from.Backings, To: to.Backings, Reversible: false, Requires: requires})
	}

	// Owner-ordered schema migrations.
	if lock != nil {
		for _, m := range lock.MigrationChain {
			steps = append(steps, UpgradeStep{Kind: "migration", Target: m.Owner + "/" + m.Name,
				Detail: m.FromSchema + " -> " + m.ToSchema, Reversible: m.Reversible, Requires: []string{"backup"}})
		}
	}

	steps = append(steps,
		UpgradeStep{Kind: "release", Target: string(to.Name), Detail: "apply target profile overlay", Reversible: true, Requires: []string{"backup"}},
		UpgradeStep{Kind: "verify", Target: string(to.Name), Detail: "layered readiness gate", Reversible: true},
	)
	return steps
}
