package gslb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// StepType 是 gslb 执行计划步骤类型（由 gslb-dns-provider 执行，GSLB-006）。
type StepType string

const (
	// StepDNSApply 将目标池记录集（含权重/TTL）写入 DNS 数据面。
	StepDNSApply StepType = "gslb_dns_apply"
	// StepDNSVerify 验证 DNS 权威查询已解析到预期目标（TTL 感知）。
	StepDNSVerify StepType = "gslb_dns_verify"
	// StepDNSRevert 补偿步骤：恢复到上一已知目标（GSLB-005 失败补偿）。
	StepDNSRevert StepType = "gslb_dns_revert"
	// StepDrillCompute 只读演练计算：不产生任何 DNS 变更（GSLB-010）。
	StepDrillCompute StepType = "gslb_drill_compute"
)

// Step 是计划中的单个可执行单元。
type Step struct {
	StepID        string         `json:"stepId"`
	StepType      StepType       `json:"stepType"`
	DependsOn     []string       `json:"dependsOn"`
	Inputs        map[string]any `json:"inputs"`
	IdempotencyKey string        `json:"idempotencyKey"`
	// Compensation 指向回滚该步骤的步骤 ID（若无则空）。
	Compensation string `json:"compensation,omitempty"`
}

// Plan 是不可变执行计划：由 Intent 解析而来，钉死目标池、权重与步骤 DAG。
type Plan struct {
	PlanID         string `json:"planId"`
	IntentID       string `json:"intentId"`
	SemanticDigest string `json:"semanticDigest"`
	Steps          []Step `json:"steps"`
}

// Plan 输入上下文：目标池的成员目标（供 provider 写入 DNS）。
type PlanInput struct {
	ServiceID    string
	TenantID     string
	Domain       string
	TargetPoolID string
	// Targets 是目标池成员的可解析目标（如集群 API 入口地址）。
	Targets []string
	// Weights 为可选成员权重（weight-update 使用）。
	Weights map[string]int
	// PreviousTargets 为回滚目标（切换/回切时提供上一已知目标）。
	PreviousTargets []string
}

// BuildPlan 将意图解析为不可变计划。Planning 失败不产生任何运行时副作用。
func (i *Intent) BuildPlan(input PlanInput) (*Plan, error) {
	if err := i.Validate(); err != nil {
		return nil, err
	}
	if input.ServiceID != i.ServiceID {
		return nil, fmt.Errorf("%w: plan serviceId mismatch", ErrInvalidIntent)
	}
	digest := i.SemanticDigest()
	planID := fmt.Sprintf("gslb-plan-%s", digest[:16])

	var steps []Step
	switch i.Kind {
	case IntentFailover, IntentSwitchback:
		if len(input.Targets) == 0 {
			return nil, fmt.Errorf("%w: %s requires non-empty targets", ErrInvalidIntent, i.Kind)
		}
		apply := Step{
			StepID:        "apply",
			StepType:      StepDNSApply,
			Inputs: map[string]any{
				"serviceId":     i.ServiceID,
				"domain":        input.Domain,
				"targetPoolId":  i.TargetPoolID,
				"targets":       input.Targets,
			},
			IdempotencyKey: fmt.Sprintf("%s-apply", i.Metadata.IdempotencyKey),
			Compensation:   "revert",
		}
		verify := Step{
			StepID:        "verify",
			StepType:      StepDNSVerify,
			DependsOn:     []string{"apply"},
			Inputs: map[string]any{
				"serviceId": i.ServiceID,
				"domain":    input.Domain,
				"targets":   input.Targets,
			},
			IdempotencyKey: fmt.Sprintf("%s-verify", i.Metadata.IdempotencyKey),
		}
		revert := Step{
			StepID:        "revert",
			StepType:      StepDNSRevert,
			Inputs: map[string]any{
				"serviceId": i.ServiceID,
				"domain":    input.Domain,
				"targets":   input.PreviousTargets,
			},
			IdempotencyKey: fmt.Sprintf("%s-revert", i.Metadata.IdempotencyKey),
		}
		steps = []Step{apply, verify, revert}
	case IntentWeightUpdate:
		weights := make(map[string]any, len(i.Weights))
		for member, weight := range i.Weights {
			weights[member] = weight
		}
		apply := Step{
			StepID:        "apply",
			StepType:      StepDNSApply,
			Inputs: map[string]any{
				"serviceId": i.ServiceID,
				"domain":    input.Domain,
				"weights":   weights,
			},
			IdempotencyKey: fmt.Sprintf("%s-apply", i.Metadata.IdempotencyKey),
		}
		verify := Step{
			StepID:        "verify",
			StepType:      StepDNSVerify,
			DependsOn:     []string{"apply"},
			Inputs: map[string]any{
				"serviceId": i.ServiceID,
				"domain":    input.Domain,
			},
			IdempotencyKey: fmt.Sprintf("%s-verify", i.Metadata.IdempotencyKey),
		}
		steps = []Step{apply, verify}
	case IntentDrill:
		drillWeights := make(map[string]any, len(i.Weights))
		for member, weight := range i.Weights {
			drillWeights[member] = weight
		}
		steps = []Step{{
			StepID:        "compute",
			StepType:      StepDrillCompute,
			Inputs: map[string]any{
				"serviceId":    i.ServiceID,
				"targetPoolId": i.TargetPoolID,
				"weights":      drillWeights,
			},
			IdempotencyKey: fmt.Sprintf("%s-compute", i.Metadata.IdempotencyKey),
		}}
	default:
		return nil, fmt.Errorf("%w: unknown kind %q", ErrPlanningNotAllowed, i.Kind)
	}

	return &Plan{
		PlanID:         planID,
		IntentID:       digest,
		SemanticDigest: digest,
		Steps:          steps,
	}, nil
}

// CanonicalDigest 对计划做确定性摘要（步骤类型 + 依赖 + 输入规范化），
// 用于计划一致性校验（钉死计划身份）。
func (p *Plan) CanonicalDigest() string {
	h := sha256.New()
	for _, step := range p.Steps {
		h.Write([]byte(step.StepType))
		h.Write([]byte{0})
		sorted := append([]string(nil), step.DependsOn...)
		sort.Strings(sorted)
		for _, dep := range sorted {
			h.Write([]byte(dep))
			h.Write([]byte{0})
		}
		for _, key := range sortedKeys(step.Inputs) {
			h.Write([]byte(key))
			h.Write([]byte("="))
			fmt.Fprint(h, step.Inputs[key])
			h.Write([]byte{1})
		}
		h.Write([]byte{2})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
