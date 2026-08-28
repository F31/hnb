package gslb

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/google/uuid"

	appgslb "github.com/F31/hnb/cmd/apiserver/internal/application/gslb"
)

// TestPostgresStoreSwitchRequestFlow 验证迁移 081 上的受控写路径：
// 服务/池/成员读写、切换请求创建（含同事务 Outbox 事件）、状态流转。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/gslb/ -run PostgresStore
func TestPostgresStoreSwitchRequestFlow(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	tenantID := "tenant-gslb-" + uuid.NewString()[:8]
	suffix := uuid.NewString()[:8]
	store := NewPostgresStore(db)

	serviceID, err := store.EnsureService(ctx, tenantID, "api-"+suffix, "api-"+suffix+".hnb.cloud", map[string][]string{
		"active": {"cluster-a"},
		"backup": {"cluster-b", "cluster-c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})

	// 服务租户隔离读取
	if svc, ok, err := store.GetService(ctx, serviceID, tenantID); err != nil || !ok || svc.Name == "" {
		t.Fatalf("get service: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetService(ctx, serviceID, "tenant-other"); err != nil || ok {
		t.Fatalf("cross-tenant service read must be denied: ok=%v err=%v", ok, err)
	}

	// 池成员（backup 池含 cluster-b/cluster-c）
	var backupPoolID string
	rows, err := db.QueryContext(ctx, `SELECT id FROM gslb_pools WHERE service_id = $1 AND name = 'backup'`, serviceID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		if err := rows.Scan(&backupPoolID); err != nil {
			t.Fatal(err)
		}
	}
	rows.Close()
	if backupPoolID == "" {
		t.Fatal("backup pool not found")
	}
	members, err := store.GetPoolMemberClusterIDs(ctx, backupPoolID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %v err=%v", members, err)
	}

	// 创建切换请求（PendingApproval）+ 同事务 outbox 事件 + 平台 Operation 行
	planJSON, _ := json.Marshal(map[string]any{
		"planId": "gslb-plan-it", "intentId": "abc123", "semanticDigest": "abc123",
		"steps": []any{
			map[string]any{"stepId": "apply", "stepType": "gslb_dns_apply", "dependsOn": []string{}, "inputs": map[string]any{"serviceId": serviceID}, "idempotencyKey": "it-key-1-apply", "compensation": "revert"},
			map[string]any{"stepId": "verify", "stepType": "gslb_dns_verify", "dependsOn": []string{"apply"}, "inputs": map[string]any{"serviceId": serviceID}, "idempotencyKey": "it-key-1-verify"},
			map[string]any{"stepId": "revert", "stepType": "gslb_dns_revert", "dependsOn": []string{}, "inputs": map[string]any{"serviceId": serviceID}, "idempotencyKey": "it-key-1-revert"},
		},
	})
	now := time.Now().UTC()
	request := appgslb.SwitchRequest{
		ID: uuid.NewString(), TenantID: tenantID, ServiceID: serviceID,
		IntentKind: "gslb.failover", IntentDigest: "abc123",
		PlanSnapshot: planJSON, IdempotencyKey: "it-key-1",
		CorrelationID: uuid.NewString(), RequireApproval: true,
		Status: appgslb.StatusPendingApproval, ActorID: "actor-a",
		OperationID: uuid.NewString(), DRGroupRef: "dr-group-it",
		CreatedAt: now, UpdatedAt: now,
	}
	events := []appgslb.OutboxEvent{{
		MessageID: uuid.NewString(), MessageType: appgslb.EventIntentSubmitted,
		SchemaVersion: "v1", Subject: appgslb.EventIntentSubmitted,
		TenantID: tenantID, ActorID: "actor-a", CorrelationID: request.CorrelationID,
		IdempotencyKey: "evt-1", AggregateID: serviceID, Payload: map[string]any{"requestId": request.ID},
	}}
	if err := store.CreateSwitchRequest(ctx, request, nil, events); err != nil {
		t.Fatal(err)
	}
	// 幂等键查询
	if got, ok, err := store.GetSwitchRequestByKey(ctx, tenantID, serviceID, "it-key-1"); err != nil || !ok || got.ID != request.ID {
		t.Fatalf("by key: ok=%v err=%v", ok, err)
	}
	// outbox 行
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, serviceID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("outbox events = %d", eventCount)
	}

	// 平台 Operation 行统一接线：同事务建立 operations/operation_steps/read model
	var opType, opStatus string
	var totalSteps int
	if err := db.QueryRowContext(ctx, `
		SELECT operation_type, status, total_steps FROM operations WHERE id = $1`, request.OperationID).
		Scan(&opType, &opStatus, &totalSteps); err != nil {
		t.Fatal(err)
	}
	if opType != "gslb_failover" || opStatus != "pending_approval" || totalSteps != 3 {
		t.Fatalf("operation row: type=%s status=%s steps=%d", opType, opStatus, totalSteps)
	}
	var stepCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM operation_steps WHERE operation_id = $1`, request.OperationID).Scan(&stepCount); err != nil {
		t.Fatal(err)
	}
	if stepCount != 3 {
		t.Fatalf("operation steps = %d", stepCount)
	}
	var rmStatus string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM operation_read_model WHERE operation_id = $1`, request.OperationID).Scan(&rmStatus); err != nil {
		t.Fatal(err)
	}
	if rmStatus != "pending_approval" {
		t.Fatalf("operation read model status = %s", rmStatus)
	}
	// DR 编排来源引用落库（GSLB-009 对接缝）
	if got, ok, err := store.GetSwitchRequest(ctx, request.ID, tenantID); err != nil || !ok || got.DRGroupRef != "dr-group-it" {
		t.Fatalf("drGroupRef = %q ok=%v err=%v", got.DRGroupRef, ok, err)
	}

	// 审批流转：Approved + 派发命令事件
	if err := store.UpdateSwitchRequestStatus(ctx, request.ID, appgslb.StatusApproved, map[string]any{
		"approved_by": "approver-a", "approved_at": time.Now().UTC(),
	}, []appgslb.OutboxEvent{{
		MessageID: uuid.NewString(), MessageType: appgslb.CommandStepRequested,
		SchemaVersion: "v1", Subject: appgslb.CommandStepRequested,
		TenantID: tenantID, ActorID: "approver-a", CorrelationID: request.CorrelationID,
		IdempotencyKey: "evt-2", AggregateID: serviceID, Payload: map[string]any{"requestId": request.ID},
	}}); err != nil {
		t.Fatal(err)
	}
	updated, ok, err := store.GetSwitchRequest(ctx, request.ID, tenantID)
	if err != nil || !ok {
		t.Fatalf("get after approve: ok=%v err=%v", ok, err)
	}
	if updated.Status != appgslb.StatusApproved || updated.ApprovedBy != "approver-a" {
		t.Fatalf("updated = %+v", updated)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, serviceID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("outbox events after approve = %d", eventCount)
	}

	// 跨租户读取请求被拒绝
	if _, ok, err := store.GetSwitchRequest(ctx, request.ID, "tenant-other"); err != nil || ok {
		t.Fatalf("cross-tenant request read must be denied: ok=%v err=%v", ok, err)
	}

	// 审批后 Operation 行同步为 queued（Operation Center 可见）
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM operations WHERE id = $1`, request.OperationID).Scan(&opStatus); err != nil {
		t.Fatal(err)
	}
	if opStatus != "queued" {
		t.Fatalf("operation status after approve = %s", opStatus)
	}
}

// TestPostgresStoreDrillReportFlow 验证 GSLB-010 结构化演练报告落库：
// drill 报告 + Read Model 最近演练 + Operation 行（gslb_drill / succeeded）。
func TestPostgresStoreDrillReportFlow(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	tenantID := "tenant-gslb-" + uuid.NewString()[:8]
	suffix := uuid.NewString()[:8]
	store := NewPostgresStore(db)

	serviceID, err := store.EnsureService(ctx, tenantID, "drill-"+suffix, "drill-"+suffix+".hnb.cloud", map[string][]string{
		"active": {"cluster-a"},
		"backup": {"cluster-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})

	planJSON, _ := json.Marshal(map[string]any{
		"planId": "gslb-plan-drill", "intentId": "drill123", "semanticDigest": "drill123",
		"steps": []any{
			map[string]any{"stepId": "compute", "stepType": "gslb_drill_compute", "dependsOn": []string{}, "inputs": map[string]any{"serviceId": serviceID}, "idempotencyKey": "it-drill-compute"},
		},
	})
	now := time.Now().UTC()
	request := appgslb.SwitchRequest{
		ID: uuid.NewString(), TenantID: tenantID, ServiceID: serviceID,
		IntentKind: "gslb.drill", IntentDigest: "drill123",
		PlanSnapshot: planJSON, IdempotencyKey: "it-drill-key",
		CorrelationID: uuid.NewString(), RequireApproval: false,
		Status: appgslb.StatusDrillCompleted, ActorID: "actor-a",
		OperationID: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	}
	reportJSON, _ := json.Marshal(map[string]any{
		"serviceId": serviceID, "domain": "drill-" + suffix + ".hnb.cloud",
		"currentTargets": []string{"cluster-a"}, "projectedTargets": []string{"cluster-b"},
		"checks": []any{map[string]any{"name": "target-pool-has-members", "passed": true}},
		"verdict": "Ready", "generatedAt": now,
	})
	drill := &appgslb.DrillReport{
		ID: uuid.NewString(), TenantID: tenantID, ServiceID: serviceID,
		RequestID: request.ID, Verdict: appgslb.DrillVerdictReady,
		Report: reportJSON, CreatedAt: now,
	}
	if err := store.CreateSwitchRequest(ctx, request, drill, nil); err != nil {
		t.Fatal(err)
	}

	// 演练报告可查询（租户隔离）
	reports, err := store.ListDrillReports(ctx, serviceID, tenantID)
	if err != nil || len(reports) != 1 {
		t.Fatalf("drill reports = %v err=%v", len(reports), err)
	}
	if reports[0].Verdict != appgslb.DrillVerdictReady || reports[0].RequestID != request.ID {
		t.Fatalf("drill report = %+v", reports[0])
	}
	if cross, err := store.ListDrillReports(ctx, serviceID, "tenant-other"); err != nil || len(cross) != 0 {
		t.Fatalf("cross-tenant drill read must be empty: %v err=%v", len(cross), err)
	}

	// Read Model 最近演练（GSLB-010）
	model, ok, err := store.GetReadModel(ctx, serviceID, tenantID)
	if err != nil || !ok {
		t.Fatalf("read model: ok=%v err=%v", ok, err)
	}
	if model.LastDrillVerdict != appgslb.DrillVerdictReady || model.LastDrillReportID != drill.ID || model.LastDrillAt == nil {
		t.Fatalf("read model drill fields = %+v", model)
	}

	// Operation 行：gslb_drill 立即 succeeded（演练历史进 Operation Center）
	var opType, opStatus string
	var completedSteps int
	if err := db.QueryRowContext(ctx, `
		SELECT operation_type, status, completed_steps FROM operations WHERE id = $1`, request.OperationID).
		Scan(&opType, &opStatus, &completedSteps); err != nil {
		t.Fatal(err)
	}
	if opType != "gslb_drill" || opStatus != "succeeded" || completedSteps != 1 {
		t.Fatalf("drill operation: type=%s status=%s completed=%d", opType, opStatus, completedSteps)
	}
}
