package gslb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	appgslb "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
	"github.com/F31/hnb/pkg/gslb"
)

// PostgresStore 访问 GSLB 平台状态（迁移 081）。
type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

// serviceColumns 含迁移 094 的健康检查模板列。
const serviceColumns = `
		id, tenant_id, name, domain, routing_mode,
		COALESCE(active_pool_id::text, ''), lifecycle_state, require_approval,
		health_protocol, health_path, health_interval_seconds,
		health_timeout_seconds, health_failure_threshold,
		created_at, updated_at`

func scanService(scanner rowScanner) (appgslb.Service, error) {
	var svc appgslb.Service
	err := scanner.Scan(&svc.ID, &svc.TenantID, &svc.Name, &svc.Domain, &svc.RoutingMode,
		&svc.ActivePoolID, &svc.LifecycleState, &svc.RequireApproval,
		&svc.HealthCheck.Protocol, &svc.HealthCheck.Path, &svc.HealthCheck.IntervalSeconds,
		&svc.HealthCheck.TimeoutSeconds, &svc.HealthCheck.FailureThreshold,
		&svc.CreatedAt, &svc.UpdatedAt)
	return svc, err
}

func (s *PostgresStore) GetService(ctx context.Context, id, tenantID string) (appgslb.Service, bool, error) {
	svc, err := scanService(s.db.QueryRowContext(ctx, `
		SELECT `+serviceColumns+`
		FROM gslb_services
		WHERE id = $1 AND tenant_id = $2`, id, tenantID))
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.Service{}, false, nil
	}
	if err != nil {
		return appgslb.Service{}, false, fmt.Errorf("gslb get service: %w", err)
	}
	return svc, true, nil
}

// GetServiceByDomain 按租户 + 入口域名解析服务（创建幂等；迁移 094 唯一索引兜底）。
func (s *PostgresStore) GetServiceByDomain(ctx context.Context, tenantID, domain string) (appgslb.Service, bool, error) {
	svc, err := scanService(s.db.QueryRowContext(ctx, `
		SELECT `+serviceColumns+`
		FROM gslb_services
		WHERE tenant_id = $1 AND domain = $2`, tenantID, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.Service{}, false, nil
	}
	if err != nil {
		return appgslb.Service{}, false, fmt.Errorf("gslb get service by domain: %w", err)
	}
	return svc, true, nil
}

// ExistingTenantClusters 返回给定集群名中存在于该租户 runtime_targets 表的子集。
// 池成员 clusterId 仍按 target name 传递，与 gslb-controller 探测目标命名一致。
func (s *PostgresStore) ExistingTenantClusters(ctx context.Context, tenantID string, clusterIDs []string) (map[string]bool, error) {
	existing := map[string]bool{}
	if len(clusterIDs) == 0 {
		return existing, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT name FROM runtime_targets
		WHERE tenant_id = $1 AND name = ANY($2) AND target_type = 'kubernetes' AND is_active`,
		tenantID, pq.Array(clusterIDs))
	if err != nil {
		return nil, fmt.Errorf("gslb tenant clusters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		existing[name] = true
	}
	return existing, rows.Err()
}

// CreateService 同事务创建服务（含健康检查模板）+ 地址池 + 成员（含权重）。
// svc.ActivePoolID 由应用层钉死（最小 priority 池），store 只负责落库。
func (s *PostgresStore) CreateService(ctx context.Context, svc appgslb.Service, health appgslb.HealthCheck, pools []appgslb.Pool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_services (
			id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval,
			health_protocol, health_path, health_interval_seconds,
			health_timeout_seconds, health_failure_threshold,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		svc.ID, svc.TenantID, svc.Name, svc.Domain, svc.RoutingMode, svc.LifecycleState, svc.RequireApproval,
		health.Protocol, health.Path, health.IntervalSeconds,
		health.TimeoutSeconds, health.FailureThreshold,
		svc.CreatedAt, svc.UpdatedAt); err != nil {
		return fmt.Errorf("insert gslb service: %w", err)
	}
	if err := insertPoolsTx(ctx, tx, svc.ID, pools); err != nil {
		return err
	}
	if svc.ActivePoolID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE gslb_services SET active_pool_id = $2 WHERE id = $1`,
			svc.ID, svc.ActivePoolID); err != nil {
			return fmt.Errorf("set gslb active pool: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gslb create service: %w", err)
	}
	return nil
}

// UpdateService 以 updated_at 乐观并发更新服务可变字段（name/requireApproval/
// 健康检查模板），返回更新后的主档；expectedUpdatedAt 不匹配返回 ok=false。
func (s *PostgresStore) UpdateService(ctx context.Context, svc appgslb.Service, expectedUpdatedAt time.Time) (appgslb.Service, bool, error) {
	updated, err := scanService(s.db.QueryRowContext(ctx, `
		UPDATE gslb_services SET
			name = $3, require_approval = $4,
			health_protocol = $5, health_path = $6, health_interval_seconds = $7,
			health_timeout_seconds = $8, health_failure_threshold = $9,
			updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND updated_at = $10
		RETURNING `+serviceColumns,
		svc.ID, svc.TenantID, svc.Name, svc.RequireApproval,
		svc.HealthCheck.Protocol, svc.HealthCheck.Path, svc.HealthCheck.IntervalSeconds,
		svc.HealthCheck.TimeoutSeconds, svc.HealthCheck.FailureThreshold,
		expectedUpdatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.Service{}, false, nil
	}
	if err != nil {
		// 租户内 name 唯一（迁移 081）：改名冲突按 409 语义返回。
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return appgslb.Service{}, false, fmt.Errorf("%w: service name %q already in use", appgslb.ErrConflict, svc.Name)
		}
		return appgslb.Service{}, false, fmt.Errorf("gslb update service: %w", err)
	}
	return updated, true, nil
}

// ReplaceServicePools 事务内整组替换地址池：CAS 校验版本并暂置空 active 引用 →
// 删旧池（级联成员）→ 建新池/成员 → 重指 active 池（newActivePoolID 为空表示当前
// 无 active 池，保持空置）→ Read Model 池引用重指（healthy 仅保留映射后的活跃池）。
func (s *PostgresStore) ReplaceServicePools(ctx context.Context, serviceID, tenantID string, pools []appgslb.Pool, newActivePoolID string, expectedUpdatedAt time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var oldActivePoolID string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(active_pool_id::text, '') FROM gslb_services
		WHERE id = $1 AND tenant_id = $2`, serviceID, tenantID).Scan(&oldActivePoolID); err != nil {
		return false, fmt.Errorf("gslb load active pool: %w", err)
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE gslb_services SET active_pool_id = NULL, updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND updated_at = $3`,
		serviceID, tenantID, expectedUpdatedAt)
	if err != nil {
		return false, fmt.Errorf("gslb replace pools guard: %w", err)
	}
	if affected, err := res.RowsAffected(); err != nil || affected == 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gslb_pools WHERE service_id = $1`, serviceID); err != nil {
		return false, fmt.Errorf("gslb delete old pools: %w", err)
	}
	if err := insertPoolsTx(ctx, tx, serviceID, pools); err != nil {
		return false, err
	}
	if newActivePoolID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE gslb_services SET active_pool_id = $2::uuid WHERE id = $1`,
			serviceID, newActivePoolID); err != nil {
			return false, fmt.Errorf("gslb repoint active pool: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gslb_read_model SET
			active_pool_id = NULLIF($2, '')::uuid,
			healthy_pools = CASE
				WHEN $2 <> '' AND $3 <> '' AND $3 = ANY(healthy_pools) THEN ARRAY[$2]::text[]
				ELSE '{}'::text[]
			END,
			observed_at = now()
		WHERE service_id = $1`, serviceID, newActivePoolID, oldActivePoolID); err != nil {
		return false, fmt.Errorf("gslb replace pools read model: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit gslb replace pools: %w", err)
	}
	return true, nil
}

// insertPoolsTx 在事务内写入地址池与成员（创建/整组替换共用）。
func insertPoolsTx(ctx context.Context, tx *sql.Tx, serviceID string, pools []appgslb.Pool) error {
	for _, pool := range pools {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gslb_pools (id, service_id, name, priority)
			VALUES ($1, $2, $3, $4)`, pool.ID, serviceID, pool.Name, pool.Priority); err != nil {
			return fmt.Errorf("insert gslb pool: %w", err)
		}
		for _, member := range pool.Members {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO gslb_pool_members (id, pool_id, cluster_id, weight, enabled, healthy)
				VALUES ($1, $2, $3, $4, true, true)`,
				uuid.NewString(), pool.ID, member.ClusterID, member.Weight); err != nil {
				return fmt.Errorf("insert gslb pool member: %w", err)
			}
		}
	}
	return nil
}

// DeleteService 删除服务主档；池/成员/Read Model/切换请求/演练报告经
// ON DELETE CASCADE 清理（迁移 081/082）。关联的平台 operations 行保留作审计。
func (s *PostgresStore) DeleteService(ctx context.Context, id, tenantID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM gslb_services WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return false, fmt.Errorf("gslb delete service: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("gslb delete service: %w", err)
	}
	return affected > 0, nil
}

// HasInflightSwitchRequests 报告服务是否存在进行中（未终态）的切换请求：
// PendingApproval（待审批）/Approved（已批准待执行）/Dispatched（执行中）。
func (s *PostgresStore) HasInflightSwitchRequests(ctx context.Context, serviceID, tenantID string) (bool, error) {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM gslb_switch_requests
			WHERE service_id = $1 AND tenant_id = $2
				AND status IN ('PendingApproval', 'Approved', 'Dispatched')
		)`, serviceID, tenantID).Scan(&exists); err != nil {
		return false, fmt.Errorf("gslb in-flight switch requests: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) GetPoolMemberClusterIDs(ctx context.Context, poolID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cluster_id FROM gslb_pool_members
		WHERE pool_id = $1 AND enabled = true
		ORDER BY cluster_id`, poolID)
	if err != nil {
		return nil, fmt.Errorf("gslb pool members: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const switchRequestColumns = `
		id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
		idempotency_key, correlation_id, require_approval, status, actor_id,
		COALESCE(approved_by, ''), approved_at, COALESCE(reason, ''),
		COALESCE(error, ''), COALESCE(operation_id::text, ''),
		COALESCE(dr_group_ref, ''), created_at, updated_at`

// ListPools 返回服务的地址池及启用成员（join gslb_services 校验租户归属；
// 按 priority 升序，成员按 cluster_id 排序）。
func (s *PostgresStore) ListPools(ctx context.Context, serviceID, tenantID string) ([]appgslb.Pool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.service_id, p.name, p.priority, m.cluster_id, m.weight
		FROM gslb_pools p
		JOIN gslb_services svc ON svc.id = p.service_id
		LEFT JOIN gslb_pool_members m ON m.pool_id = p.id AND m.enabled = true
		WHERE p.service_id = $1 AND svc.tenant_id = $2
		ORDER BY p.priority, p.id, m.cluster_id`, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gslb list pools: %w", err)
	}
	defer rows.Close()
	var pools []appgslb.Pool
	index := map[string]int{}
	for rows.Next() {
		var poolID, poolServiceID, name string
		var priority int
		var clusterID sql.NullString
		var weight sql.NullInt32
		if err := rows.Scan(&poolID, &poolServiceID, &name, &priority, &clusterID, &weight); err != nil {
			return nil, err
		}
		i, seen := index[poolID]
		if !seen {
			pools = append(pools, appgslb.Pool{ID: poolID, PoolID: poolID, ServiceID: poolServiceID, Name: name, Priority: priority, Members: []appgslb.PoolMember{}})
			i = len(pools) - 1
			index[poolID] = i
		}
		if clusterID.Valid {
			pools[i].Members = append(pools[i].Members, appgslb.PoolMember{ClusterID: clusterID.String, Weight: int(weight.Int32), Enabled: true})
		}
	}
	return pools, rows.Err()
}

func (s *PostgresStore) GetSwitchRequestByKey(ctx context.Context, tenantID, serviceID, idempotencyKey string) (appgslb.SwitchRequest, bool, error) {
	req, ok, err := s.getSwitchRequest(ctx, `
		SELECT `+switchRequestColumns+`
		FROM gslb_switch_requests
		WHERE tenant_id = $1 AND service_id = $2 AND idempotency_key = $3`,
		tenantID, serviceID, idempotencyKey)
	return req, ok, err
}

func (s *PostgresStore) GetSwitchRequest(ctx context.Context, id, tenantID string) (appgslb.SwitchRequest, bool, error) {
	req, ok, err := s.getSwitchRequest(ctx, `
		SELECT `+switchRequestColumns+`
		FROM gslb_switch_requests
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return req, ok, err
}

// ListSwitchRequests 返回服务维度租户隔离的切换请求（按 created_at 倒序，上限 100）。
func (s *PostgresStore) ListSwitchRequests(ctx context.Context, tenantID, serviceID, status string) ([]appgslb.SwitchRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+switchRequestColumns+`
		FROM gslb_switch_requests
		WHERE tenant_id = $1 AND service_id = $2 AND ($3 = '' OR status = $3)
		ORDER BY created_at DESC, id DESC
		LIMIT 100`, tenantID, serviceID, status)
	if err != nil {
		return nil, fmt.Errorf("gslb list switch requests: %w", err)
	}
	defer rows.Close()
	var requests []appgslb.SwitchRequest
	for rows.Next() {
		req, err := scanSwitchRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

// rowScanner 抽象 QueryRow 与 Rows 的 Scan，供单行/多行查询复用。
type rowScanner interface{ Scan(dest ...any) error }

func scanSwitchRequest(scanner rowScanner) (appgslb.SwitchRequest, error) {
	var req appgslb.SwitchRequest
	var plan []byte
	var approvedAt sql.NullTime
	err := scanner.Scan(&req.ID, &req.TenantID, &req.ServiceID, &req.IntentKind, &req.IntentDigest, &plan,
		&req.IdempotencyKey, &req.CorrelationID, &req.RequireApproval, &req.Status, &req.ActorID,
		&req.ApprovedBy, &approvedAt, &req.Reason, &req.Error,
		&req.OperationID, &req.DRGroupRef, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		return appgslb.SwitchRequest{}, err
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		req.ApprovedAt = &t
	}
	req.PlanSnapshot = json.RawMessage(plan)
	return req, nil
}

func (s *PostgresStore) getSwitchRequest(ctx context.Context, query string, args ...any) (appgslb.SwitchRequest, bool, error) {
	req, err := scanSwitchRequest(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.SwitchRequest{}, false, nil
	}
	if err != nil {
		return appgslb.SwitchRequest{}, false, fmt.Errorf("gslb get switch request: %w", err)
	}
	return req, true, nil
}

// CreateSwitchRequest 同事务写入：切换请求 + 平台 operations/operation_steps/
// operation_read_model 行（Operation Center 统一接线，GSLB-005）+ 演练报告
// （GSLB-010）+ Outbox 事件。
func (s *PostgresStore) CreateSwitchRequest(ctx context.Context, req appgslb.SwitchRequest, drill *appgslb.DrillReport, events []appgslb.OutboxEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin gslb request transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 平台 Operation 行须先于切换请求插入（operation_id 外键约束）。
	if err := insertOperationRows(ctx, tx, req, drill); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, actor_id,
			reason, operation_id, dr_group_ref, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''),
			NULLIF($13, '')::uuid, NULLIF($14, ''), $15, $16)`,
		req.ID, req.TenantID, req.ServiceID, req.IntentKind, req.IntentDigest,
		string(req.PlanSnapshot), req.IdempotencyKey, req.CorrelationID,
		req.RequireApproval, req.Status, req.ActorID, req.Reason,
		req.OperationID, req.DRGroupRef, req.CreatedAt, req.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert gslb switch request: %w", err)
	}
	if drill != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gslb_drill_reports (id, tenant_id, service_id, request_id, verdict, report, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			drill.ID, drill.TenantID, drill.ServiceID, drill.RequestID,
			drill.Verdict, string(drill.Report), drill.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert gslb drill report: %w", err)
		}
		// 最近演练写入 Read Model（GSLB-010）；投影行不存在时按服务主档补建。
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gslb_read_model (
				service_id, tenant_id, domain, lifecycle_state,
				last_drill_report_id, last_drill_verdict, last_drill_at
			)
			SELECT id, tenant_id, domain, lifecycle_state, $2, $3, $4
			FROM gslb_services WHERE id = $1
			ON CONFLICT (service_id) DO UPDATE SET
				last_drill_report_id = EXCLUDED.last_drill_report_id,
				last_drill_verdict = EXCLUDED.last_drill_verdict,
				last_drill_at = EXCLUDED.last_drill_at`,
			drill.ServiceID, drill.ID, drill.Verdict, drill.CreatedAt,
		); err != nil {
			return fmt.Errorf("project gslb drill read model: %w", err)
		}
	}
	for _, event := range events {
		if err := insertOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gslb request transaction: %w", err)
	}
	return nil
}

// operationTypeForIntent 映射 gslb 意图类型到平台 operation_type（迁移 082）。
func operationTypeForIntent(intentKind string) string {
	switch intentKind {
	case "gslb.failover":
		return "gslb_failover"
	case "gslb.switchback":
		return "gslb_switchback"
	case "gslb.weight-update":
		return "gslb_weight_update"
	default:
		return "gslb_drill"
	}
}

// operationStatusForRequest 映射切换请求状态到平台 Operation 状态机
// （engine 10 态子集：pending_approval/queued/cancelled/succeeded）。
func operationStatusForRequest(requestStatus string) string {
	switch requestStatus {
	case appgslb.StatusPendingApproval:
		return "pending_approval"
	case appgslb.StatusApproved:
		return "queued"
	case appgslb.StatusRejected:
		return "cancelled"
	case appgslb.StatusDispatched:
		return "in_progress"
	case appgslb.StatusFailed:
		return "failed"
	default:
		return "succeeded"
	}
}

// insertOperationRows 在平台 operations 表建立对应行与步骤行（含只读投影），
// 使 GSLB 流量变更在 Operation Center 统一可观测、可审计。
func insertOperationRows(ctx context.Context, tx *sql.Tx, req appgslb.SwitchRequest, drill *appgslb.DrillReport) error {
	var plan gslb.Plan
	if err := json.Unmarshal(req.PlanSnapshot, &plan); err != nil {
		return fmt.Errorf("unmarshal plan snapshot: %w", err)
	}
	opStatus := operationStatusForRequest(req.Status)
	isDrill := req.IntentKind == "gslb.drill"
	completedSteps := 0
	if isDrill {
		completedSteps = len(plan.Steps)
	}
	tags := map[string]any{
		"gslbServiceId":   req.ServiceID,
		"switchRequestId": req.ID,
		"intentKind":      req.IntentKind,
	}
	if req.DRGroupRef != "" {
		tags["drGroupRef"] = req.DRGroupRef
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("marshal operation tags: %w", err)
	}

	var completedAt any
	if isDrill {
		completedAt = req.CreatedAt
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest,
			total_steps, completed_steps, failed_steps, tags, created_at, completed_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, 0, $11, $12, $13)`,
		req.OperationID, req.TenantID, operationTypeForIntent(req.IntentKind), opStatus,
		req.ActorID, req.CorrelationID, "gslb-"+req.ID, req.IntentDigest,
		len(plan.Steps), completedSteps, string(tagsJSON), req.CreatedAt, completedAt,
	); err != nil {
		return fmt.Errorf("insert gslb operation row: %w", err)
	}

	for _, step := range plan.Steps {
		stepStatus := "pending"
		var stepOutput, stepCompletedAt any
		if isDrill {
			stepStatus = "succeeded"
			stepCompletedAt = req.CreatedAt
			if drill != nil {
				output, _ := json.Marshal(map[string]any{"verdict": drill.Verdict, "drillReportId": drill.ID})
				stepOutput = string(output)
			}
		}
		inputJSON, err := json.Marshal(step.Inputs)
		if err != nil {
			return fmt.Errorf("marshal step inputs: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operation_steps (
				operation_id, plan_step_id, step_name, step_type, status,
				idempotency_key, depends_on, step_input, step_output, completed_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			req.OperationID, step.StepID, step.StepID, string(step.StepType), stepStatus,
			step.IdempotencyKey, pq.Array(step.DependsOn), string(inputJSON), stepOutput, stepCompletedAt,
		); err != nil {
			return fmt.Errorf("insert gslb operation step: %w", err)
		}
	}

	summary := fmt.Sprintf("%s %s: %s", operationTypeForIntent(req.IntentKind), req.OperationID, opStatus)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, operation_type, status, total_steps,
			completed_steps, failed_steps, initiated_by, summary, tags,
			created_at, completed_at, last_state_changed_at
		) VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, $9, $10, $11, now())`,
		req.OperationID, req.TenantID, operationTypeForIntent(req.IntentKind), opStatus,
		len(plan.Steps), completedSteps, req.ActorID, summary, string(tagsJSON),
		req.CreatedAt, completedAt,
	); err != nil {
		return fmt.Errorf("insert gslb operation read model: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateSwitchRequestStatus(ctx context.Context, id, status string, fields map[string]any, events []appgslb.OutboxEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin gslb status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	setClause := "status = $2, updated_at = now()"
	args := []any{id, status}
	// 受控字段白名单，防止任意列注入
	for _, key := range []string{"approved_by", "approved_at", "error", "reason"} {
		if value, ok := fields[key]; ok {
			args = append(args, value)
			setClause += fmt.Sprintf(", %s = $%d", key, len(args))
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE gslb_switch_requests SET `+setClause+` WHERE id = $1`, args...); err != nil {
		return fmt.Errorf("update gslb switch request status: %w", err)
	}
	// 同步平台 Operation 行（Operation Center 统一接线）：审批通过 → queued，
	// 拒绝 → cancelled。
	opStatus := operationStatusForRequest(status)
	if _, err := tx.ExecContext(ctx, `
		UPDATE operations SET status = $2, status_reason = COALESCE(NULLIF($3, ''), status_reason), updated_at = now()
		WHERE id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`, id, opStatus, ""); err != nil {
		return fmt.Errorf("sync gslb operation status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_read_model SET status = $2, last_state_changed_at = now()
		WHERE operation_id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`, id, opStatus); err != nil {
		return fmt.Errorf("sync gslb operation read model: %w", err)
	}
	for _, event := range events {
		if err := insertOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gslb status transaction: %w", err)
	}
	return nil
}

// CompleteSwitchRequest 在控制台批准后完成本地投影闭环：请求/Operation 进入
// Succeeded，服务 active_pool_id 与 read model 更新为不可变计划中的目标池。
// DNS 数据面仍只能由 gslb-controller 消费 outbox 命令触达；这里不调用 provider。
func (s *PostgresStore) CompleteSwitchRequest(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin gslb completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := completeSwitchRequestTx(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gslb completion transaction: %w", err)
	}
	return nil
}

func completeSwitchRequestTx(ctx context.Context, tx *sql.Tx, id string) error {
	var serviceID, tenantID, domain, intentKind string
	var planJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT r.service_id::text, r.tenant_id, s.domain, r.intent_kind, r.plan_snapshot
		FROM gslb_switch_requests r
		JOIN gslb_services s ON s.id = r.service_id
		WHERE r.id = $1`, id).Scan(&serviceID, &tenantID, &domain, &intentKind, &planJSON); err != nil {
		return fmt.Errorf("load gslb completion context: %w", err)
	}
	targetPoolID, targets, err := targetFromPlan(planJSON)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE gslb_switch_requests SET status = $2, updated_at = now()
		WHERE id = $1 AND status IN ($3, $4)`, id, appgslb.StatusSucceeded, appgslb.StatusApproved, appgslb.StatusDispatched); err != nil {
		return fmt.Errorf("complete gslb switch request: %w", err)
	}
	// weight-update：将计划权重落到池成员（与 controller 成功路径一致）。
	if intentKind == string(gslb.IntentWeightUpdate) {
		for member, weight := range weightsFromPlan(planJSON) {
			if _, err := tx.ExecContext(ctx, `
				UPDATE gslb_pool_members pm SET weight = $3
				FROM gslb_pools p
				WHERE pm.pool_id = p.id AND p.service_id = $1 AND pm.cluster_id = $2`,
				serviceID, member, weight); err != nil {
				return fmt.Errorf("apply gslb member weight: %w", err)
			}
		}
	}
	if targetPoolID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE gslb_services
			SET active_pool_id = $2::uuid, lifecycle_state = 'Active', updated_at = now()
			WHERE id = $1`, serviceID, targetPoolID); err != nil {
			return fmt.Errorf("complete gslb active pool: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gslb_read_model (
				service_id, tenant_id, domain, active_pool_id, lifecycle_state,
				healthy_pools, current_dns_targets, last_switch_request_id, last_switch_at, observed_at
			) VALUES ($1, $2, $3, $4::uuid, 'Active', ARRAY[$4]::text[], $5, $6, now(), now())
			ON CONFLICT (service_id) DO UPDATE SET
				tenant_id = EXCLUDED.tenant_id,
				domain = EXCLUDED.domain,
				active_pool_id = EXCLUDED.active_pool_id,
				lifecycle_state = EXCLUDED.lifecycle_state,
				healthy_pools = ARRAY(SELECT DISTINCT unnest(gslb_read_model.healthy_pools || EXCLUDED.healthy_pools)),
				current_dns_targets = EXCLUDED.current_dns_targets,
				last_switch_request_id = EXCLUDED.last_switch_request_id,
				last_switch_at = EXCLUDED.last_switch_at,
				observed_at = EXCLUDED.observed_at`,
			serviceID, tenantID, domain, targetPoolID, pq.Array(targets), id); err != nil {
			return fmt.Errorf("complete gslb read model: %w", err)
		}
	}
	if err := syncCompletedOperationTx(ctx, tx, id); err != nil {
		return err
	}
	return nil
}

func syncCompletedOperationTx(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status = 'succeeded', completed_at = now(), completed_steps = total_steps,
			failed_steps = 0, updated_at = now()
		WHERE id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`, id); err != nil {
		return fmt.Errorf("complete gslb operation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_steps
		SET status = CASE WHEN step_type = 'gslb_dns_revert' THEN 'skipped' ELSE 'succeeded' END,
			completed_at = now(), updated_at = now()
		WHERE operation_id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`, id); err != nil {
		return fmt.Errorf("complete gslb operation steps: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_read_model
		SET status = 'succeeded', completed_at = now(), completed_steps = total_steps,
			failed_steps = 0, last_state_changed_at = now()
		WHERE operation_id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`, id); err != nil {
		return fmt.Errorf("complete gslb operation read model: %w", err)
	}
	return nil
}

func targetFromPlan(planJSON []byte) (string, []string, error) {
	var plan gslb.Plan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return "", nil, fmt.Errorf("unmarshal gslb completion plan: %w", err)
	}
	for _, step := range plan.Steps {
		if step.StepType != gslb.StepDNSApply {
			continue
		}
		poolID, _ := step.Inputs["targetPoolId"].(string)
		return poolID, stringSlice(step.Inputs["targets"]), nil
	}
	return "", nil, nil
}

// weightsFromPlan 提取计划 apply 步骤中钉死的成员权重（weight-update）。
func weightsFromPlan(planJSON []byte) map[string]int {
	var plan gslb.Plan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return nil
	}
	for _, step := range plan.Steps {
		if step.StepType != gslb.StepDNSApply {
			continue
		}
		raw, ok := step.Inputs["weights"].(map[string]any)
		if !ok {
			return nil
		}
		weights := make(map[string]int, len(raw))
		for member, value := range raw {
			if n, ok := value.(float64); ok {
				weights[member] = int(n)
			}
		}
		return weights
	}
	return nil
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ListDrillReports 返回服务的结构化演练报告（GSLB-010，按时间倒序）。
func (s *PostgresStore) ListDrillReports(ctx context.Context, serviceID, tenantID string) ([]appgslb.DrillReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, service_id, request_id, verdict, report, created_at
		FROM gslb_drill_reports
		WHERE service_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
		LIMIT 50`, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gslb list drill reports: %w", err)
	}
	defer rows.Close()
	var reports []appgslb.DrillReport
	for rows.Next() {
		var report appgslb.DrillReport
		var payload []byte
		if err := rows.Scan(&report.ID, &report.TenantID, &report.ServiceID,
			&report.RequestID, &report.Verdict, &payload, &report.CreatedAt); err != nil {
			return nil, err
		}
		report.Report = json.RawMessage(payload)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func insertOutboxEvent(ctx context.Context, tx *sql.Tx, event appgslb.OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal gslb outbox payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, payload
		) VALUES (
			$1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10, $11, $12, $13
		)`,
		event.MessageID, event.MessageType, event.SchemaVersion, event.Subject,
		time.Now().UTC(), event.TenantID, event.ActorID, event.CorrelationID,
		event.IdempotencyKey, event.AggregateID, event.AggregateVersion,
		event.AggregateID, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert gslb outbox event: %w", err)
	}
	return nil
}

// EnsureService 为测试/演示创建 GSLB 服务与池（生产由创建 API 提供）。
// 复用 CreateService 事务路径；成员权重固定 100，健康检查取默认模板。
func (s *PostgresStore) EnsureService(ctx context.Context, tenantID, name, domain string, targetPoolMembers map[string][]string) (string, error) {
	now := time.Now().UTC()
	svc := appgslb.Service{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		Name:            name,
		Domain:          domain,
		RoutingMode:     "dns",
		LifecycleState:  "Active",
		RequireApproval: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	pools := make([]appgslb.Pool, 0, len(targetPoolMembers))
	for poolName, members := range targetPoolMembers {
		pool := appgslb.Pool{ID: uuid.NewString(), Name: poolName}
		if poolName != "active" {
			pool.Priority = 1
		}
		for _, clusterID := range members {
			pool.Members = append(pool.Members, appgslb.PoolMember{ClusterID: clusterID, Weight: 100})
		}
		if poolName == "active" {
			svc.ActivePoolID = pool.ID
		}
		pools = append(pools, pool)
	}
	if err := s.CreateService(ctx, svc, appgslb.DefaultHealthCheck(), pools); err != nil {
		return "", err
	}
	return svc.ID, nil
}

// readModelColumns 投影列 + join gslb_services 的健康检查模板（迁移 094）。
const readModelColumns = `
		rm.service_id, rm.tenant_id, rm.domain, COALESCE(rm.active_pool_id::text, ''),
		rm.lifecycle_state, rm.healthy_pools, rm.current_dns_targets,
		COALESCE(rm.last_drill_report_id::text, ''), COALESCE(rm.last_drill_verdict, ''),
		rm.last_drill_at, rm.observed_at,
		s.health_protocol, s.health_path, s.health_interval_seconds,
		s.health_timeout_seconds, s.health_failure_threshold`

func scanReadModel(scanner rowScanner) (appgslb.ReadModel, error) {
	var m appgslb.ReadModel
	var lastDrillAt sql.NullTime
	err := scanner.Scan(&m.ServiceID, &m.TenantID, &m.Domain, &m.ActivePoolID,
		&m.LifecycleState, pq.Array(&m.HealthyPools), pq.Array(&m.CurrentDNSTargets),
		&m.LastDrillReportID, &m.LastDrillVerdict, &lastDrillAt, &m.ObservedAt,
		&m.HealthCheck.Protocol, &m.HealthCheck.Path, &m.HealthCheck.IntervalSeconds,
		&m.HealthCheck.TimeoutSeconds, &m.HealthCheck.FailureThreshold)
	if err != nil {
		return appgslb.ReadModel{}, err
	}
	if lastDrillAt.Valid {
		t := lastDrillAt.Time
		m.LastDrillAt = &t
	}
	return m, nil
}

// ListReadModels 返回租户范围内的 GSLB 只读投影（GSLB-007，请求路径零探测）。
func (s *PostgresStore) ListReadModels(ctx context.Context, tenantID string) ([]appgslb.ReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+readModelColumns+`
		FROM gslb_read_model rm
		JOIN gslb_services s ON s.id = rm.service_id
		WHERE rm.tenant_id = $1
		ORDER BY rm.domain`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gslb list read models: %w", err)
	}
	defer rows.Close()
	var models []appgslb.ReadModel
	for rows.Next() {
		m, err := scanReadModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// GetReadModel 返回单个投影（租户隔离）。
func (s *PostgresStore) GetReadModel(ctx context.Context, id, tenantID string) (appgslb.ReadModel, bool, error) {
	m, err := scanReadModel(s.db.QueryRowContext(ctx, `
		SELECT `+readModelColumns+`
		FROM gslb_read_model rm
		JOIN gslb_services s ON s.id = rm.service_id
		WHERE rm.service_id = $1 AND rm.tenant_id = $2`, id, tenantID))
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.ReadModel{}, false, nil
	}
	if err != nil {
		return appgslb.ReadModel{}, false, fmt.Errorf("gslb get read model: %w", err)
	}
	return m, true, nil
}
