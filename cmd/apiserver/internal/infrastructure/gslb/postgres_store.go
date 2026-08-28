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

func (s *PostgresStore) GetService(ctx context.Context, id, tenantID string) (appgslb.Service, bool, error) {
	var svc appgslb.Service
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, domain, routing_mode,
			COALESCE(active_pool_id::text, ''), lifecycle_state, require_approval,
			created_at, updated_at
		FROM gslb_services
		WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&svc.ID, &svc.TenantID, &svc.Name, &svc.Domain, &svc.RoutingMode,
			&svc.ActivePoolID, &svc.LifecycleState, &svc.RequireApproval,
			&svc.CreatedAt, &svc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.Service{}, false, nil
	}
	if err != nil {
		return appgslb.Service{}, false, fmt.Errorf("gslb get service: %w", err)
	}
	return svc, true, nil
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

func (s *PostgresStore) GetSwitchRequestByKey(ctx context.Context, tenantID, serviceID, idempotencyKey string) (appgslb.SwitchRequest, bool, error) {
	req, ok, err := s.getSwitchRequest(ctx, `
		SELECT id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, actor_id,
			COALESCE(approved_by, ''), approved_at, COALESCE(reason, ''),
			COALESCE(error, ''), COALESCE(operation_id::text, ''),
			COALESCE(dr_group_ref, ''), created_at, updated_at
		FROM gslb_switch_requests
		WHERE tenant_id = $1 AND service_id = $2 AND idempotency_key = $3`,
		tenantID, serviceID, idempotencyKey)
	return req, ok, err
}

func (s *PostgresStore) GetSwitchRequest(ctx context.Context, id, tenantID string) (appgslb.SwitchRequest, bool, error) {
	req, ok, err := s.getSwitchRequest(ctx, `
		SELECT id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, actor_id,
			COALESCE(approved_by, ''), approved_at, COALESCE(reason, ''),
			COALESCE(error, ''), COALESCE(operation_id::text, ''),
			COALESCE(dr_group_ref, ''), created_at, updated_at
		FROM gslb_switch_requests
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return req, ok, err
}

func (s *PostgresStore) getSwitchRequest(ctx context.Context, query string, args ...any) (appgslb.SwitchRequest, bool, error) {
	var req appgslb.SwitchRequest
	var plan []byte
	var approvedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, args...).
		Scan(&req.ID, &req.TenantID, &req.ServiceID, &req.IntentKind, &req.IntentDigest, &plan,
			&req.IdempotencyKey, &req.CorrelationID, &req.RequireApproval, &req.Status, &req.ActorID,
			&req.ApprovedBy, &approvedAt, &req.Reason, &req.Error,
			&req.OperationID, &req.DRGroupRef, &req.CreatedAt, &req.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.SwitchRequest{}, false, nil
	}
	if err != nil {
		return appgslb.SwitchRequest{}, false, fmt.Errorf("gslb get switch request: %w", err)
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		req.ApprovedAt = &t
	}
	req.PlanSnapshot = json.RawMessage(plan)
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

// EnsureService 为测试/演示创建 GSLB 服务与池（生产由 T5 创建 API 提供）。
func (s *PostgresStore) EnsureService(ctx context.Context, tenantID, name, domain string, targetPoolMembers map[string][]string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	serviceID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval)
		VALUES ($1, $2, $3, $4, 'dns', 'Active', true)`,
		serviceID, tenantID, name, domain); err != nil {
		return "", fmt.Errorf("insert gslb service: %w", err)
	}
	activePoolID := ""
	for poolName, members := range targetPoolMembers {
		poolID := uuid.NewString()
		priority := 0
		if poolName != "active" {
			priority = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gslb_pools (id, service_id, name, priority)
			VALUES ($1, $2, $3, $4)`, poolID, serviceID, poolName, priority); err != nil {
			return "", err
		}
		for _, clusterID := range members {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO gslb_pool_members (id, pool_id, cluster_id, weight, enabled, healthy)
				VALUES ($1, $2, $3, 100, true, true)`, uuid.NewString(), poolID, clusterID); err != nil {
				return "", err
			}
		}
		if poolName == "active" {
			activePoolID = poolID
		}
	}
	if activePoolID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE gslb_services SET active_pool_id = $2 WHERE id = $1`,
			serviceID, activePoolID); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return serviceID, nil
}

// ListReadModels 返回租户范围内的 GSLB 只读投影（GSLB-007，请求路径零探测）。
func (s *PostgresStore) ListReadModels(ctx context.Context, tenantID string) ([]appgslb.ReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT service_id, tenant_id, domain, COALESCE(active_pool_id::text, ''),
			lifecycle_state, healthy_pools, current_dns_targets,
			COALESCE(last_drill_report_id::text, ''), COALESCE(last_drill_verdict, ''),
			last_drill_at, observed_at
		FROM gslb_read_model
		WHERE tenant_id = $1
		ORDER BY domain`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gslb list read models: %w", err)
	}
	defer rows.Close()
	var models []appgslb.ReadModel
	for rows.Next() {
		var m appgslb.ReadModel
		var lastDrillAt sql.NullTime
		if err := rows.Scan(&m.ServiceID, &m.TenantID, &m.Domain, &m.ActivePoolID,
			&m.LifecycleState, pq.Array(&m.HealthyPools), pq.Array(&m.CurrentDNSTargets),
			&m.LastDrillReportID, &m.LastDrillVerdict, &lastDrillAt, &m.ObservedAt); err != nil {
			return nil, err
		}
		if lastDrillAt.Valid {
			t := lastDrillAt.Time
			m.LastDrillAt = &t
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// GetReadModel 返回单个投影（租户隔离）。
func (s *PostgresStore) GetReadModel(ctx context.Context, id, tenantID string) (appgslb.ReadModel, bool, error) {
	var m appgslb.ReadModel
	var lastDrillAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT service_id, tenant_id, domain, COALESCE(active_pool_id::text, ''),
			lifecycle_state, healthy_pools, current_dns_targets,
			COALESCE(last_drill_report_id::text, ''), COALESCE(last_drill_verdict, ''),
			last_drill_at, observed_at
		FROM gslb_read_model
		WHERE service_id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&m.ServiceID, &m.TenantID, &m.Domain, &m.ActivePoolID,
			&m.LifecycleState, pq.Array(&m.HealthyPools), pq.Array(&m.CurrentDNSTargets),
			&m.LastDrillReportID, &m.LastDrillVerdict, &lastDrillAt, &m.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return appgslb.ReadModel{}, false, nil
	}
	if err != nil {
		return appgslb.ReadModel{}, false, fmt.Errorf("gslb get read model: %w", err)
	}
	if lastDrillAt.Valid {
		t := lastDrillAt.Time
		m.LastDrillAt = &t
	}
	return m, true, nil
}
