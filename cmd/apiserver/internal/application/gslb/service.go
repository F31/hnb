package gslb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
)

var (
	ErrNotFound = errors.New("gslb service not found")
	ErrForbidden = errors.New("gslb forbidden")
	ErrInvalid   = errors.New("gslb invalid")
)

// 状态常量（与迁移 081 的 CHECK 一致）
const (
	StatusPendingApproval = "PendingApproval"
	StatusApproved        = "Approved"
	StatusRejected        = "Rejected"
	StatusDispatched      = "Dispatched"
	StatusSucceeded       = "Succeeded"
	StatusFailed          = "Failed"
	StatusDrillCompleted  = "DrillCompleted"
)

// NATS subjects（V2.6 §22.3 事件命名 + 仓库 hnb.event.<domain>.<action>.v1 约定）
const (
	EventIntentSubmitted = "hnb.event.gslb.intent-submitted.v1"
	EventStatusChanged   = "hnb.event.gslb.status-changed.v1"
	// 执行命令：经 relay 自建的 domain-events 流（hnb.event.>）可靠投递给
	// gslb-controller 消费者；控制器只在该命令驱动下执行 DNS 变更（GSLB-005）。
	CommandStepRequested = "hnb.event.gslb.step-requested.v1"
)

// Service 是 GSLB 服务域模型（与迁移 081 gslb_services 对齐）。
type Service struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	Name            string    `json:"name"`
	Domain          string    `json:"domain"`
	RoutingMode     string    `json:"routingMode"`
	ActivePoolID    string    `json:"activePoolId,omitempty"`
	LifecycleState  string    `json:"lifecycleState"`
	RequireApproval bool      `json:"requireApproval"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ReadModel 是 GSLB 只读投影（GSLB-007）：查询只读本快照。
type ReadModel struct {
	ServiceID        string    `json:"serviceId"`
	TenantID         string    `json:"tenantId"`
	Domain           string    `json:"domain"`
	ActivePoolID     string    `json:"activePoolId,omitempty"`
	LifecycleState   string    `json:"lifecycleState"`
	HealthyPools     []string  `json:"healthyPools"`
	CurrentDNSTargets []string `json:"currentDnsTargets"`
	// 最近演练结果（GSLB-010：演练报告写入 Read Model）
	LastDrillReportID string     `json:"lastDrillReportId,omitempty"`
	LastDrillVerdict  string     `json:"lastDrillVerdict,omitempty"`
	LastDrillAt       *time.Time `json:"lastDrillAt,omitempty"`
	ObservedAt       time.Time `json:"observedAt"`
}

// SwitchRequest 是审批门控的流量变更请求（受控写路径，GSLB-005）。
type SwitchRequest struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId"`
	ServiceID       string          `json:"serviceId"`
	IntentKind      string          `json:"intentKind"`
	IntentDigest    string          `json:"intentDigest"`
	PlanSnapshot    json.RawMessage `json:"planSnapshot"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	CorrelationID   string          `json:"correlationId"`
	RequireApproval bool            `json:"requireApproval"`
	Status          string          `json:"status"`
	// OperationID 关联平台 operations 行（Operation Center 统一接线）。
	OperationID     string          `json:"operationId,omitempty"`
	// DRGroupRef 是 DRProtectionGroup 编排来源引用（GSLB-009 对接缝）。
	DRGroupRef      string          `json:"drGroupRef,omitempty"`
	ActorID         string          `json:"actorId,omitempty"`
	ApprovedBy      string          `json:"approvedBy,omitempty"`
	ApprovedAt      *time.Time      `json:"approvedAt,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Error           string          `json:"error,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// 演练结论（与迁移 082 gslb_drill_reports.verdict CHECK 一致）
const (
	DrillVerdictReady    = "Ready"
	DrillVerdictDegraded = "Degraded"
	DrillVerdictBlocked  = "Blocked"
)

// DrillCheck 是演练报告中的单项检查。
type DrillCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// DrillReport 是只读演练的结构化报告（GSLB-010）：独立落库，供查询 API
// 与 Operation 详情展示；演练不产生任何真实 DNS 变更。
type DrillReport struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenantId"`
	ServiceID string          `json:"serviceId"`
	RequestID string          `json:"requestId"`
	Verdict   string          `json:"verdict"`
	Report    json.RawMessage `json:"report"`
	CreatedAt time.Time       `json:"createdAt"`
}

// drillReportPayload 是 report JSONB 的结构。
type drillReportPayload struct {
	ServiceID        string            `json:"serviceId"`
	Domain           string            `json:"domain"`
	ActivePoolID     string            `json:"activePoolId,omitempty"`
	TargetPoolID     string            `json:"targetPoolId,omitempty"`
	CurrentTargets   []string          `json:"currentTargets"`
	ProjectedTargets []string          `json:"projectedTargets"`
	ProjectedWeights map[string]int    `json:"projectedWeights,omitempty"`
	HealthyPools     []string          `json:"healthyPools,omitempty"`
	Checks           []DrillCheck      `json:"checks"`
	Verdict          string            `json:"verdict"`
	GeneratedAt      time.Time         `json:"generatedAt"`
}

// OutboxEvent 与写操作同事务的可靠事件（Transactional Outbox）。
type OutboxEvent struct {
	MessageID        string
	MessageType      string
	SchemaVersion    string
	Subject          string
	TenantID         string
	ActorID          string
	CorrelationID    string
	IdempotencyKey   string
	AggregateID      string
	AggregateVersion int64
	Payload          any
}

// Repository 访问 GSLB 平台状态（迁移 081/082）。
type Repository interface {
	GetService(ctx context.Context, id, tenantID string) (Service, bool, error)
	GetPoolMemberClusterIDs(ctx context.Context, poolID string) ([]string, error)
	GetSwitchRequestByKey(ctx context.Context, tenantID, serviceID, idempotencyKey string) (SwitchRequest, bool, error)
	GetSwitchRequest(ctx context.Context, id, tenantID string) (SwitchRequest, bool, error)
	// CreateSwitchRequest 同事务写入：切换请求 + 平台 operations/operation_steps
	// 行（Operation Center 统一接线）+ 演练报告（drill 非空时）+ Outbox 事件。
	CreateSwitchRequest(ctx context.Context, req SwitchRequest, drill *DrillReport, events []OutboxEvent) error
	// UpdateSwitchRequestStatus 同事务更新请求状态并同步关联 Operation 行。
	UpdateSwitchRequestStatus(ctx context.Context, id, status string, fields map[string]any, events []OutboxEvent) error
	ListReadModels(ctx context.Context, tenantID string) ([]ReadModel, error)
	GetReadModel(ctx context.Context, id, tenantID string) (ReadModel, bool, error)
	// ListDrillReports 返回服务的结构化演练报告（GSLB-010，租户隔离）。
	ListDrillReports(ctx context.Context, serviceID, tenantID string) ([]DrillReport, error)
}

type App struct{ repo Repository }

func NewService(repo Repository) *App { return &App{repo: repo} }

// SubmitIntent 是 GSLB 流量变更的唯一提交入口（GSLB-005）：
// 解析/校验意图 → 读取服务（租户范围）→ 生成不可变计划 → 幂等写入
// 审批门控的切换请求 → 同事务 Outbox 事件。
func (a *App) SubmitIntent(ctx context.Context, body []byte, pathServiceID string, trusted iam.TrustedContext) (SwitchRequest, error) {
	intent, err := gslb.ParseIntent(body)
	if err != nil {
		return SwitchRequest{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if intent.ServiceID != pathServiceID {
		return SwitchRequest{}, fmt.Errorf("%w: serviceId mismatch", ErrInvalid)
	}
	if intent.TenantID != trusted.TenantID {
		return SwitchRequest{}, fmt.Errorf("%w: tenantId mismatch", ErrInvalid)
	}
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionExecute) {
		return SwitchRequest{}, ErrForbidden
	}

	service, ok, err := a.repo.GetService(ctx, pathServiceID, trusted.TenantID)
	if err != nil {
		return SwitchRequest{}, err
	}
	if !ok {
		return SwitchRequest{}, ErrNotFound
	}

	// 幂等：同 key 已存在则直接返回既有请求
	if existing, found, err := a.repo.GetSwitchRequestByKey(ctx, trusted.TenantID, pathServiceID, intent.Metadata.IdempotencyKey); err != nil {
		return SwitchRequest{}, err
	} else if found {
		return existing, nil
	}

	plan, err := a.buildPlan(ctx, intent, service)
	if err != nil {
		return SwitchRequest{}, err
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return SwitchRequest{}, fmt.Errorf("marshal plan: %w", err)
	}

	requireApproval := intent.RequiresApproval() && service.RequireApproval
	// GSLB-009：DR 保护组编排的回切必须显式人工确认，不允许服务级降级跳过审批。
	if intent.DRGroupRef != "" && intent.Kind == gslb.IntentSwitchback {
		requireApproval = true
	}
	status := StatusApproved
	if requireApproval {
		status = StatusPendingApproval
	}
	if intent.IsDrill() {
		status = StatusDrillCompleted
	}

	now := time.Now().UTC()
	request := SwitchRequest{
		ID:              uuid.NewString(),
		TenantID:        trusted.TenantID,
		ServiceID:       pathServiceID,
		IntentKind:      string(intent.Kind),
		IntentDigest:    intent.SemanticDigest(),
		PlanSnapshot:    planJSON,
		IdempotencyKey:  intent.Metadata.IdempotencyKey,
		CorrelationID:   intent.Metadata.CorrelationID,
		RequireApproval: requireApproval,
		Status:          status,
		OperationID:     uuid.NewString(),
		DRGroupRef:      intent.DRGroupRef,
		ActorID:         trusted.SubjectID,
		Reason:          intent.Reason,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// 只读演练：生成结构化报告（GSLB-010），不产生任何执行命令。
	var drill *DrillReport
	if intent.IsDrill() {
		drill, err = a.buildDrillReport(ctx, intent, service, request.ID, now)
		if err != nil {
			return SwitchRequest{}, err
		}
	}

	events := []OutboxEvent{submittedEvent(intent, trusted, request.ID)}
	// 免审批且可执行的请求立即派发执行命令（GSLB-005：控制器只在命令下执行）
	if status == StatusApproved && intent.IsExecutable() {
		events = append(events, stepRequestedEvent(planJSON, intent, trusted, request))
	}
	if err := a.repo.CreateSwitchRequest(ctx, request, drill, events); err != nil {
		return SwitchRequest{}, err
	}
	return request, nil
}

// ListServices 返回租户范围内的只读投影（GSLB-007）。
func (a *App) ListServices(ctx context.Context, tenantID string) ([]ReadModel, error) {
	return a.repo.ListReadModels(ctx, tenantID)
}

// GetServiceProjection 返回单个只读投影（租户隔离）。
func (a *App) GetServiceProjection(ctx context.Context, id, tenantID string) (ReadModel, error) {
	model, ok, err := a.repo.GetReadModel(ctx, id, tenantID)
	if err != nil {
		return ReadModel{}, err
	}
	if !ok {
		return ReadModel{}, ErrNotFound
	}
	return model, nil
}

// ListDrillReports 返回服务的结构化演练报告（GSLB-010，租户隔离）。
func (a *App) ListDrillReports(ctx context.Context, serviceID, tenantID string) ([]DrillReport, error) {
	return a.repo.ListDrillReports(ctx, serviceID, tenantID)
}

// buildDrillReport 计算只读演练报告：对比当前活跃池与目标池，产出结论与
// 检查项。纯计算 + 落库，不触达 DNS 数据面（GSLB-010）。
func (a *App) buildDrillReport(ctx context.Context, intent *gslb.Intent, service Service, requestID string, now time.Time) (*DrillReport, error) {
	var currentTargets []string
	if service.ActivePoolID != "" {
		targets, err := a.repo.GetPoolMemberClusterIDs(ctx, service.ActivePoolID)
		if err != nil {
			return nil, err
		}
		currentTargets = targets
	}
	var projectedTargets []string
	if intent.TargetPoolID != "" {
		targets, err := a.repo.GetPoolMemberClusterIDs(ctx, intent.TargetPoolID)
		if err != nil {
			return nil, err
		}
		projectedTargets = targets
	}
	// 契约要求数组非 null（GslbDrillReportPayload）。
	if currentTargets == nil {
		currentTargets = []string{}
	}
	if projectedTargets == nil {
		projectedTargets = []string{}
	}
	// 健康上下文来自 Read Model 投影（请求路径零探测，GSLB-007）。
	var healthyPools []string
	if model, ok, err := a.repo.GetReadModel(ctx, service.ID, service.TenantID); err != nil {
		return nil, err
	} else if ok {
		healthyPools = model.HealthyPools
	}

	checks := []DrillCheck{
		{Name: "target-pool-selected", Passed: intent.TargetPoolID != "",
			Detail: "演练未指定目标池时仅评估当前流量分布"},
		{Name: "target-pool-has-members", Passed: intent.TargetPoolID == "" || len(projectedTargets) > 0,
			Detail: "目标池无启用成员时切换将被阻断"},
		{Name: "current-targets-known", Passed: service.ActivePoolID == "" || len(currentTargets) > 0,
			Detail: "当前活跃池无启用成员，回滚目标为空"},
	}
	verdict := DrillVerdictReady
	for _, check := range checks {
		if !check.Passed {
			verdict = DrillVerdictBlocked
			break
		}
	}
	if verdict == DrillVerdictReady && intent.TargetPoolID != "" && len(healthyPools) > 0 {
		healthy := false
		for _, poolID := range healthyPools {
			if poolID == intent.TargetPoolID {
				healthy = true
				break
			}
		}
		if !healthy {
			verdict = DrillVerdictDegraded
			checks = append(checks, DrillCheck{
				Name: "target-pool-healthy", Passed: false,
				Detail: "目标池当前未在健康投影中，切换后可用性存在风险",
			})
		}
	}

	payload := drillReportPayload{
		ServiceID:        service.ID,
		Domain:           service.Domain,
		ActivePoolID:     service.ActivePoolID,
		TargetPoolID:     intent.TargetPoolID,
		CurrentTargets:   currentTargets,
		ProjectedTargets: projectedTargets,
		ProjectedWeights: intent.Weights,
		HealthyPools:     healthyPools,
		Checks:           checks,
		Verdict:          verdict,
		GeneratedAt:      now,
	}
	reportJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal drill report: %w", err)
	}
	return &DrillReport{
		ID:        uuid.NewString(),
		TenantID:  service.TenantID,
		ServiceID: service.ID,
		RequestID: requestID,
		Verdict:   verdict,
		Report:    reportJSON,
		CreatedAt: now,
	}, nil
}

// Approve 审批通过切换请求；可执行请求随后派发执行命令。
func (a *App) Approve(ctx context.Context, requestID string, trusted iam.TrustedContext) (SwitchRequest, error) {
	return a.transition(ctx, requestID, trusted, StatusApproved, "approved_by", trusted.SubjectID, true)
}

// Reject 拒绝切换请求。
func (a *App) Reject(ctx context.Context, requestID string, trusted iam.TrustedContext) (SwitchRequest, error) {
	return a.transition(ctx, requestID, trusted, StatusRejected, "approved_by", trusted.SubjectID, false)
}

func (a *App) transition(ctx context.Context, requestID string, trusted iam.TrustedContext, toStatus, field, value string, dispatch bool) (SwitchRequest, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionUpdate) {
		return SwitchRequest{}, ErrForbidden
	}
	request, ok, err := a.repo.GetSwitchRequest(ctx, requestID, trusted.TenantID)
	if err != nil {
		return SwitchRequest{}, err
	}
	if !ok {
		return SwitchRequest{}, ErrNotFound
	}
	if request.Status != StatusPendingApproval {
		return SwitchRequest{}, fmt.Errorf("%w: cannot transition from %s", ErrInvalid, request.Status)
	}

	fields := map[string]any{field: value, "approved_at": time.Now().UTC()}
	events := []OutboxEvent{statusChangedEvent(request, toStatus, trusted)}
	if toStatus == StatusApproved && dispatch {
		events = append(events, stepRequestedEvent(request.PlanSnapshot, nil, trusted, request))
	}
	if err := a.repo.UpdateSwitchRequestStatus(ctx, requestID, toStatus, fields, events); err != nil {
		return SwitchRequest{}, err
	}
	request.Status = toStatus
	return request, nil
}

// buildPlan 从意图构造不可变计划：目标池成员作为 DNS 目标，
// 当前活跃池成员作为回滚目标。
func (a *App) buildPlan(ctx context.Context, intent *gslb.Intent, service Service) (*gslb.Plan, error) {
	input := gslb.PlanInput{
		ServiceID:    intent.ServiceID,
		TenantID:     intent.TenantID,
		Domain:       service.Domain,
		TargetPoolID: intent.TargetPoolID,
	}
	if intent.TargetPoolID != "" {
		targets, err := a.repo.GetPoolMemberClusterIDs(ctx, intent.TargetPoolID)
		if err != nil {
			return nil, err
		}
		input.Targets = targets
	}
	if service.ActivePoolID != "" && service.ActivePoolID != intent.TargetPoolID {
		previous, err := a.repo.GetPoolMemberClusterIDs(ctx, service.ActivePoolID)
		if err != nil {
			return nil, err
		}
		input.PreviousTargets = previous
	}
	input.Weights = intent.Weights
	return intent.BuildPlan(input)
}

func hasPermission(trusted iam.TrustedContext, resource string, action iam.AuthorizationAction) bool {
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID == trusted.TenantID && permission.Action == action && (permission.ResourceKind == resource || permission.ResourceKind == "*") {
			return true
		}
	}
	return false
}

func submittedEvent(intent *gslb.Intent, trusted iam.TrustedContext, requestID string) OutboxEvent {
	return OutboxEvent{
		MessageID:        uuid.NewString(),
		MessageType:      EventIntentSubmitted,
		SchemaVersion:    "v1",
		Subject:          EventIntentSubmitted,
		TenantID:         trusted.TenantID,
		ActorID:          trusted.SubjectID,
		CorrelationID:    intent.Metadata.CorrelationID,
		IdempotencyKey:   fmt.Sprintf("gslb-submit-%s", intent.Metadata.IdempotencyKey),
		AggregateID:      intent.ServiceID,
		AggregateVersion: 0,
		Payload: map[string]any{
			"requestId":   requestID,
			"serviceId":   intent.ServiceID,
			"kind":        string(intent.Kind),
			"digest":      intent.SemanticDigest(),
			"tenantId":    trusted.TenantID,
			"drGroupRef":  intent.DRGroupRef,
		},
	}
}

func statusChangedEvent(request SwitchRequest, toStatus string, trusted iam.TrustedContext) OutboxEvent {
	return OutboxEvent{
		MessageID:        uuid.NewString(),
		MessageType:      EventStatusChanged,
		SchemaVersion:    "v1",
		Subject:          EventStatusChanged,
		TenantID:         request.TenantID,
		ActorID:          trusted.SubjectID,
		CorrelationID:    request.CorrelationID,
		IdempotencyKey:   fmt.Sprintf("gslb-status-%s-%s", request.ID, toStatus),
		AggregateID:      request.ServiceID,
		AggregateVersion: 0,
		Payload: map[string]any{
			"requestId":   request.ID,
			"serviceId":   request.ServiceID,
			"status":      toStatus,
			"intentKind":  request.IntentKind,
			"operationId": request.OperationID,
			"drGroupRef":  request.DRGroupRef,
		},
	}
}

func stepRequestedEvent(planJSON []byte, intent *gslb.Intent, trusted iam.TrustedContext, request ...SwitchRequest) OutboxEvent {
	req := SwitchRequest{}
	if len(request) > 0 {
		req = request[0]
	}
	correlationID := ""
	idempotencyKey := ""
	if intent != nil {
		correlationID = intent.Metadata.CorrelationID
		idempotencyKey = intent.Metadata.IdempotencyKey
	} else {
		correlationID = req.CorrelationID
		idempotencyKey = req.IdempotencyKey
	}
	payload := map[string]any{
		"requestId":   req.ID,
		"serviceId":   req.ServiceID,
		"operationId": req.OperationID,
		"plan":        json.RawMessage(planJSON),
		"intentKind":  req.IntentKind,
	}
	return OutboxEvent{
		MessageID:        uuid.NewString(),
		MessageType:      CommandStepRequested,
		SchemaVersion:    "v1",
		Subject:          CommandStepRequested,
		TenantID:         req.TenantID,
		ActorID:          trusted.SubjectID,
		CorrelationID:    correlationID,
		IdempotencyKey:   fmt.Sprintf("gslb-dispatch-%s", idempotencyKey),
		AggregateID:      req.ServiceID,
		AggregateVersion: 0,
		Payload:          payload,
	}
}
