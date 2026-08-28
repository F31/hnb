// Package dr 实现 DRProtectionGroup 容灾编排（OpenSpec change dr-protection-group，
// OBS-008）：按"数据层 → 流量层"顺序编排地域级切换；流量层步骤复用 gslb 受控
// 意图链路（OBS-007 对接缝：drGroupRef + 审批门控 + Operation 行）。
//
// 本包只做编排与状态推进，不直接触达 DNS/数据面；数据层成员为引用 + 人工
// 确认门（Provider 化接入留待后续 change）。
package dr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	gslbapp "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
)

var (
	ErrNotFound  = errors.New("dr resource not found")
	ErrForbidden = errors.New("dr forbidden")
	ErrInvalid   = errors.New("dr invalid")
	ErrConflict  = errors.New("dr conflict")
)

// 运行状态（与迁移 083 dr_switch_runs.status CHECK 一致）
const (
	RunDataLayerPending   = "DataLayerPending"
	RunDataLayerCompleted = "DataLayerCompleted"
	RunTrafficDispatched  = "TrafficDispatched"
	RunAwaitingApproval   = "AwaitingApproval"
	RunCompleted          = "Completed"
	RunFailed             = "Failed"
	RunCancelled          = "Cancelled"
)

// 成员类型（与迁移 083 CHECK 一致）
const (
	MemberGSLBService = "gslb_service"
	MemberDataLayer   = "data_layer_ref"
)

// 切换方向
const (
	DirectionFailover   = "failover"
	DirectionSwitchback = "switchback"
)

// NATS subjects（仓库 hnb.event.<domain>.<action>.v1 约定）
const (
	EventSwitchInitiated    = "hnb.event.dr.switch-initiated.v1"
	EventDataLayerConfirmed = "hnb.event.dr.data-layer-confirmed.v1"
	EventTrafficDispatched  = "hnb.event.dr.traffic-dispatched.v1"
	EventRunStatusChanged   = "hnb.event.dr.run-status-changed.v1"
)

// Group 是 DRProtectionGroup（迁移 083）。
type Group struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenantId"`
	Name           string    `json:"name"`
	PrimaryRegion  string    `json:"primaryRegion"`
	StandbyRegion  string    `json:"standbyRegion"`
	LifecycleState string    `json:"lifecycleState"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Member 是保护组成员：gslb_service（流量层）或 data_layer_ref（数据层引用）。
type Member struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"groupId"`
	MemberType string    `json:"memberType"`
	RefID      string    `json:"refId"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SwitchRun 是一次切换链运行（OBS-008）。
type SwitchRun struct {
	ID                string    `json:"id"`
	GroupID           string    `json:"groupId"`
	TenantID          string    `json:"tenantId"`
	Direction         string    `json:"direction"`
	Status            string    `json:"status"`
	IdempotencyKey    string    `json:"idempotencyKey"`
	CorrelationID     string    `json:"correlationId"`
	OperationID       string    `json:"operationId,omitempty"`
	TrafficRequestIDs []string  `json:"trafficRequestIds"`
	Reason            string    `json:"reason,omitempty"`
	Error             string    `json:"error,omitempty"`
	ActorID           string    `json:"actorId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// GroupDetail 是组详情（含成员与最近运行）。
type GroupDetail struct {
	Group   Group       `json:"group"`
	Members []Member    `json:"members"`
	Runs    []SwitchRun `json:"runs"`
}

// OutboxEvent 与写操作同事务的可靠事件（Transactional Outbox）。
type OutboxEvent struct {
	MessageID      string
	MessageType    string
	SchemaVersion  string
	Subject        string
	TenantID       string
	ActorID        string
	CorrelationID  string
	IdempotencyKey string
	AggregateID    string
	Payload        any
}

// GSLBSubmitter 是 gslb 受控意图提交入口（复用 gslb 应用层审批门控链路）。
type GSLBSubmitter interface {
	SubmitIntent(ctx context.Context, body []byte, pathServiceID string, trusted iam.TrustedContext) (gslbapp.SwitchRequest, error)
}

// Repository 访问 DR 平台状态（迁移 083）。
type Repository interface {
	CreateGroup(ctx context.Context, group Group) error
	GetGroup(ctx context.Context, id, tenantID string) (Group, bool, error)
	ListGroups(ctx context.Context, tenantID string) ([]Group, error)
	AddMember(ctx context.Context, member Member) error
	ListMembers(ctx context.Context, groupID string) ([]Member, error)
	// GetGSLBBackupPool 返回流量层切换目标：优先级最高的非活跃池。
	GetGSLBBackupPool(ctx context.Context, serviceID, activePoolID string) (string, bool, error)
	// GetGSLBPrimaryPool 返回回切目标：优先级最低的主池。
	GetGSLBPrimaryPool(ctx context.Context, serviceID string) (string, bool, error)
	GetRunByKey(ctx context.Context, groupID, idempotencyKey string) (SwitchRun, bool, error)
	GetRun(ctx context.Context, id, tenantID string) (SwitchRun, bool, error)
	// CreateRun 同事务写入运行 + 平台 operations/operation_read_model 行 + Outbox 事件。
	CreateRun(ctx context.Context, run SwitchRun, events []OutboxEvent) error
	// UpdateRun 同事务更新运行状态并同步关联 Operation 行。
	UpdateRun(ctx context.Context, id, status string, fields map[string]any, events []OutboxEvent) error
	ListRuns(ctx context.Context, groupID, tenantID string) ([]SwitchRun, error)
	// TrafficRequestStatuses 返回子 gslb 切换请求的当前状态（运行终态聚合）。
	TrafficRequestStatuses(ctx context.Context, requestIDs []string) ([]string, error)
}

type App struct {
	repo Repository
	gslb GSLBSubmitter
}

func NewService(repo Repository, submitter GSLBSubmitter) *App {
	return &App{repo: repo, gslb: submitter}
}

// CreateGroup 创建保护组（dr:create）。
func (a *App) CreateGroup(ctx context.Context, name, primaryRegion, standbyRegion string, trusted iam.TrustedContext) (Group, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionCreate) {
		return Group{}, ErrForbidden
	}
	if name == "" || len(name) > 128 || primaryRegion == "" || standbyRegion == "" {
		return Group{}, fmt.Errorf("%w: name/primaryRegion/standbyRegion required", ErrInvalid)
	}
	now := time.Now().UTC()
	group := Group{
		ID: uuid.NewString(), TenantID: trusted.TenantID, Name: name,
		PrimaryRegion: primaryRegion, StandbyRegion: standbyRegion,
		LifecycleState: "Ready", CreatedAt: now, UpdatedAt: now,
	}
	if err := a.repo.CreateGroup(ctx, group); err != nil {
		return Group{}, err
	}
	return group, nil
}

// ListGroups 返回租户范围内保护组（dr:list）。
func (a *App) ListGroups(ctx context.Context, trusted iam.TrustedContext) ([]Group, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionList) {
		return nil, ErrForbidden
	}
	return a.repo.ListGroups(ctx, trusted.TenantID)
}

// GetGroup 返回组详情（dr:read）。
func (a *App) GetGroup(ctx context.Context, id string, trusted iam.TrustedContext) (GroupDetail, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionRead) {
		return GroupDetail{}, ErrForbidden
	}
	group, ok, err := a.repo.GetGroup(ctx, id, trusted.TenantID)
	if err != nil {
		return GroupDetail{}, err
	}
	if !ok {
		return GroupDetail{}, ErrNotFound
	}
	members, err := a.repo.ListMembers(ctx, id)
	if err != nil {
		return GroupDetail{}, err
	}
	runs, err := a.aggregateRuns(ctx, id, trusted.TenantID)
	if err != nil {
		return GroupDetail{}, err
	}
	return GroupDetail{Group: group, Members: members, Runs: runs}, nil
}

// AddMember 添加组成员（dr:update）；gslb_service 成员引用必须是存在的 GSLB 服务。
func (a *App) AddMember(ctx context.Context, groupID, memberType, refID, name string, trusted iam.TrustedContext) (Member, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionUpdate) {
		return Member{}, ErrForbidden
	}
	if _, ok, err := a.repo.GetGroup(ctx, groupID, trusted.TenantID); err != nil {
		return Member{}, err
	} else if !ok {
		return Member{}, ErrNotFound
	}
	if memberType != MemberGSLBService && memberType != MemberDataLayer {
		return Member{}, fmt.Errorf("%w: unknown memberType %q", ErrInvalid, memberType)
	}
	if refID == "" || name == "" || len(name) > 128 {
		return Member{}, fmt.Errorf("%w: refId/name required", ErrInvalid)
	}
	member := Member{
		ID: uuid.NewString(), GroupID: groupID, MemberType: memberType,
		RefID: refID, Name: name, CreatedAt: time.Now().UTC(),
	}
	if err := a.repo.AddMember(ctx, member); err != nil {
		return Member{}, err
	}
	return member, nil
}

// InitiateSwitch 发起切换链（dr:execute）：幂等创建运行；无数据层成员时立即
// 进入流量层，否则停留 DataLayerPending 等待显式确认（OBS-008 顺序保证）。
func (a *App) InitiateSwitch(ctx context.Context, groupID, direction, reason, idempotencyKey string, trusted iam.TrustedContext) (SwitchRun, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionExecute) {
		return SwitchRun{}, ErrForbidden
	}
	if direction != DirectionFailover && direction != DirectionSwitchback {
		return SwitchRun{}, fmt.Errorf("%w: unknown direction %q", ErrInvalid, direction)
	}
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return SwitchRun{}, fmt.Errorf("%w: idempotencyKey required", ErrInvalid)
	}
	_, ok, err := a.repo.GetGroup(ctx, groupID, trusted.TenantID)
	if err != nil {
		return SwitchRun{}, err
	}
	if !ok {
		return SwitchRun{}, ErrNotFound
	}
	// 幂等：同 key 直接返回既有运行
	if existing, found, err := a.repo.GetRunByKey(ctx, groupID, idempotencyKey); err != nil {
		return SwitchRun{}, err
	} else if found {
		return existing, nil
	}
	// 单活跃运行守卫
	runs, err := a.repo.ListRuns(ctx, groupID, trusted.TenantID)
	if err != nil {
		return SwitchRun{}, err
	}
	for _, r := range runs {
		switch r.Status {
		case RunDataLayerPending, RunDataLayerCompleted, RunTrafficDispatched, RunAwaitingApproval:
			return SwitchRun{}, fmt.Errorf("%w: active run %s in progress", ErrConflict, r.ID)
		}
	}
	members, err := a.repo.ListMembers(ctx, groupID)
	if err != nil {
		return SwitchRun{}, err
	}
	var trafficMembers, dataMembers []Member
	for _, m := range members {
		if m.MemberType == MemberGSLBService {
			trafficMembers = append(trafficMembers, m)
		} else {
			dataMembers = append(dataMembers, m)
		}
	}
	if len(trafficMembers) == 0 {
		return SwitchRun{}, fmt.Errorf("%w: group has no traffic-layer (gslb_service) member", ErrInvalid)
	}

	now := time.Now().UTC()
	run := SwitchRun{
		ID: uuid.NewString(), GroupID: groupID, TenantID: trusted.TenantID,
		Direction: direction, Status: RunDataLayerPending,
		IdempotencyKey: idempotencyKey, CorrelationID: uuid.NewString(),
		OperationID: uuid.NewString(), TrafficRequestIDs: []string{},
		Reason: reason, ActorID: trusted.SubjectID, CreatedAt: now, UpdatedAt: now,
	}
	events := []OutboxEvent{a.newEvent(EventSwitchInitiated, run, trusted, map[string]any{
		"direction": direction, "trafficMembers": len(trafficMembers), "dataLayerMembers": len(dataMembers),
	})}
	if len(dataMembers) == 0 {
		run.Status = RunDataLayerCompleted
	}
	if err := a.repo.CreateRun(ctx, run, events); err != nil {
		return SwitchRun{}, err
	}
	if run.Status == RunDataLayerCompleted {
		return a.dispatchTraffic(ctx, run, trafficMembers, trusted)
	}
	return run, nil
}

// ConfirmDataLayer 数据层切换显式确认（dr:update）：确认后推进流量层步骤。
func (a *App) ConfirmDataLayer(ctx context.Context, runID string, trusted iam.TrustedContext) (SwitchRun, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionUpdate) {
		return SwitchRun{}, ErrForbidden
	}
	run, ok, err := a.repo.GetRun(ctx, runID, trusted.TenantID)
	if err != nil {
		return SwitchRun{}, err
	}
	if !ok {
		return SwitchRun{}, ErrNotFound
	}
	if run.Status != RunDataLayerPending {
		return SwitchRun{}, fmt.Errorf("%w: run not awaiting data-layer confirmation (status %s)", ErrInvalid, run.Status)
	}
	members, err := a.repo.ListMembers(ctx, run.GroupID)
	if err != nil {
		return SwitchRun{}, err
	}
	var trafficMembers []Member
	for _, m := range members {
		if m.MemberType == MemberGSLBService {
			trafficMembers = append(trafficMembers, m)
		}
	}
	run.Status = RunDataLayerCompleted
	if err := a.repo.UpdateRun(ctx, run.ID, RunDataLayerCompleted, nil, []OutboxEvent{
		a.newEvent(EventDataLayerConfirmed, run, trusted, nil),
	}); err != nil {
		return SwitchRun{}, err
	}
	return a.dispatchTraffic(ctx, run, trafficMembers, trusted)
}

// dispatchTraffic 为每个流量层成员提交携带 drGroupRef 的 gslb 受控意图
// （OBS-007 对接缝：审批门控由 gslb 链路保证，DR 回切强制审批）。
func (a *App) dispatchTraffic(ctx context.Context, run SwitchRun, trafficMembers []Member, trusted iam.TrustedContext) (SwitchRun, error) {
	requestIDs := append([]string(nil), run.TrafficRequestIDs...)
	for _, member := range trafficMembers {
		intent, err := a.buildTrafficIntent(ctx, run, member, trusted)
		if err != nil {
			return a.failRun(ctx, run, trusted, err.Error())
		}
		body, err := json.Marshal(intent)
		if err != nil {
			return a.failRun(ctx, run, trusted, err.Error())
		}
		request, err := a.gslb.SubmitIntent(ctx, body, member.RefID, trusted)
		if err != nil {
			return a.failRun(ctx, run, trusted, fmt.Sprintf("traffic member %s: %v", member.RefID, err))
		}
		requestIDs = append(requestIDs, request.ID)
	}
	status := RunAwaitingApproval
	if err := a.repo.UpdateRun(ctx, run.ID, status, map[string]any{
		"traffic_request_ids": requestIDs,
	}, []OutboxEvent{a.newEvent(EventTrafficDispatched, run, trusted, map[string]any{
		"trafficRequestIds": requestIDs,
	})}); err != nil {
		return SwitchRun{}, err
	}
	run.Status = status
	run.TrafficRequestIDs = requestIDs
	return run, nil
}

// buildTrafficIntent 构造流量层 gslb 意图：failover → 优先级最高的非活跃池；
// switchback → 优先级最低的主池；均携带 drGroupRef（组 ID）。
func (a *App) buildTrafficIntent(ctx context.Context, run SwitchRun, member Member, trusted iam.TrustedContext) (*gslb.Intent, error) {
	var kind gslb.IntentKind
	var targetPoolID string
	var ok bool
	var err error
	if run.Direction == DirectionFailover {
		kind = gslb.IntentFailover
		// 活跃池由 gslb 侧在服务主档中维护；编排器不假设其值，交由仓储解析。
		targetPoolID, ok, err = a.repo.GetGSLBBackupPool(ctx, member.RefID, "")
	} else {
		kind = gslb.IntentSwitchback
		targetPoolID, ok, err = a.repo.GetGSLBPrimaryPool(ctx, member.RefID)
	}
	if err != nil {
		return nil, err
	}
	if !ok || targetPoolID == "" {
		return nil, fmt.Errorf("%w: no target pool for gslb service %s", ErrInvalid, member.RefID)
	}
	return &gslb.Intent{
		APIVersion:   gslb.APIVersion,
		Kind:         kind,
		ServiceID:    member.RefID,
		TenantID:     trusted.TenantID,
		TargetPoolID: targetPoolID,
		Reason:       fmt.Sprintf("dr-%s: %s", run.Direction, run.Reason),
		DRGroupRef:   run.GroupID,
		Metadata: gslb.IntentMetadata{
			// 每成员幂等键：运行幂等键 + 成员引用，重试安全
			IdempotencyKey: fmt.Sprintf("%s-%s", run.IdempotencyKey, member.RefID),
			CorrelationID:  run.CorrelationID,
		},
	}, nil
}

// failRun 将运行置为 Failed 并记录原因。
func (a *App) failRun(ctx context.Context, run SwitchRun, trusted iam.TrustedContext, message string) (SwitchRun, error) {
	if err := a.repo.UpdateRun(ctx, run.ID, RunFailed, map[string]any{"error": message}, []OutboxEvent{
		a.newEvent(EventRunStatusChanged, run, trusted, map[string]any{"status": RunFailed, "error": message}),
	}); err != nil {
		return SwitchRun{}, err
	}
	run.Status = RunFailed
	run.Error = message
	return run, nil
}

// ListRuns 返回组的切换运行（终态按子流量请求聚合推导）。
func (a *App) ListRuns(ctx context.Context, groupID string, trusted iam.TrustedContext) ([]SwitchRun, error) {
	if !hasPermission(trusted, string(iam.ResourceDR), iam.ActionRead) {
		return nil, ErrForbidden
	}
	if _, ok, err := a.repo.GetGroup(ctx, groupID, trusted.TenantID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotFound
	}
	return a.aggregateRuns(ctx, groupID, trusted.TenantID)
}

// aggregateRuns 读取运行并按子 gslb 请求终态聚合推导运行终态：
// 全部 Succeeded → Completed；任一 Failed/Rejected → Failed。
func (a *App) aggregateRuns(ctx context.Context, groupID, tenantID string) ([]SwitchRun, error) {
	runs, err := a.repo.ListRuns(ctx, groupID, tenantID)
	if err != nil {
		return nil, err
	}
	for i, run := range runs {
		if run.Status != RunAwaitingApproval && run.Status != RunTrafficDispatched {
			continue
		}
		statuses, err := a.repo.TrafficRequestStatuses(ctx, run.TrafficRequestIDs)
		if err != nil {
			return nil, err
		}
		if len(statuses) == 0 || len(statuses) != len(run.TrafficRequestIDs) {
			continue
		}
		succeeded, failed := 0, 0
		for _, s := range statuses {
			switch s {
			case gslbapp.StatusSucceeded:
				succeeded++
			case gslbapp.StatusFailed, gslbapp.StatusRejected:
				failed++
			}
		}
		if failed > 0 {
			runs[i].Status = RunFailed
		} else if succeeded == len(statuses) {
			runs[i].Status = RunCompleted
		}
	}
	return runs, nil
}

func (a *App) newEvent(subject string, run SwitchRun, trusted iam.TrustedContext, extra map[string]any) OutboxEvent {
	payload := map[string]any{
		"runId":       run.ID,
		"groupId":     run.GroupID,
		"operationId": run.OperationID,
	}
	for k, v := range extra {
		payload[k] = v
	}
	return OutboxEvent{
		MessageID:      uuid.NewString(),
		MessageType:    subject,
		SchemaVersion:  "v1",
		Subject:        subject,
		TenantID:       trusted.TenantID,
		ActorID:        trusted.SubjectID,
		CorrelationID:  run.CorrelationID,
		IdempotencyKey: fmt.Sprintf("dr-%s-%s", run.ID, subject),
		AggregateID:    run.GroupID,
		Payload:        payload,
	}
}

func hasPermission(trusted iam.TrustedContext, resource string, action iam.AuthorizationAction) bool {
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID == trusted.TenantID && permission.Action == action && (permission.ResourceKind == resource || permission.ResourceKind == "*") {
			return true
		}
	}
	return false
}
