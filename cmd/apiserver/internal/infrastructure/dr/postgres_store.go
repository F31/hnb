// Package dr 提供 DRProtectionGroup 容灾编排（迁移 083）的 Postgres 仓储实现。
package dr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	drapp "github.com/F31/hnb/cmd/apiserver/internal/application/dr"
)

// PostgresStore 访问 DR 平台状态（迁移 083）。
type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) CreateGroup(ctx context.Context, group drapp.Group) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dr_protection_groups (
			id, tenant_id, name, primary_region, standby_region, lifecycle_state, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		group.ID, group.TenantID, group.Name, group.PrimaryRegion, group.StandbyRegion,
		group.LifecycleState, group.CreatedAt, group.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert dr protection group: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetGroup(ctx context.Context, id, tenantID string) (drapp.Group, bool, error) {
	var group drapp.Group
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, primary_region, standby_region, lifecycle_state, created_at, updated_at
		FROM dr_protection_groups
		WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&group.ID, &group.TenantID, &group.Name, &group.PrimaryRegion, &group.StandbyRegion,
			&group.LifecycleState, &group.CreatedAt, &group.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return drapp.Group{}, false, nil
	}
	if err != nil {
		return drapp.Group{}, false, fmt.Errorf("dr get group: %w", err)
	}
	return group, true, nil
}

func (s *PostgresStore) ListGroups(ctx context.Context, tenantID string) ([]drapp.Group, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, primary_region, standby_region, lifecycle_state, created_at, updated_at
		FROM dr_protection_groups
		WHERE tenant_id = $1
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("dr list groups: %w", err)
	}
	defer rows.Close()
	var groups []drapp.Group
	for rows.Next() {
		var group drapp.Group
		if err := rows.Scan(&group.ID, &group.TenantID, &group.Name, &group.PrimaryRegion, &group.StandbyRegion,
			&group.LifecycleState, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *PostgresStore) AddMember(ctx context.Context, member drapp.Member) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dr_group_members (
			id, group_id, member_type, ref_id, name, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		member.ID, member.GroupID, member.MemberType, member.RefID, member.Name, member.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert dr group member: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListMembers(ctx context.Context, groupID string) ([]drapp.Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, member_type, ref_id, name, created_at
		FROM dr_group_members
		WHERE group_id = $1
		ORDER BY created_at`, groupID)
	if err != nil {
		return nil, fmt.Errorf("dr list members: %w", err)
	}
	defer rows.Close()
	var members []drapp.Member
	for rows.Next() {
		var member drapp.Member
		if err := rows.Scan(&member.ID, &member.GroupID, &member.MemberType,
			&member.RefID, &member.Name, &member.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// GetGSLBBackupPool 返回优先级最高的非活跃池（failover 目标）。
func (s *PostgresStore) GetGSLBBackupPool(ctx context.Context, serviceID, activePoolID string) (string, bool, error) {
	var poolID string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id::text FROM gslb_pools p
		JOIN gslb_services s ON s.id = p.service_id
		WHERE p.service_id = $1
			AND (s.active_pool_id IS NULL OR p.id <> s.active_pool_id)
			AND ($2 = '' OR p.id::text <> $2)
		ORDER BY p.priority DESC LIMIT 1`, serviceID, activePoolID).Scan(&poolID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("dr gslb backup pool: %w", err)
	}
	return poolID, true, nil
}

// GetGSLBPrimaryPool 返回优先级最低的主池（switchback 目标）。
func (s *PostgresStore) GetGSLBPrimaryPool(ctx context.Context, serviceID string) (string, bool, error) {
	var poolID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text FROM gslb_pools WHERE service_id = $1
		ORDER BY priority ASC LIMIT 1`, serviceID).Scan(&poolID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("dr gslb primary pool: %w", err)
	}
	return poolID, true, nil
}

const drRunColumns = `id, group_id, tenant_id, direction, status, idempotency_key, correlation_id,
	COALESCE(operation_id::text, ''), traffic_request_ids,
	COALESCE(reason, ''), COALESCE(error, ''), COALESCE(actor_id, ''),
	created_at, updated_at`

func (s *PostgresStore) GetRunByKey(ctx context.Context, groupID, idempotencyKey string) (drapp.SwitchRun, bool, error) {
	return s.getRun(ctx, `
		SELECT `+drRunColumns+`
		FROM dr_switch_runs
		WHERE group_id = $1 AND idempotency_key = $2`, groupID, idempotencyKey)
}

func (s *PostgresStore) GetRun(ctx context.Context, id, tenantID string) (drapp.SwitchRun, bool, error) {
	return s.getRun(ctx, `
		SELECT `+drRunColumns+`
		FROM dr_switch_runs
		WHERE id = $1 AND tenant_id = $2`, id, tenantID)
}

func (s *PostgresStore) getRun(ctx context.Context, query string, args ...any) (drapp.SwitchRun, bool, error) {
	var run drapp.SwitchRun
	err := s.db.QueryRowContext(ctx, query, args...).
		Scan(&run.ID, &run.GroupID, &run.TenantID, &run.Direction, &run.Status,
			&run.IdempotencyKey, &run.CorrelationID, &run.OperationID,
			pq.Array(&run.TrafficRequestIDs), &run.Reason, &run.Error, &run.ActorID,
			&run.CreatedAt, &run.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return drapp.SwitchRun{}, false, nil
	}
	if err != nil {
		return drapp.SwitchRun{}, false, fmt.Errorf("dr get run: %w", err)
	}
	return run, true, nil
}

func (s *PostgresStore) ListRuns(ctx context.Context, groupID, tenantID string) ([]drapp.SwitchRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+drRunColumns+`
		FROM dr_switch_runs
		WHERE group_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC`, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("dr list runs: %w", err)
	}
	defer rows.Close()
	var runs []drapp.SwitchRun
	for rows.Next() {
		var run drapp.SwitchRun
		if err := rows.Scan(&run.ID, &run.GroupID, &run.TenantID, &run.Direction, &run.Status,
			&run.IdempotencyKey, &run.CorrelationID, &run.OperationID,
			pq.Array(&run.TrafficRequestIDs), &run.Reason, &run.Error, &run.ActorID,
			&run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// operationStatusForRun 映射切换运行状态到平台 Operation 状态机（10 态子集）。
func operationStatusForRun(runStatus string) string {
	switch runStatus {
	case drapp.RunCompleted:
		return "succeeded"
	case drapp.RunFailed:
		return "failed"
	case drapp.RunCancelled:
		return "cancelled"
	default:
		return "in_progress"
	}
}

// CreateRun 同事务写入：平台 operations/operation_read_model 行（Operation Center
// 统一观测）+ dr_switch_runs 行 + Outbox 事件。
func (s *PostgresStore) CreateRun(ctx context.Context, run drapp.SwitchRun, events []drapp.OutboxEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dr run transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 平台 Operation 行须先于切换运行插入（operation_id 外键约束）。
	opStatus := operationStatusForRun(run.Status)
	tagsJSON, err := json.Marshal(map[string]any{
		"drGroupId": run.GroupID,
		"direction": run.Direction,
		"runId":     run.ID,
	})
	if err != nil {
		return fmt.Errorf("marshal dr operation tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key,
			total_steps, completed_steps, failed_steps, tags, created_at
		) VALUES ($1, $2, 'switchover', $3, NULLIF($4, ''), $5, $6, 0, 0, 0, $7, $8)`,
		run.OperationID, run.TenantID, opStatus, run.ActorID,
		run.CorrelationID, "dr-"+run.ID, string(tagsJSON), run.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert dr operation row: %w", err)
	}

	summary := fmt.Sprintf("dr-%s %s: %s", run.Direction, run.OperationID, opStatus)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, operation_type, status, total_steps,
			completed_steps, failed_steps, initiated_by, summary, tags,
			created_at, completed_at, last_state_changed_at
		) VALUES ($1, $2, 'switchover', $3, 0, 0, 0, $4, $5, $6, $7, NULL, now())`,
		run.OperationID, run.TenantID, opStatus, run.ActorID, summary, string(tagsJSON),
		run.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert dr operation read model: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dr_switch_runs (
			id, group_id, tenant_id, direction, status, idempotency_key, correlation_id,
			operation_id, traffic_request_ids, reason, error, actor_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid, $9,
			NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), $13, $14)`,
		run.ID, run.GroupID, run.TenantID, run.Direction, run.Status,
		run.IdempotencyKey, run.CorrelationID, run.OperationID,
		pq.Array(run.TrafficRequestIDs), run.Reason, run.Error, run.ActorID,
		run.CreatedAt, run.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert dr switch run: %w", err)
	}
	for _, event := range events {
		if err := insertOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dr run transaction: %w", err)
	}
	return nil
}

// UpdateRun 同事务更新运行状态并同步关联的平台 Operation 行（终态写 completed_at）。
func (s *PostgresStore) UpdateRun(ctx context.Context, id, status string, fields map[string]any, events []drapp.OutboxEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dr status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	setClause := "status = $2, updated_at = now()"
	args := []any{id, status}
	// 受控字段白名单，防止任意列注入
	if value, ok := fields["traffic_request_ids"]; ok {
		args = append(args, pq.Array(value))
		setClause += fmt.Sprintf(", traffic_request_ids = $%d", len(args))
	}
	if value, ok := fields["error"]; ok {
		args = append(args, value)
		setClause += fmt.Sprintf(", error = $%d", len(args))
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE dr_switch_runs SET `+setClause+` WHERE id = $1`, args...); err != nil {
		return fmt.Errorf("update dr switch run status: %w", err)
	}

	// 同步平台 Operation 行（Operation Center 统一观测）。
	opStatus := operationStatusForRun(status)
	terminal := opStatus == "succeeded" || opStatus == "failed" || opStatus == "cancelled"
	opSet := "status = $2, updated_at = now()"
	if terminal {
		opSet += ", completed_at = now()"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operations SET `+opSet+`
		WHERE id = (SELECT operation_id FROM dr_switch_runs WHERE id = $1)`, id, opStatus); err != nil {
		return fmt.Errorf("sync dr operation status: %w", err)
	}
	readModelSet := "status = $2, last_state_changed_at = now()"
	if terminal {
		readModelSet += ", completed_at = now()"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operation_read_model SET `+readModelSet+`
		WHERE operation_id = (SELECT operation_id FROM dr_switch_runs WHERE id = $1)`, id, opStatus); err != nil {
		return fmt.Errorf("sync dr operation read model: %w", err)
	}
	for _, event := range events {
		if err := insertOutboxEvent(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dr status transaction: %w", err)
	}
	return nil
}

// TrafficRequestStatuses 返回子 gslb 切换请求的当前状态（运行终态聚合）。
func (s *PostgresStore) TrafficRequestStatuses(ctx context.Context, requestIDs []string) ([]string, error) {
	if len(requestIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT status FROM gslb_switch_requests WHERE id = ANY($1)`, pq.Array(requestIDs))
	if err != nil {
		return nil, fmt.Errorf("dr traffic request statuses: %w", err)
	}
	defer rows.Close()
	var statuses []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

func insertOutboxEvent(ctx context.Context, tx *sql.Tx, event drapp.OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal dr outbox payload: %w", err)
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
		event.IdempotencyKey, event.AggregateID, 1,
		event.AggregateID, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert dr outbox event: %w", err)
	}
	return nil
}
