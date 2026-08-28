package core

import "time"

type FreshnessPolicy struct {
	StaleThreshold     time.Duration
	ActionOnStale      string
	MaxOfflineDuration time.Duration
}

type FreshnessTracker struct {
	policies map[TargetType]*FreshnessPolicy
}

func NewFreshnessTracker() *FreshnessTracker {
	return &FreshnessTracker{
		policies: map[TargetType]*FreshnessPolicy{
			TargetKubernetes: {
				StaleThreshold:     5 * time.Minute,
				ActionOnStale:      "warn",
				MaxOfflineDuration: 30 * time.Minute,
			},
			TargetContainerEngine: {
				StaleThreshold:     5 * time.Minute,
				ActionOnStale:      "warn",
				MaxOfflineDuration: 30 * time.Minute,
			},
			TargetEdgeRuntime: {
				StaleThreshold:     2 * time.Minute,
				ActionOnStale:      "queue_offline",
				MaxOfflineDuration: 60 * time.Minute,
			},
			TargetExternalService: {
				StaleThreshold:     15 * time.Minute,
				ActionOnStale:      "warn",
				MaxOfflineDuration: 120 * time.Minute,
			},
		},
	}
}

func (ft *FreshnessTracker) SetPolicy(targetType TargetType, policy *FreshnessPolicy) {
	ft.policies[targetType] = policy
}

func (ft *FreshnessTracker) GetPolicy(targetType TargetType) *FreshnessPolicy {
	if p, ok := ft.policies[targetType]; ok {
		return p
	}
	return &FreshnessPolicy{
		StaleThreshold: 5 * time.Minute,
		ActionOnStale:  "warn",
	}
}

func (ft *FreshnessTracker) Evaluate(target *RuntimeTarget) (bool, string) {
	if target.ObservedAt == nil {
		return false, "queue_offline"
	}

	policy := ft.GetPolicy(target.TargetType)
	sinceObserved := time.Since(*target.ObservedAt)

	if sinceObserved <= policy.StaleThreshold {
		return true, ""
	}

	return false, policy.ActionOnStale
}

func (ft *FreshnessTracker) ApplyEdgeDiscovery(target *RuntimeTarget, discovery *KubeEdgeDiscoveryResult) EdgeNodeStatus {
	now := time.Now()
	target.ObservedAt = &now

	if discovery == nil {
		target.Status = StatusUnknown
		return EdgeNodeUnknown
	}

	if discovery.OfflineCount > 0 && discovery.OfflineCount == discovery.TotalNodes {
		target.Status = StatusOffline
		return EdgeNodeOffline
	}

	if discovery.OfflineCount > 0 {
		target.Status = StatusDegraded
		return EdgeNodeOnline
	}

	target.Status = StatusOnline
	return EdgeNodeOnline
}

func (ft *FreshnessTracker) EvaluateEdgeTarget(target *RuntimeTarget) (bool, string, TargetStatus) {
	ok, action := ft.Evaluate(target)
	if !ok {
		switch action {
		case "queue_offline":
			target.Status = StatusOffline
			return ok, action, StatusOffline
		case "reject":
			return ok, action, target.Status
		default:
			return ok, action, target.Status
		}
	}
	return ok, action, target.Status
}