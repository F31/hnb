package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/F31/hnb/pkg/gslb"
)

// DrillService 是演练候选服务（含生命周期，供调度器过滤 Active）。
type DrillService struct {
	ServiceRow
	LifecycleState string
}

// ListDrillCandidates 返回未终结的 GSLB 服务（调度器按生命周期过滤 Active）。
func (s *SwitchRequestStore) ListDrillCandidates(ctx context.Context) ([]DrillService, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, domain, COALESCE(active_pool_id::text, ''), lifecycle_state
		FROM gslb_services
		WHERE lifecycle_state NOT IN ('Terminated', 'Disabled')`)
	if err != nil {
		return nil, fmt.Errorf("gslb drill candidates: %w", err)
	}
	defer rows.Close()
	var services []DrillService
	for rows.Next() {
		var svc DrillService
		if err := rows.Scan(&svc.ID, &svc.TenantID, &svc.Domain, &svc.ActivePoolID, &svc.LifecycleState); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

// EnsureAutoDrill 幂等创建只读自动演练（GSLB 四期）：复用 drill 落库路径——
// 切换请求（DrillCompleted）+ 平台 Operation 行 + 结构化演练报告 +
// Read Model last_drill_* + Outbox 事件；不产生任何真实 DNS 变更。
// 幂等键带周期序号，由 (tenant_id, service_id, idempotency_key) 唯一约束兜底。
func (s *SwitchRequestStore) EnsureAutoDrill(ctx context.Context, service DrillService, idempotencyKey string) (bool, error) {
	var existing int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM gslb_switch_requests
		WHERE tenant_id = $1 AND service_id = $2 AND idempotency_key = $3`,
		service.TenantID, service.ID, idempotencyKey).Scan(&existing); err != nil {
		return false, fmt.Errorf("check auto drill key: %w", err)
	}
	if existing > 0 {
		return false, nil
	}

	intent := &gslb.Intent{
		APIVersion: gslb.APIVersion,
		Kind:       gslb.IntentDrill,
		ServiceID:  service.ID,
		TenantID:   service.TenantID,
		Reason:     "auto-drill: scheduled periodic drill",
		Metadata: gslb.IntentMetadata{
			IdempotencyKey: idempotencyKey,
			CorrelationID:  uuid.NewString(),
		},
	}
	plan, err := intent.BuildPlan(gslb.PlanInput{
		ServiceID: service.ID, TenantID: service.TenantID, Domain: service.Domain,
	})
	if err != nil {
		return false, err
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return false, err
	}

	// 只读演练报告（GSLB-010 同构）：评估当前流量分布，不假设目标池。
	var currentTargets []string
	if service.ActivePoolID != "" {
		currentTargets, err = s.activePoolMembers(ctx, service.ActivePoolID)
		if err != nil {
			return false, err
		}
	}
	if currentTargets == nil {
		currentTargets = []string{}
	}
	verdict := "Ready"
	checks := []map[string]any{{
		"name":   "current-targets-known",
		"passed": service.ActivePoolID == "" || len(currentTargets) > 0,
		"detail": "当前活跃池无启用成员，回滚目标为空",
	}}
	if service.ActivePoolID != "" && len(currentTargets) == 0 {
		verdict = "Blocked"
	}
	reportJSON, err := json.Marshal(map[string]any{
		"serviceId": service.ID, "domain": service.Domain,
		"activePoolId": service.ActivePoolID,
		"currentTargets": currentTargets, "projectedTargets": []string{},
		"checks": checks, "verdict": verdict, "generatedAt": time.Now().UTC(),
	})
	if err != nil {
		return false, err
	}

	now := time.Now().UTC()
	requestID := uuid.NewString()
	operationID := uuid.NewString()
	reportID := uuid.NewString()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// 平台 Operation 行（统一接线）：演练即时完成（与 apiserver drill 路径一致）。
	tagsJSON, _ := json.Marshal(map[string]any{
		"gslbServiceId": service.ID, "switchRequestId": requestID,
		"intentKind": string(intent.Kind), "origin": "auto-drill",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, operation_type, status, initiated_by,
			correlation_id, idempotency_key, plan_digest,
			total_steps, completed_steps, failed_steps, tags, created_at, completed_at
		) VALUES ($1, $2, 'gslb_drill', 'succeeded', 'gslb-controller', $3, $4, $5, $6, $6, 0, $7, $8, $8)`,
		operationID, service.TenantID, intent.Metadata.CorrelationID,
		"gslb-"+requestID, intent.SemanticDigest(), len(plan.Steps), string(tagsJSON), now,
	); err != nil {
		return false, fmt.Errorf("insert auto drill operation: %w", err)
	}
	for _, step := range plan.Steps {
		inputJSON, _ := json.Marshal(step.Inputs)
		outputJSON, _ := json.Marshal(map[string]any{"verdict": verdict, "drillReportId": reportID})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO operation_steps (
				operation_id, plan_step_id, step_name, step_type, status,
				idempotency_key, depends_on, step_input, step_output, completed_at
			) VALUES ($1, $2, $3, $4, 'succeeded', $5, $6, $7, $8, $9)`,
			operationID, step.StepID, step.StepID, string(step.StepType),
			step.IdempotencyKey, pq.Array(step.DependsOn), string(inputJSON), string(outputJSON), now,
		); err != nil {
			return false, fmt.Errorf("insert auto drill operation step: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operation_read_model (
			operation_id, tenant_id, operation_type, status, total_steps,
			completed_steps, failed_steps, initiated_by, summary, tags,
			created_at, completed_at, last_state_changed_at
		) VALUES ($1, $2, 'gslb_drill', 'succeeded', $3, $3, 0, 'gslb-controller', $4, $5, $6, $6, now())`,
		operationID, service.TenantID, len(plan.Steps),
		fmt.Sprintf("gslb_drill %s: succeeded", operationID), string(tagsJSON), now,
	); err != nil {
		return false, fmt.Errorf("insert auto drill operation read model: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, actor_id, reason,
			operation_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, 'DrillCompleted', 'gslb-controller', $9, $10, $11, $11)`,
		requestID, service.TenantID, service.ID, string(intent.Kind), intent.SemanticDigest(),
		string(planJSON), idempotencyKey, intent.Metadata.CorrelationID,
		intent.Reason, operationID, now,
	); err != nil {
		return false, fmt.Errorf("insert auto drill request: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gslb_drill_reports (id, tenant_id, service_id, request_id, verdict, report, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		reportID, service.TenantID, service.ID, requestID, verdict, string(reportJSON), now,
	); err != nil {
		return false, fmt.Errorf("insert auto drill report: %w", err)
	}

	// 最近演练写入 Read Model（GSLB-010）
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
		service.ID, reportID, verdict, now,
	); err != nil {
		return false, fmt.Errorf("project auto drill read model: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"requestId": requestID, "serviceId": service.ID,
		"kind": string(intent.Kind), "digest": intent.SemanticDigest(),
		"reason": "auto-drill",
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, payload
		) VALUES ($1, $2, 'v1', $3, $4, $5, 'gslb-controller', $6, $7, $8, 0, $8, $9)`,
		uuid.NewString(), EventIntentSubmitted, EventIntentSubmitted, now,
		service.TenantID, intent.Metadata.CorrelationID, idempotencyKey, service.ID, string(payload),
	); err != nil {
		return false, fmt.Errorf("insert auto drill outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
