package gslb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/gslb"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

var (
	ErrNotFound  = errors.New("gslb service not found")
	ErrForbidden = errors.New("gslb forbidden")
	ErrInvalid   = errors.New("gslb invalid")
	// ErrConflict 表示状态/版本冲突（切换中、If-Match 过期、active 池将被移除、
	// 存在进行中切换请求、名称冲突等），handler 映射为 409。
	ErrConflict = errors.New("gslb conflict")
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
	ID              string `json:"id"`
	TenantID        string `json:"tenantId"`
	Name            string `json:"name"`
	Domain          string `json:"domain"`
	RoutingMode     string `json:"routingMode"`
	ActivePoolID    string `json:"activePoolId,omitempty"`
	LifecycleState  string `json:"lifecycleState"`
	RequireApproval bool   `json:"requireApproval"`
	// HealthCheck 是服务级健康检查模板（迁移 094）。
	HealthCheck HealthCheck `json:"healthCheck"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

// HealthCheck 是服务级健康检查模板（迁移 094）：controller 探测成员集群时
// 按服务读取；同一集群被多服务引用时 controller 取最严格配置。
type HealthCheck struct {
	Protocol         string `json:"protocol"`
	Path             string `json:"path"`
	IntervalSeconds  int    `json:"intervalSeconds"`
	TimeoutSeconds   int    `json:"timeoutSeconds"`
	FailureThreshold int    `json:"failureThreshold"`
}

// DefaultHealthCheck 与迁移 094 列默认值一致。
func DefaultHealthCheck() HealthCheck {
	return HealthCheck{Protocol: "http", Path: "/healthz", IntervalSeconds: 10, TimeoutSeconds: 3, FailureThreshold: 3}
}

// validate 校验健康检查模板（与迁移 094 CHECK 一致）。
func (h HealthCheck) validate() error {
	if h.Protocol != "http" && h.Protocol != "https" {
		return fmt.Errorf("%w: healthCheck.protocol must be http or https", ErrInvalid)
	}
	if !strings.HasPrefix(h.Path, "/") {
		return fmt.Errorf("%w: healthCheck.path must start with /", ErrInvalid)
	}
	if h.IntervalSeconds < 5 || h.IntervalSeconds > 300 {
		return fmt.Errorf("%w: healthCheck.intervalSeconds must be 5-300", ErrInvalid)
	}
	if h.TimeoutSeconds < 1 || h.TimeoutSeconds > 30 {
		return fmt.Errorf("%w: healthCheck.timeoutSeconds must be 1-30", ErrInvalid)
	}
	if h.FailureThreshold < 1 || h.FailureThreshold > 10 {
		return fmt.Errorf("%w: healthCheck.failureThreshold must be 1-10", ErrInvalid)
	}
	return nil
}

// ReadModel 是 GSLB 只读投影（GSLB-007）：查询只读本快照。
type ReadModel struct {
	ServiceID         string   `json:"serviceId"`
	TenantID          string   `json:"tenantId"`
	Domain            string   `json:"domain"`
	ActivePoolID      string   `json:"activePoolId,omitempty"`
	LifecycleState    string   `json:"lifecycleState"`
	HealthyPools      []string `json:"healthyPools"`
	CurrentDNSTargets []string `json:"currentDnsTargets"`
	// HealthCheck 来自 gslb_services 主档（join 读取，不复制进投影表）。
	HealthCheck HealthCheck `json:"healthCheck"`
	// 最近演练结果（GSLB-010：演练报告写入 Read Model）
	LastDrillReportID string     `json:"lastDrillReportId,omitempty"`
	LastDrillVerdict  string     `json:"lastDrillVerdict,omitempty"`
	LastDrillAt       *time.Time `json:"lastDrillAt,omitempty"`
	ObservedAt        time.Time  `json:"observedAt"`
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
	OperationID string `json:"operationId,omitempty"`
	// DRGroupRef 是 DRProtectionGroup 编排来源引用（GSLB-009 对接缝）。
	DRGroupRef string     `json:"drGroupRef,omitempty"`
	ActorID    string     `json:"actorId,omitempty"`
	ApprovedBy string     `json:"approvedBy,omitempty"`
	ApprovedAt *time.Time `json:"approvedAt,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
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

// PoolMember 是地址池成员（集群 + 权重）。
type PoolMember struct {
	ClusterID string `json:"clusterId"`
	Weight    int    `json:"weight"`
	Enabled   bool   `json:"enabled"`
	Healthy   bool   `json:"healthy"`
}

// Pool 是服务的地址池拓扑视图：isActive 对照服务 active_pool_id，
// healthy 对照 Read Model 的 healthy_pools（投影缺失时 false）。
type Pool struct {
	ID        string       `json:"id"`
	PoolID    string       `json:"poolId,omitempty"`
	ServiceID string       `json:"serviceId"`
	Name      string       `json:"name"`
	Priority  int          `json:"priority"`
	IsActive  bool         `json:"isActive"`
	Healthy   bool         `json:"healthy"`
	Members   []PoolMember `json:"members"`
}

// CreateServiceInput 是服务创建 API 的输入（GSLB 三期）。
type CreateServiceInput struct {
	Name            string       `json:"name"`
	Domain          string       `json:"domain"`
	RequireApproval bool         `json:"requireApproval"`
	Pools           []Pool       `json:"pools"`
	HealthCheck     *HealthCheck `json:"healthCheck,omitempty"`
}

// UpdateServiceInput 是服务编辑（PATCH）的输入：仅名称、审批策略与健康检查
// 模板可变；domain/routingMode/lifecycleState 不可由该路径修改。
type UpdateServiceInput struct {
	Name            *string      `json:"name,omitempty"`
	RequireApproval *bool        `json:"requireApproval,omitempty"`
	HealthCheck     *HealthCheck `json:"healthCheck,omitempty"`
}

// ReplacePoolsInput 是地址池整组替换（PUT）的输入：与创建时的 pools 结构一致，
// 池以 name 为身份（服务端重新生成 pool id），active 池按名字保留。
type ReplacePoolsInput struct {
	Pools []Pool `json:"pools"`
}

// ServiceDetail 是创建/详情响应：服务主档（含健康检查模板）+ 地址池拓扑。
type ServiceDetail struct {
	Service
	Pools []Pool `json:"pools"`
}

var (
	gslbNamePattern   = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	gslbDomainPattern = regexp.MustCompile(`^(\*\.)?[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?(\.[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)+$`)
)

// drillReportPayload 是 report JSONB 的结构。
type drillReportPayload struct {
	ServiceID        string         `json:"serviceId"`
	Domain           string         `json:"domain"`
	ActivePoolID     string         `json:"activePoolId,omitempty"`
	TargetPoolID     string         `json:"targetPoolId,omitempty"`
	CurrentTargets   []string       `json:"currentTargets"`
	ProjectedTargets []string       `json:"projectedTargets"`
	ProjectedWeights map[string]int `json:"projectedWeights,omitempty"`
	HealthyPools     []string       `json:"healthyPools,omitempty"`
	Checks           []DrillCheck   `json:"checks"`
	Verdict          string         `json:"verdict"`
	GeneratedAt      time.Time      `json:"generatedAt"`
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
	// GetServiceByDomain 按租户 + 入口域名解析服务（创建幂等）。
	GetServiceByDomain(ctx context.Context, tenantID, domain string) (Service, bool, error)
	// ExistingTenantClusters 返回给定集群名中存在于该租户 runtime targets 的子集。
	ExistingTenantClusters(ctx context.Context, tenantID string, clusterIDs []string) (map[string]bool, error)
	// CreateService 同事务创建服务 + 地址池 + 成员（含权重），并钉死 active 池。
	CreateService(ctx context.Context, svc Service, health HealthCheck, pools []Pool) error
	// UpdateService 以 updated_at 乐观并发更新可变字段（name/requireApproval/
	// 健康检查模板）；expectedUpdatedAt 不匹配（并发修改）时返回 ok=false。
	UpdateService(ctx context.Context, svc Service, expectedUpdatedAt time.Time) (Service, bool, error)
	// ReplaceServicePools 事务内整组替换地址池：CAS 校验版本 → 删旧池（级联成员）
	// → 建新池/成员 → 按 newActivePoolID 重指 active 池 → 同步 Read Model 池引用。
	ReplaceServicePools(ctx context.Context, serviceID, tenantID string, pools []Pool, newActivePoolID string, expectedUpdatedAt time.Time) (bool, error)
	// DeleteService 删除服务主档；池/成员/Read Model/切换请求/演练报告经
	// ON DELETE CASCADE 清理（迁移 081/082）。
	DeleteService(ctx context.Context, id, tenantID string) (bool, error)
	// HasInflightSwitchRequests 报告服务是否存在进行中（未终态）的切换请求。
	HasInflightSwitchRequests(ctx context.Context, serviceID, tenantID string) (bool, error)
	GetPoolMemberClusterIDs(ctx context.Context, poolID string) ([]string, error)
	GetSwitchRequestByKey(ctx context.Context, tenantID, serviceID, idempotencyKey string) (SwitchRequest, bool, error)
	GetSwitchRequest(ctx context.Context, id, tenantID string) (SwitchRequest, bool, error)
	// ListPools 返回服务的地址池及成员（按 priority 升序，成员按 cluster_id 排序）。
	ListPools(ctx context.Context, serviceID, tenantID string) ([]Pool, error)
	// ListSwitchRequests 返回服务维度租户隔离的切换请求（按 created_at 倒序）。
	ListSwitchRequests(ctx context.Context, tenantID, serviceID, status string) ([]SwitchRequest, error)
	// CreateSwitchRequest 同事务写入：切换请求 + 平台 operations/operation_steps
	// 行（Operation Center 统一接线）+ 演练报告（drill 非空时）+ Outbox 事件。
	CreateSwitchRequest(ctx context.Context, req SwitchRequest, drill *DrillReport, events []OutboxEvent) error
	// UpdateSwitchRequestStatus 同事务更新请求状态并同步关联 Operation 行。
	UpdateSwitchRequestStatus(ctx context.Context, id, status string, fields map[string]any, events []OutboxEvent) error
	// CompleteSwitchRequest 将已批准请求按不可变计划完成投影闭环；不触达 DNS 数据面。
	CompleteSwitchRequest(ctx context.Context, id string) error
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

// CreateService 创建 GSLB 服务（GSLB 三期）：校验 → 幂等检查（同租户同
// domain 返回既有服务，created=false）→ 同事务落库服务/池/成员。
func (a *App) CreateService(ctx context.Context, input CreateServiceInput, trusted iam.TrustedContext) (ServiceDetail, bool, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionCreate) {
		return ServiceDetail{}, false, ErrForbidden
	}
	health := DefaultHealthCheck()
	if input.HealthCheck != nil {
		health = *input.HealthCheck
	}
	if err := health.validate(); err != nil {
		return ServiceDetail{}, false, err
	}
	if !gslbNamePattern.MatchString(input.Name) {
		return ServiceDetail{}, false, fmt.Errorf("%w: name must be a dns label", ErrInvalid)
	}
	if len(input.Domain) > 253 || !gslbDomainPattern.MatchString(input.Domain) {
		return ServiceDetail{}, false, fmt.Errorf("%w: domain must be a valid dns name", ErrInvalid)
	}
	clusterIDs, err := validatePools(input.Pools)
	if err != nil {
		return ServiceDetail{}, false, err
	}
	if err := a.ensureTenantClusters(ctx, trusted.TenantID, clusterIDs); err != nil {
		return ServiceDetail{}, false, err
	}

	// 幂等：同租户同 domain 重复创建返回既有服务（200）
	if svc, found, err := a.repo.GetServiceByDomain(ctx, trusted.TenantID, input.Domain); err != nil {
		return ServiceDetail{}, false, err
	} else if found {
		pools, err := a.repo.ListPools(ctx, svc.ID, trusted.TenantID)
		if err != nil {
			return ServiceDetail{}, false, err
		}
		return ServiceDetail{Service: svc, Pools: pools}, false, nil
	}

	now := time.Now().UTC()
	service := Service{
		ID:              uuid.NewString(),
		TenantID:        trusted.TenantID,
		Name:            input.Name,
		Domain:          input.Domain,
		RoutingMode:     "dns",
		LifecycleState:  "Active",
		RequireApproval: input.RequireApproval,
		HealthCheck:     health,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	pools := make([]Pool, 0, len(input.Pools))
	for _, p := range input.Pools {
		pools = append(pools, Pool{
			ID: uuid.NewString(), Name: p.Name, Priority: p.Priority, Members: p.Members,
		})
	}
	// 最小 priority 的池为初始 active 池
	active := pools[0]
	for _, p := range pools[1:] {
		if p.Priority < active.Priority {
			active = p
		}
	}
	service.ActivePoolID = active.ID
	if err := a.repo.CreateService(ctx, service, health, pools); err != nil {
		return ServiceDetail{}, false, err
	}
	for i := range pools {
		pools[i].IsActive = pools[i].ID == active.ID
	}
	return ServiceDetail{Service: service, Pools: pools}, true, nil
}

// validatePools 校验地址池配置（创建与整组替换共用）：≥1 池、名字非空、
// priority 唯一、成员非空、weight 0-100；返回引用的集群名集合。
func validatePools(pools []Pool) (map[string]bool, error) {
	if len(pools) == 0 {
		return nil, fmt.Errorf("%w: at least one pool required", ErrInvalid)
	}
	priorities := map[int]bool{}
	clusterIDs := map[string]bool{}
	for i, pool := range pools {
		if pool.Name == "" {
			return nil, fmt.Errorf("%w: pools[%d].name required", ErrInvalid, i)
		}
		if priorities[pool.Priority] {
			return nil, fmt.Errorf("%w: duplicate pool priority %d", ErrInvalid, pool.Priority)
		}
		priorities[pool.Priority] = true
		if len(pool.Members) == 0 {
			return nil, fmt.Errorf("%w: pool %q requires members", ErrInvalid, pool.Name)
		}
		for _, member := range pool.Members {
			if member.ClusterID == "" || member.Weight < 0 || member.Weight > 100 {
				return nil, fmt.Errorf("%w: invalid member %q weight %d", ErrInvalid, member.ClusterID, member.Weight)
			}
			clusterIDs[member.ClusterID] = true
		}
	}
	return clusterIDs, nil
}

// ensureTenantClusters 校验所有引用集群存在于当前租户 runtime targets。
func (a *App) ensureTenantClusters(ctx context.Context, tenantID string, clusterIDs map[string]bool) error {
	ids := make([]string, 0, len(clusterIDs))
	for id := range clusterIDs {
		ids = append(ids, id)
	}
	existing, err := a.repo.ExistingTenantClusters(ctx, tenantID, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !existing[id] {
			return fmt.Errorf("%w: cluster %q not found in tenant", ErrInvalid, id)
		}
	}
	return nil
}

// checkEditable 读取服务并施加编辑前置条件：租户隔离 404、FailingOver 409、
// If-Match 版本比对（携带时）。
func (a *App) checkEditable(ctx context.Context, id string, ifMatch *time.Time, trusted iam.TrustedContext) (Service, error) {
	service, ok, err := a.repo.GetService(ctx, id, trusted.TenantID)
	if err != nil {
		return Service{}, err
	}
	if !ok {
		return Service{}, ErrNotFound
	}
	if service.LifecycleState == "FailingOver" {
		return Service{}, fmt.Errorf("%w: service is failing over, edit after the switch settles", ErrConflict)
	}
	if ifMatch != nil && !ifMatch.Equal(service.UpdatedAt) {
		return Service{}, fmt.Errorf("%w: If-Match does not match current updatedAt", ErrConflict)
	}
	return service, nil
}

// UpdateService 编辑服务可变字段（name/requireApproval/healthCheck）。
// 幂等：合并后与现状一致时不落库，直接返回当前详情（200）。
func (a *App) UpdateService(ctx context.Context, id string, input UpdateServiceInput, ifMatch *time.Time, trusted iam.TrustedContext) (ServiceDetail, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionUpdate) {
		return ServiceDetail{}, ErrForbidden
	}
	service, err := a.checkEditable(ctx, id, ifMatch, trusted)
	if err != nil {
		return ServiceDetail{}, err
	}
	merged := service
	changed := false
	if input.Name != nil {
		if !gslbNamePattern.MatchString(*input.Name) {
			return ServiceDetail{}, fmt.Errorf("%w: name must be a dns label", ErrInvalid)
		}
		if *input.Name != service.Name {
			merged.Name = *input.Name
			changed = true
		}
	}
	if input.RequireApproval != nil && *input.RequireApproval != service.RequireApproval {
		merged.RequireApproval = *input.RequireApproval
		changed = true
	}
	if input.HealthCheck != nil {
		if err := input.HealthCheck.validate(); err != nil {
			return ServiceDetail{}, err
		}
		if *input.HealthCheck != service.HealthCheck {
			merged.HealthCheck = *input.HealthCheck
			changed = true
		}
	}
	if changed {
		updated, ok, err := a.repo.UpdateService(ctx, merged, service.UpdatedAt)
		if err != nil {
			return ServiceDetail{}, err
		}
		if !ok {
			return ServiceDetail{}, fmt.Errorf("%w: service was modified concurrently", ErrConflict)
		}
		merged = updated
	}
	pools, err := a.ListPools(ctx, merged.ID, trusted.TenantID)
	if err != nil {
		return ServiceDetail{}, err
	}
	return ServiceDetail{Service: merged, Pools: pools}, nil
}

// ReplaceServicePools 整组替换地址池配置（校验同创建）。池以 name 为身份：
// 当前 active 池的名字必须仍在新配置中，否则 409（先 failover 再删池）。
// 幂等：新配置与现状语义一致（同名同 priority 同成员权重）时不落库。
func (a *App) ReplaceServicePools(ctx context.Context, id string, input ReplacePoolsInput, ifMatch *time.Time, trusted iam.TrustedContext) ([]Pool, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionUpdate) {
		return nil, ErrForbidden
	}
	service, err := a.checkEditable(ctx, id, ifMatch, trusted)
	if err != nil {
		return nil, err
	}
	clusterIDs, err := validatePools(input.Pools)
	if err != nil {
		return nil, err
	}
	if err := a.ensureTenantClusters(ctx, trusted.TenantID, clusterIDs); err != nil {
		return nil, err
	}
	current, err := a.repo.ListPools(ctx, id, trusted.TenantID)
	if err != nil {
		return nil, err
	}
	activeName := ""
	for _, pool := range current {
		if service.ActivePoolID != "" && pool.ID == service.ActivePoolID {
			activeName = pool.Name
			break
		}
	}
	newNames := map[string]bool{}
	for _, pool := range input.Pools {
		newNames[pool.Name] = true
	}
	if activeName != "" && !newNames[activeName] {
		return nil, fmt.Errorf("%w: active pool %q would be removed; fail over to a retained pool first", ErrConflict, activeName)
	}
	if poolsEquivalent(current, input.Pools) {
		return a.ListPools(ctx, id, trusted.TenantID)
	}
	pools := make([]Pool, 0, len(input.Pools))
	newActiveID := ""
	for _, p := range input.Pools {
		pool := Pool{ID: uuid.NewString(), Name: p.Name, Priority: p.Priority, Members: p.Members}
		if pool.Name == activeName {
			newActiveID = pool.ID
		}
		pools = append(pools, pool)
	}
	ok, err := a.repo.ReplaceServicePools(ctx, id, trusted.TenantID, pools, newActiveID, service.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: service was modified concurrently", ErrConflict)
	}
	return a.ListPools(ctx, id, trusted.TenantID)
}

// poolsEquivalent 比较现有池与目标配置是否语义一致（按名字对齐，
// 比较 priority 与成员 cluster→weight 多重集）。
func poolsEquivalent(current, desired []Pool) bool {
	if len(current) != len(desired) {
		return false
	}
	byName := make(map[string]Pool, len(current))
	for _, pool := range current {
		byName[pool.Name] = pool
	}
	for _, want := range desired {
		existing, ok := byName[want.Name]
		if !ok || existing.Priority != want.Priority || len(existing.Members) != len(want.Members) {
			return false
		}
		weights := map[string]int{}
		for _, member := range existing.Members {
			weights[member.ClusterID] += member.Weight
		}
		for _, member := range want.Members {
			weights[member.ClusterID] -= member.Weight
		}
		for _, weight := range weights {
			if weight != 0 {
				return false
			}
		}
	}
	return true
}

// DeleteService 删除服务（危险操作，需 gslb:delete）：仅 Inactive/Paused 可直接删；
// 其余状态须显式 force=true。两种路径都要求无进行中（未终态）的切换请求。
// 池/成员/Read Model/切换历史/演练报告经外键 CASCADE 同事务清理。
func (a *App) DeleteService(ctx context.Context, id string, force bool, trusted iam.TrustedContext) error {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionDelete) {
		return ErrForbidden
	}
	service, ok, err := a.repo.GetService(ctx, id, trusted.TenantID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	switch service.LifecycleState {
	case "Inactive", "Paused":
	default:
		if !force {
			return fmt.Errorf("%w: service is %s; only Inactive/Paused services can be deleted (or pass force=true)", ErrConflict, service.LifecycleState)
		}
	}
	inflight, err := a.repo.HasInflightSwitchRequests(ctx, id, trusted.TenantID)
	if err != nil {
		return err
	}
	if inflight {
		return fmt.Errorf("%w: service has in-flight switch requests; wait for completion or reject them first", ErrConflict)
	}
	deleted, err := a.repo.DeleteService(ctx, id, trusted.TenantID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
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

// ListPools 返回服务的地址池拓扑（租户隔离）：store 读取池与成员，
// isActive 对照服务 active_pool_id，healthy 对照 Read Model healthy_pools。
func (a *App) ListPools(ctx context.Context, serviceID, tenantID string) ([]Pool, error) {
	service, ok, err := a.repo.GetService(ctx, serviceID, tenantID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	pools, err := a.repo.ListPools(ctx, serviceID, tenantID)
	if err != nil {
		return nil, err
	}
	healthyPools := map[string]bool{}
	if model, ok, err := a.repo.GetReadModel(ctx, serviceID, tenantID); err != nil {
		return nil, err
	} else if ok {
		for _, poolID := range model.HealthyPools {
			healthyPools[poolID] = true
		}
	}
	for i := range pools {
		pools[i].PoolID = pools[i].ID
		pools[i].ServiceID = service.ID
		pools[i].IsActive = service.ActivePoolID != "" && pools[i].ID == service.ActivePoolID
		pools[i].Healthy = healthyPools[pools[i].ID]
		if pools[i].Members == nil {
			pools[i].Members = []PoolMember{}
		}
		for j := range pools[i].Members {
			pools[i].Members[j].Enabled = true
		}
	}
	if pools == nil {
		pools = []Pool{}
	}
	return pools, nil
}

// 合法切换请求状态过滤值（与迁移 081 CHECK 及 openapi GslbSwitchRequest.status 一致）
var validSwitchStatuses = map[string]bool{
	StatusPendingApproval: true,
	StatusApproved:        true,
	StatusRejected:        true,
	StatusDispatched:      true,
	StatusSucceeded:       true,
	StatusFailed:          true,
	StatusDrillCompleted:  true,
}

// ListSwitchRequests 返回服务维度租户隔离的切换请求列表（审批中心/切换记录页签）。
// service_id 必须指向当前租户的服务，防止跨租户枚举。
func (a *App) ListSwitchRequests(ctx context.Context, serviceID, status string, trusted iam.TrustedContext) ([]SwitchRequest, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionRead) &&
		!hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionList) &&
		!hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionUpdate) {
		return nil, ErrForbidden
	}
	if status != "" && !validSwitchStatuses[status] {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, status)
	}
	if _, ok, err := a.repo.GetService(ctx, serviceID, trusted.TenantID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotFound
	}
	requests, err := a.repo.ListSwitchRequests(ctx, trusted.TenantID, serviceID, status)
	if err != nil {
		return nil, err
	}
	if requests == nil {
		requests = []SwitchRequest{}
	}
	return requests, nil
}

// GetSwitchRequest 返回租户范围内的切换请求状态，供控制台轮询执行闭环。
func (a *App) GetSwitchRequest(ctx context.Context, requestID string, trusted iam.TrustedContext) (SwitchRequest, error) {
	if !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionRead) && !hasPermission(trusted, string(iam.ResourceGSLB), iam.ActionUpdate) {
		return SwitchRequest{}, ErrForbidden
	}
	request, ok, err := a.repo.GetSwitchRequest(ctx, requestID, trusted.TenantID)
	if err != nil {
		return SwitchRequest{}, err
	}
	if !ok {
		return SwitchRequest{}, ErrNotFound
	}
	return request, nil
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
	if toStatus == StatusApproved && dispatch {
		if err := a.repo.CompleteSwitchRequest(ctx, requestID); err != nil {
			return SwitchRequest{}, err
		}
		request.Status = StatusSucceeded
	}
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
	// weight-update：计划必须钉死权重作用的成员集合（活跃池成员），否则执行器
	// 只有权重没有目标，DNS 记录无法构造（权重链路闭环，GSLB 三期）。
	if intent.Kind == gslb.IntentWeightUpdate {
		if service.ActivePoolID == "" {
			return nil, fmt.Errorf("%w: weight-update requires an active pool", ErrInvalid)
		}
		targets := input.PreviousTargets
		if service.ActivePoolID == intent.TargetPoolID {
			targets = input.Targets
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("%w: weight-update active pool has no enabled members", ErrInvalid)
		}
		memberSet := map[string]bool{}
		for _, member := range targets {
			memberSet[member] = true
		}
		for member := range intent.Weights {
			if !memberSet[member] {
				return nil, fmt.Errorf("%w: weight-update member %q not in active pool", ErrInvalid, member)
			}
		}
		input.Targets = targets
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
			"requestId":  requestID,
			"serviceId":  intent.ServiceID,
			"kind":       string(intent.Kind),
			"digest":     intent.SemanticDigest(),
			"tenantId":   trusted.TenantID,
			"drGroupRef": intent.DRGroupRef,
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
