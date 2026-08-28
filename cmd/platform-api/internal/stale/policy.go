package stale

import "fmt"

type Outcome string

const (
	OutcomeAllow           Outcome = "allow"
	OutcomeRequireApproval Outcome = "require_approval"
	OutcomeQueuedOffline   Outcome = "queued_offline"
	OutcomeDeny            Outcome = "deny"
)

type Policy struct {
	Upgrade  Outcome
	Unmanage Outcome
}

func NewPolicy(upgrade, unmanage string) (Policy, error) {
	p := Policy{Upgrade: Outcome(upgrade), Unmanage: Outcome(unmanage)}
	if !validOutcome(p.Upgrade) || !validOutcome(p.Unmanage) {
		return Policy{}, fmt.Errorf("STALE policy must be allow, require_approval, queued_offline, or deny")
	}
	return p, nil
}

func DefaultPolicy() Policy {
	return Policy{Upgrade: OutcomeRequireApproval, Unmanage: OutcomeRequireApproval}
}

func (p Policy) Evaluate(intentKind string) Outcome {
	if intentKind == "DeleteRuntimeTarget" {
		return p.Unmanage
	}
	return p.Upgrade
}

func validOutcome(outcome Outcome) bool {
	return outcome == OutcomeAllow || outcome == OutcomeRequireApproval || outcome == OutcomeQueuedOffline || outcome == OutcomeDeny
}
