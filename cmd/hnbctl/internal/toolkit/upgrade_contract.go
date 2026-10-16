package toolkit

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateUpgrade verifies that a lock authorises the requested profile
// transition and that the declared migration chain is contiguous per owner.
// Callers use it before planning or executing an upgrade.
func (l *ReleaseLock) ValidateUpgrade(from, to Profile) []string {
	var problems []string
	if len(l.SupportedUpgrades) == 0 {
		return []string{"UPGRADE_PATH_UNDECLARED: lock declares no supported upgrades"}
	}
	var matched *LockUpgradePath
	for i := range l.SupportedUpgrades {
		if l.SupportedUpgrades[i].From == string(from) && l.SupportedUpgrades[i].To == string(to) {
			matched = &l.SupportedUpgrades[i]
			break
		}
	}
	if matched == nil {
		problems = append(problems, fmt.Sprintf("UPGRADE_PATH_UNDECLARED: %s -> %s is not declared", from, to))
	} else if backingChangeRequired(from, to) && !matched.BackingChange {
		problems = append(problems, fmt.Sprintf("BACKING_CHANGE_NOT_SUPPORTED: %s -> %s changes the data-plane backing but the lock path does not set backingChange", from, to))
	}
	return append(problems, l.validateMigrationChain()...)
}

// backingChangeRequired reports whether a from->to profile transition changes
// the data-plane backing kind, per the shape vectors.
func backingChangeRequired(from, to Profile) bool {
	fs, okF := ProfileShape(from)
	ts, okT := ProfileShape(to)
	if !okF || !okT {
		return false
	}
	return fs.Backings != ts.Backings
}

// validateMigrationChain checks that each owner's schema transitions form a
// contiguous chain with no gap.
func (l *ReleaseLock) validateMigrationChain() []string {
	var problems []string
	last := map[string]string{}
	for i, step := range l.MigrationChain {
		if prev, ok := last[step.Owner]; ok && prev != step.FromSchema {
			problems = append(problems, fmt.Sprintf("MIGRATION_CHAIN_GAP: owner %s step %d starts at %s after %s", step.Owner, i, step.FromSchema, prev))
		}
		last[step.Owner] = step.ToSchema
	}
	return problems
}

// ProfileContract returns the declared contract for a profile, if any.
func (l *ReleaseLock) ProfileContract(p Profile) (LockProfileContract, bool) {
	for _, contract := range l.Profiles {
		if contract.Name == string(p) {
			return contract, true
		}
	}
	return LockProfileContract{}, false
}

// CheckProfileContract verifies a target profile satisfies the lock's declared
// required capabilities and minimum Kubernetes version.
func (l *ReleaseLock) CheckProfileContract(p Profile, kubernetesVersion string) []string {
	contract, ok := l.ProfileContract(p)
	if !ok {
		return nil
	}
	var problems []string
	shape, known := ProfileShape(p)
	if known {
		have := map[string]bool{}
		for _, c := range shape.Capabilities {
			have[c] = true
		}
		for _, required := range contract.RequiredCapabilities {
			if !have[required] {
				problems = append(problems, "lock requires capability "+required+" not provided by profile "+string(p))
			}
		}
		if len(contract.Backings) > 0 && shape.Backings != "" && !containsStr(contract.Backings, shape.Backings) {
			problems = append(problems, fmt.Sprintf("lock profile %s does not allow backing %s", p, shape.Backings))
		}
	}
	if contract.MinKubernetesVersion != "" && kubernetesVersion != "" && compareDottedVersions(kubernetesVersion, contract.MinKubernetesVersion) < 0 {
		problems = append(problems, fmt.Sprintf("kubernetes %s is below the lock minimum %s for profile %s", kubernetesVersion, contract.MinKubernetesVersion, p))
	}
	return problems
}

// compareDottedVersions compares dotted numeric versions (-1, 0, 1). Non-numeric
// suffixes (e.g. +k3s1) are ignored; unparsable segments compare as 0.
func compareDottedVersions(a, b string) int {
	split := func(v string) []int {
		v = strings.SplitN(v, "-", 2)[0]
		v = strings.SplitN(v, "+", 2)[0]
		parts := strings.Split(v, ".")
		out := make([]int, len(parts))
		for i, p := range parts {
			out[i], _ = strconv.Atoi(strings.TrimSpace(p))
		}
		return out
	}
	as, bs := split(a), split(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}
