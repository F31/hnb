// Package gslb 实现 GSLB 流量层容灾的受控变更核心（OpenSpec change
// gslb-traffic-resilience，GSLB-005）。
//
// 本包只包含"意图 + 校验 + 不可变计划"的纯逻辑，不依赖数据库与消息系统：
//  - Intent：类型化 RuntimeIntent，只携带引用与有界参数（GSLB-005 /
//    白皮书 §3.3：不携带可执行步骤、Provider 命令、目标凭据、fencing
//    token 或审批结果）；
//  - Plan：由 Intent 解析出的不可变执行计划（步骤 DAG + 语义摘要），
//    Planning 失败不产生任何运行时副作用。
package gslb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// APIVersion 是 gslb 意图契约的固定版本。
const APIVersion = "gslb.hnb.io/v1"

// IntentKind 受控流量变更意图类型（GSLB-005）。
type IntentKind string

const (
	// IntentFailover 切换 active pool（默认 require_approval）。
	IntentFailover IntentKind = "gslb.failover"
	// IntentSwitchback 回切主池（显式人工确认 + 默认 require_approval）。
	IntentSwitchback IntentKind = "gslb.switchback"
	// IntentWeightUpdate 调整成员权重（灰度，不强制审批）。
	IntentWeightUpdate IntentKind = "gslb.weight-update"
	// IntentDrill 只读演练：计算假设切换结果，不产生任何真实 DNS 变更（GSLB-010）。
	IntentDrill IntentKind = "gslb.drill"
)

// IntentMetadata 幂等与追踪元数据。
type IntentMetadata struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId"`
}

// Intent 是类型化 RuntimeIntent。禁止字段：可执行步骤、Provider 命令、
// 任意 URL、凭据、fencing token 与审批结果（fail-closed）。
type Intent struct {
	APIVersion   string         `json:"apiVersion"`
	Kind         IntentKind     `json:"kind"`
	ServiceID    string         `json:"serviceId"`
	TenantID     string         `json:"tenantId"`
	TargetPoolID string         `json:"targetPoolId,omitempty"`
	Weights      map[string]int `json:"weights,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	// DRGroupRef 是 DRProtectionGroup 编排对接缝（GSLB-009）：DR 编排器发起
	// 流量层步骤时携带保护组引用，仅用于审计/追踪，不携带执行细节。
	DRGroupRef   string         `json:"drGroupRef,omitempty"`
	Metadata     IntentMetadata `json:"metadata"`

	rawBody []byte
}

var (
	// ErrInvalidIntent 表示意图不合法（fail-closed，拒绝规划）。
	ErrInvalidIntent = errors.New("invalid gslb intent")
	// ErrPlanningNotAllowed 表示该意图类型不允许生成计划（防御）。
	ErrPlanningNotAllowed = errors.New("planning not allowed")

	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	idemPattern = regexp.MustCompile(`^[!-~]{1,128}$`)
	// drGroupRefPattern 约束 DR 保护组引用为安全标识符（GSLB-009 对接缝）。
	drGroupRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
)

// forbiddenFields 是意图中禁止出现的执行类字段（对齐契约规则
// scanRuntimeIntentExecutionFields 的精神：意图不能携带执行细节）。
var forbiddenFields = map[string]bool{
	"step": true, "steps": true, "steptype": true, "stepType": true,
	"command": true, "commands": true, "providerId": true, "providerid": true,
	"credential": true, "credentials": true, "url": true, "endpoint": true,
	"fencing": true, "fencingtoken": true, "fencingToken": true,
	"policyresult": true, "approvalresult": true, "approvalResult": true,
}

// ParseIntent 解析并校验请求体；校验失败返回 ErrInvalidIntent。
// rawBody 保留用于审计与幂等（与平台 intent 解析一致）。
func ParseIntent(body []byte) (*Intent, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrInvalidIntent)
	}
	if err := scanForbiddenFields(body); err != nil {
		return nil, err
	}
	intent := &Intent{rawBody: body}
	if err := json.Unmarshal(body, intent); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidIntent, err)
	}
	if err := intent.Validate(); err != nil {
		return nil, err
	}
	return intent, nil
}

// scanForbiddenFields 递归扫描请求体中的执行类字段并 fail-closed。
// Go 的 json.Unmarshal 会静默丢弃结构体未声明字段，因此必须先对原始
// JSON 做禁入字段扫描（GSLB-005：意图不得携带执行细节）。
func scanForbiddenFields(body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidIntent, err)
	}
	var walk func(map[string]any) error
	walk = func(node map[string]any) error {
		for key, value := range node {
			if forbiddenFields[strings.ToLower(key)] {
				return fmt.Errorf("%w: forbidden execution field %q", ErrInvalidIntent, key)
			}
			if child, ok := value.(map[string]any); ok {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(raw)
}

// RawBody 返回原始请求体（供审计；调用方不得将其作为可执行内容）。
func (i *Intent) RawBody() []byte { return i.rawBody }

// Validate 校验意图契约（GSLB-005：引用与有界参数，无执行细节）。
func (i *Intent) Validate() error {
	if i == nil {
		return fmt.Errorf("%w: nil intent", ErrInvalidIntent)
	}
	if i.APIVersion != APIVersion {
		return fmt.Errorf("%w: unsupported apiVersion %q", ErrInvalidIntent, i.APIVersion)
	}
	switch i.Kind {
	case IntentFailover, IntentSwitchback, IntentWeightUpdate, IntentDrill:
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidIntent, i.Kind)
	}
	if !uuidPattern.MatchString(i.ServiceID) {
		return fmt.Errorf("%w: serviceId must be a uuid", ErrInvalidIntent)
	}
	if i.TenantID == "" || len(i.TenantID) > 128 {
		return fmt.Errorf("%w: tenantId required (<=128)", ErrInvalidIntent)
	}
	if !idemPattern.MatchString(i.Metadata.IdempotencyKey) {
		return fmt.Errorf("%w: idempotencyKey required ([!-~]{1,128})", ErrInvalidIntent)
	}
	if !uuidPattern.MatchString(i.Metadata.CorrelationID) {
		return fmt.Errorf("%w: correlationId must be a uuid", ErrInvalidIntent)
	}
	if len(i.Reason) > 512 {
		return fmt.Errorf("%w: reason too long", ErrInvalidIntent)
	}
	if i.DRGroupRef != "" && !drGroupRefPattern.MatchString(i.DRGroupRef) {
		return fmt.Errorf("%w: invalid drGroupRef", ErrInvalidIntent)
	}

	switch i.Kind {
	case IntentFailover, IntentSwitchback:
		if !uuidPattern.MatchString(i.TargetPoolID) {
			return fmt.Errorf("%w: %s requires targetPoolId", ErrInvalidIntent, i.Kind)
		}
	case IntentWeightUpdate:
		if len(i.Weights) == 0 {
			return fmt.Errorf("%w: weight-update requires non-empty weights", ErrInvalidIntent)
		}
		total := 0
		for member, weight := range i.Weights {
			if member == "" || weight < 0 || weight > 100 {
				return fmt.Errorf("%w: invalid weight for member %q", ErrInvalidIntent, member)
			}
			total += weight
		}
		if total <= 0 {
			return fmt.Errorf("%w: total weight must be > 0", ErrInvalidIntent)
		}
	}
	return nil
}

// RequiresApproval 返回该意图是否默认 require_approval（GSLB-005）。
// failover/switchback 默认审批；weight-update 与 drill 不强制。
func (i *Intent) RequiresApproval() bool {
	switch i.Kind {
	case IntentFailover, IntentSwitchback:
		return true
	default:
		return false
	}
}

// IsDrill 返回该意图是否为只读演练（GSLB-010）。
func (i *Intent) IsDrill() bool { return i.Kind == IntentDrill }

// IsExecutable 返回该意图是否会对 DNS 数据面产生真实变更。
func (i *Intent) IsExecutable() bool { return !i.IsDrill() }

// semanticPayload 是摘要的规范化输入（键排序，确定性）。
func (i *Intent) semanticPayload() string {
	weights := make([]string, 0, len(i.Weights))
	for member, weight := range i.Weights {
		weights = append(weights, fmt.Sprintf("%q:%d", member, weight))
	}
	sort.Strings(weights)
	return strings.Join([]string{
		string(i.Kind), i.ServiceID, i.TenantID, i.TargetPoolID,
		"{" + strings.Join(weights, ",") + "}", i.DRGroupRef,
	}, "|")
}

// SemanticDigest 返回意图的确定性语义摘要（sha256），
// 用于幂等与计划一致性校验。
func (i *Intent) SemanticDigest() string {
	sum := sha256.Sum256([]byte(i.semanticPayload()))
	return hex.EncodeToString(sum[:])
}
