package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/F31/hnb/pkg/gslb"
)

// 状态常量（与迁移 081 CHECK 一致）
const (
	RequestStatusPendingApproval = "PendingApproval"
	RequestStatusApproved        = "Approved"
	RequestStatusDispatched      = "Dispatched"
	RequestStatusSucceeded       = "Succeeded"
	RequestStatusFailed          = "Failed"
)

// Event 常量（与 apiserver 应用层一致）
const (
	EventStatusChanged     = "hnb.event.gslb.status-changed.v1"
	EventIntentSubmitted   = "hnb.event.gslb.intent-submitted.v1"
)

// SwitchRequest 是控制器视角的切换请求投影。
type SwitchRequest struct {
	ID            string
	TenantID      string
	ServiceID     string
	IntentKind    string
	IntentDigest  string
	PlanSnapshot  json.RawMessage
	IdempotencyKey string
	Status        string
	ApprovedBy    string
	Error         string
	// OperationID 关联平台 operations 行（Operation Center 统一接线，迁移 082）。
	OperationID   string
}

// SwitchRequestStore 读写 gslb_switch_requests（平台共享状态）。
type SwitchRequestStore struct{ db *sql.DB }

func NewSwitchRequestStore(db *sql.DB) *SwitchRequestStore {
	return &SwitchRequestStore{db: db}
}

// EnsureFailoverForDomain 按域名解析服务并幂等创建故障转移请求
// （reconciler 决策上报入口）。
func (s *SwitchRequestStore) EnsureFailoverForDomain(ctx context.Context, domain string) (bool, error) {
	service, ok, err := s.FindServiceByDomain(ctx, domain)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return s.EnsureFailoverRequest(ctx, service)
}

// FindServiceByDomain 按入口域名解析 GSLB 服务（多租户下要求唯一）。
func (s *SwitchRequestStore) FindServiceByDomain(ctx context.Context, domain string) (ServiceRow, bool, error) {
	var svc ServiceRow
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, domain, COALESCE(active_pool_id::text, '')
		FROM gslb_services
		WHERE domain = $1
		ORDER BY created_at
		LIMIT 1`, domain).
		Scan(&svc.ID, &svc.TenantID, &svc.Domain, &svc.ActivePoolID)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceRow{}, false, nil
	}
	if err != nil {
		return ServiceRow{}, false, fmt.Errorf("find gslb service by domain: %w", err)
	}
	return svc, true, nil
}

type ServiceRow struct {
	ID            string
	TenantID      string
	Domain        string
	ActivePoolID  string
}

// BackupPool 返回服务的备用池（优先级最高的非活跃池）及其成员。
func (s *SwitchRequestStore) BackupPool(ctx context.Context, serviceID, activePoolID string) (poolID string, memberClusterIDs []string, err error) {
	var id sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT p.id
		FROM gslb_pools p
		WHERE p.service_id = $1 AND p.id::text <> COALESCE($2, '')
		ORDER BY p.priority DESC
		LIMIT 1`, serviceID, activePoolID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !id.Valid) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("gslb backup pool: %w", err)
	}
	poolID = id.String

	rows, err := s.db.QueryContext(ctx, `
		SELECT pm.cluster_id
		FROM gslb_pool_members pm
		WHERE pm.pool_id = $1 AND pm.enabled = true
		ORDER BY pm.cluster_id`, poolID)
	if err != nil {
		return "", nil, fmt.Errorf("gslb backup pool members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var member string
		if err := rows.Scan(&member); err != nil {
			return "", nil, err
		}
		memberClusterIDs = append(memberClusterIDs, member)
	}
	return poolID, memberClusterIDs, rows.Err()
}

// HasActiveFailoverRequest 判断服务是否存在未终态的自动故障转移请求。
func (s *SwitchRequestStore) HasActiveFailoverRequest(ctx context.Context, serviceID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM gslb_switch_requests
		WHERE service_id = $1
			AND intent_kind = 'gslb.failover'
			AND status IN ('PendingApproval', 'Approved', 'Dispatched')`, serviceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("gslb active failover request: %w", err)
	}
	return count > 0, nil
}

// EnsureFailoverRequest 幂等创建自动故障转移请求（GSLB-005 受控）：
// 只写请求 + Outbox 事件，绝不直接修改 DNS。
func (s *SwitchRequestStore) EnsureFailoverRequest(ctx context.Context, service ServiceRow) (created bool, err error) {
	backupPoolID, backupTargets, err := s.BackupPool(ctx, service.ID, service.ActivePoolID)
	if err != nil {
		return false, err
	}
	if backupPoolID == "" || len(backupTargets) == 0 {
		return false, nil
	}
	active, err := s.HasActiveFailoverRequest(ctx, service.ID)
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	intent := &gslb.Intent{
		APIVersion:   gslb.APIVersion,
		Kind:         gslb.IntentFailover,
		ServiceID:    service.ID,
		TenantID:     service.TenantID,
		TargetPoolID: backupPoolID,
		Reason:       "auto-failover: active pool unhealthy",
		Metadata: gslb.IntentMetadata{
			IdempotencyKey: fmt.Sprintf("auto-failover-%s", service.ID),
			CorrelationID:  uuid.NewString(),
		},
	}
	plan, err := intent.BuildPlan(gslb.PlanInput{
		ServiceID: service.ID, TenantID: service.TenantID, Domain: service.Domain,
		TargetPoolID: backupPoolID, Targets: backupTargets,
	})
	if err != nil {
		return false, err
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	requestID := uuid.NewString()
	operationID := uuid.NewString()
	// 平台 Operation 行（Operation Center 统一接线）：与请求同事务建立；
	// 须先于 gslb_switch_requests 插入（operation_id 外键）。
	tagsJSON, _ := json.Marshal(map[string]any{
		"gslbServiceId": service.ID, "switchRequestId": requestID,
		"intentKind": string(intent.Kind), "origin": "auto-failover",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest, total_steps, tags
		) VALUES ($1, $2, 'gslb_failover', 'pending_approval', 'gslb-controller', $3, $4, $5, $6, $7)`,
		operationID, service.TenantID, intent.Metadata.CorrelationID,
		"gslb-"+requestID, intent.SemanticDigest(), len(plan.Steps), string(tagsJSON),
	); err != nil {
		return false, fmt.Errorf("insert auto failover operation: %w", err)
	}
	for _, step := range plan.Steps {
		inputJSON, _ := json.Marshal(step.Inputs)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operation_steps (
				operation_id, plan_step_id, step_name, step_type, idempotency_key, depends_on, step_input
			) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			operationID, step.StepID, step.StepID, string(step.StepType),
			step.IdempotencyKey, pq.Array(step.DependsOn), string(inputJSON),
		); err != nil {
			return false, fmt.Errorf("insert auto failover operation step: %w", err)
		}
	}
	summary := fmt.Sprintf("gslb_failover %s: pending_approval", operationID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, operation_type, status, total_steps,
			initiated_by, summary, tags, created_at, last_state_changed_at
		) VALUES ($1, $2, 'gslb_failover', 'pending_approval', $3, 'gslb-controller', $4, $5, now(), now())`,
		operationID, service.TenantID, len(plan.Steps), summary, string(tagsJSON),
	); err != nil {
		return false, fmt.Errorf("insert auto failover operation read model: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, actor_id, reason,
			operation_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, 'PendingApproval', 'gslb-controller', $9, $10, $11, $11)`,
		requestID, service.TenantID, service.ID, string(intent.Kind), intent.SemanticDigest(),
		string(planJSON), intent.Metadata.IdempotencyKey, intent.Metadata.CorrelationID,
		intent.Reason, operationID, time.Now().UTC(),
	); err != nil {
		return false, fmt.Errorf("insert auto failover request: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"requestId": requestID, "serviceId": service.ID,
		"kind": string(intent.Kind), "digest": intent.SemanticDigest(),
		"reason": "auto-failover",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, payload
		) VALUES ($1, $2, 'v1', $3, $4, $5, 'gslb-controller', $6, $7, $8, 0, $8, $9)`,
		uuid.NewString(), EventIntentSubmitted, EventIntentSubmitted, time.Now().UTC(),
		service.TenantID, intent.Metadata.CorrelationID,
		fmt.Sprintf("gslb-auto-%s", service.ID), service.ID, string(payload),
	); err != nil {
		return false, fmt.Errorf("insert auto failover outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetRequest 读取切换请求。
func (s *SwitchRequestStore) GetRequest(ctx context.Context, id string) (SwitchRequest, bool, error) {
	var req SwitchRequest
	var plan []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, status, COALESCE(approved_by, ''), COALESCE(error, ''),
			COALESCE(operation_id::text, '')
		FROM gslb_switch_requests
		WHERE id = $1`, id).
		Scan(&req.ID, &req.TenantID, &req.ServiceID, &req.IntentKind, &req.IntentDigest, &plan,
			&req.IdempotencyKey, &req.Status, &req.ApprovedBy, &req.Error, &req.OperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SwitchRequest{}, false, nil
	}
	if err != nil {
		return SwitchRequest{}, false, err
	}
	req.PlanSnapshot = plan
	return req, true, nil
}

// Transition 带状态守卫的状态流转（并发安全：仅在 from 状态上迁移），
// 并同事务同步关联的平台 Operation 行（Operation Center 统一接线）。
func (s *SwitchRequestStore) Transition(ctx context.Context, id string, from []string, to string, errorMsg string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin gslb transition transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE gslb_switch_requests
		SET status = $2, error = NULLIF($3, ''), updated_at = now()
		WHERE id = $1 AND status = ANY($4)`, id, to, errorMsg, pq.Array(from))
	if err != nil {
		return fmt.Errorf("transition gslb request: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("gslb request transition rejected: status guard failed")
	}
	if err := syncOperationTx(ctx, tx, id, to, errorMsg); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gslb transition transaction: %w", err)
	}
	return nil
}

// syncOperationTx 将执行结果同步到平台 operations / operation_read_model /
// operation_steps：Dispatched → in_progress；Succeeded → succeeded（apply/verify
// 步骤 succeeded，revert 补偿步骤 skipped）；Failed → failed（记录原因）。
func syncOperationTx(ctx context.Context, tx *sql.Tx, requestID, toStatus, errorMsg string) error {
	var opStatus string
	switch toStatus {
	case RequestStatusDispatched:
		opStatus = "in_progress"
	case RequestStatusSucceeded:
		opStatus = "succeeded"
	case RequestStatusFailed:
		opStatus = "failed"
	default:
		return nil
	}
	var startedAt, completedAt any
	if toStatus == RequestStatusDispatched {
		startedAt = time.Now().UTC()
	}
	if toStatus == RequestStatusSucceeded || toStatus == RequestStatusFailed {
		completedAt = time.Now().UTC()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status = $2,
			status_reason = NULLIF($3, ''),
			started_at = COALESCE($4, started_at),
			completed_at = COALESCE($5, completed_at),
			completed_steps = CASE WHEN $2 = 'succeeded' THEN total_steps ELSE completed_steps END,
			failed_steps = CASE WHEN $2 = 'failed' THEN 1 ELSE failed_steps END,
			updated_at = now()
		WHERE id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`,
		requestID, opStatus, errorMsg, startedAt, completedAt); err != nil {
		return fmt.Errorf("sync gslb operation row: %w", err)
	}
	if toStatus == RequestStatusSucceeded {
		if _, err := tx.ExecContext(ctx, `
			UPDATE operation_steps
			SET status = CASE WHEN step_type = 'gslb_dns_revert' THEN 'skipped' ELSE 'succeeded' END,
				completed_at = now(), updated_at = now()
			WHERE operation_id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`,
			requestID); err != nil {
			return fmt.Errorf("sync gslb operation steps: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_read_model
		SET status = $2,
			started_at = COALESCE($3, started_at),
			completed_at = COALESCE($4, completed_at),
			completed_steps = CASE WHEN $2 = 'succeeded' THEN total_steps ELSE completed_steps END,
			failed_steps = CASE WHEN $2 = 'failed' THEN 1 ELSE failed_steps END,
			last_state_changed_at = now()
		WHERE operation_id = (SELECT operation_id FROM gslb_switch_requests WHERE id = $1)`,
		requestID, opStatus, startedAt, completedAt); err != nil {
		return fmt.Errorf("sync gslb operation read model: %w", err)
	}
	return nil
}

// EmitStatusChanged 写执行结果领域事件（状态流转审计）。
func (s *SwitchRequestStore) EmitStatusChanged(ctx context.Context, request SwitchRequest, status, errorMsg string) error {
	payload, _ := json.Marshal(map[string]any{
		"requestId": request.ID, "serviceId": request.ServiceID,
		"status": status, "error": errorMsg, "operationId": request.OperationID,
	})
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, payload
		) VALUES ($1, $2, 'v1', $3, $4, $5, 'gslb-controller', $6, $7, $8, 0, $8, $9)`,
		uuid.NewString(), EventStatusChanged, EventStatusChanged, time.Now().UTC(),
		request.TenantID, uuid.NewString(),
		fmt.Sprintf("gslb-status-%s-%s", request.ID, status),
		request.ServiceID, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert gslb status event: %w", err)
	}
	return nil
}

// ProjectReadModel 更新 GSLB 只读投影（GSLB-007）：按域名解析服务，
// 统计健康池与当前 DNS 目标，upsert 到 gslb_read_model。
// healthyTargets 为控制器探测出的健康集群集合。
func (s *SwitchRequestStore) ProjectReadModel(ctx context.Context, domain string, healthyTargets []string) error {
	service, ok, err := s.FindServiceByDomain(ctx, domain)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	healthySet := make(map[string]bool, len(healthyTargets))
	for _, target := range healthyTargets {
		healthySet[target] = true
	}

	// 健康池与当前目标：以控制器探活结果（healthyTargets）为准（GSLB-011）。
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.priority, pm.cluster_id
		FROM gslb_pools p
		LEFT JOIN gslb_pool_members pm ON pm.pool_id = p.id AND pm.enabled = true
		WHERE p.service_id = $1
		ORDER BY p.priority ASC, p.id, pm.cluster_id`, service.ID)
	if err != nil {
		return fmt.Errorf("gslb pool members: %w", err)
	}
	defer rows.Close()

	poolHealth := make(map[string]bool) // poolID -> 是否有健康成员
	poolTargets := make(map[string][]string)
	for rows.Next() {
		var poolID, member sql.NullString
		var priority int
		if err := rows.Scan(&poolID, &priority, &member); err != nil {
			return err
		}
		if !poolID.Valid {
			continue
		}
		if member.Valid {
			poolTargets[poolID.String] = append(poolTargets[poolID.String], member.String)
			if healthySet[member.String] {
				poolHealth[poolID.String] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var healthyPools []string
	var currentTargets []string
	for poolID := range poolHealth {
		if poolHealth[poolID] {
			healthyPools = append(healthyPools, poolID)
			if poolID == service.ActivePoolID {
				for _, member := range poolTargets[poolID] {
					if healthySet[member] {
						currentTargets = append(currentTargets, member)
					}
				}
			}
		}
	}
	sort.Strings(healthyPools)
	sort.Strings(currentTargets)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO gslb_read_model (
			service_id, tenant_id, domain, active_pool_id, lifecycle_state,
			healthy_pools, current_dns_targets, observed_at
		) VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::text[]), COALESCE($7, '{}'::text[]), now())
		ON CONFLICT (service_id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			domain = EXCLUDED.domain,
			active_pool_id = EXCLUDED.active_pool_id,
			lifecycle_state = EXCLUDED.lifecycle_state,
			healthy_pools = COALESCE(EXCLUDED.healthy_pools, '{}'::text[]),
			current_dns_targets = COALESCE(EXCLUDED.current_dns_targets, '{}'::text[]),
			observed_at = now()`,
		service.ID, service.TenantID, service.Domain, nullUUID(service.ActivePoolID),
		"Active", pq.Array(healthyPools), pq.Array(currentTargets))
	if err != nil {
		return fmt.Errorf("upsert gslb read model: %w", err)
	}
	return nil
}

func (s *SwitchRequestStore) activePoolMembers(ctx context.Context, poolID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cluster_id FROM gslb_pool_members
		WHERE pool_id = $1 AND enabled = true
		ORDER BY cluster_id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var member string
		if err := rows.Scan(&member); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func nullUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
