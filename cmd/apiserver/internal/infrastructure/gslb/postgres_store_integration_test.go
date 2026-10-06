package gslb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

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
		"checks":  []any{map[string]any{"name": "target-pool-has-members", "passed": true}},
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

// TestPostgresStoreListPoolsAndSwitchRequests 验证两个只读查询：
// 地址池拓扑（priority 升序、成员 cluster_id 排序、租户隔离）与
// 切换请求列表（created_at 倒序、status 过滤、租户隔离）。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/gslb/ -run PostgresStore
func TestPostgresStoreListPoolsAndSwitchRequests(t *testing.T) {
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

	serviceID, err := store.EnsureService(ctx, tenantID, "ro-"+suffix, "ro-"+suffix+".hnb.cloud", map[string][]string{
		"active": {"cluster-b", "cluster-a"},
		"backup": {"cluster-c"},
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

	// 地址池拓扑：priority 升序（active 0 → backup 1），成员按 cluster_id 排序
	pools, err := store.ListPools(ctx, serviceID, tenantID)
	if err != nil || len(pools) != 2 {
		t.Fatalf("pools = %v err=%v", len(pools), err)
	}
	if pools[0].Name != "active" || pools[0].Priority != 0 || pools[1].Name != "backup" || pools[1].Priority != 1 {
		t.Fatalf("pool order = %+v", pools)
	}
	if len(pools[0].Members) != 2 || pools[0].Members[0].ClusterID != "cluster-a" || pools[0].Members[1].ClusterID != "cluster-b" {
		t.Fatalf("active members = %+v", pools[0].Members)
	}
	if pools[0].Members[0].Weight != 100 {
		t.Fatalf("member weight = %+v", pools[0].Members[0])
	}
	// 跨租户不可见
	if cross, err := store.ListPools(ctx, serviceID, "tenant-other"); err != nil || len(cross) != 0 {
		t.Fatalf("cross-tenant pools must be empty: %v err=%v", len(cross), err)
	}

	// 两条不同状态的切换请求
	planJSON, _ := json.Marshal(map[string]any{
		"planId": "gslb-plan-ro", "intentId": "ro123", "semanticDigest": "ro123",
		"steps": []any{
			map[string]any{"stepId": "apply", "stepType": "gslb_dns_apply", "dependsOn": []string{}, "inputs": map[string]any{"serviceId": serviceID}, "idempotencyKey": "ro-key-apply"},
		},
	})
	newRequest := func(key, status string, createdAt time.Time) appgslb.SwitchRequest {
		return appgslb.SwitchRequest{
			ID: uuid.NewString(), TenantID: tenantID, ServiceID: serviceID,
			IntentKind: "gslb.failover", IntentDigest: "ro123",
			PlanSnapshot: planJSON, IdempotencyKey: key,
			CorrelationID: uuid.NewString(), RequireApproval: true,
			Status: status, ActorID: "actor-a",
			OperationID: uuid.NewString(), CreatedAt: createdAt, UpdatedAt: createdAt,
		}
	}
	older := newRequest("ro-key-older", appgslb.StatusSucceeded, time.Now().UTC().Add(-time.Hour))
	newer := newRequest("ro-key-newer", appgslb.StatusPendingApproval, time.Now().UTC())
	for _, req := range []appgslb.SwitchRequest{older, newer} {
		if err := store.CreateSwitchRequest(ctx, req, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	// 全量：created_at 倒序
	all, err := store.ListSwitchRequests(ctx, tenantID, serviceID, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("requests = %v err=%v", len(all), err)
	}
	if all[0].ID != newer.ID || all[1].ID != older.ID {
		t.Fatalf("order = %s, %s", all[0].ID, all[1].ID)
	}
	// status 过滤
	pending, err := store.ListSwitchRequests(ctx, tenantID, serviceID, appgslb.StatusPendingApproval)
	if err != nil || len(pending) != 1 || pending[0].ID != newer.ID {
		t.Fatalf("pending = %v err=%v", len(pending), err)
	}
	// 租户隔离
	if cross, err := store.ListSwitchRequests(ctx, "tenant-other", serviceID, ""); err != nil || len(cross) != 0 {
		t.Fatalf("cross-tenant requests must be empty: %v err=%v", len(cross), err)
	}
}

// TestPostgresStoreCreateService 验证 GSLB 三期创建路径（迁移 094）：
// 带权重的池/成员落库、健康检查模板读写、domain 幂等查询、权重落库闭环。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/gslb/ -run CreateService
func TestPostgresStoreCreateService(t *testing.T) {
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

	now := time.Now().UTC()
	activePoolID := uuid.NewString()
	backupPoolID := uuid.NewString()
	svc := appgslb.Service{
		ID: uuid.NewString(), TenantID: tenantID, Name: "svc-" + suffix,
		Domain: "svc-" + suffix + ".hnb.cloud", RoutingMode: "dns",
		ActivePoolID: activePoolID, LifecycleState: "Active", RequireApproval: true,
		HealthCheck: appgslb.HealthCheck{Protocol: "https", Path: "/readyz", IntervalSeconds: 15, TimeoutSeconds: 5, FailureThreshold: 4},
		CreatedAt:   now, UpdatedAt: now,
	}
	pools := []appgslb.Pool{
		{ID: activePoolID, Name: "active", Priority: 0, Members: []appgslb.PoolMember{
			{ClusterID: "cluster-a", Weight: 80}, {ClusterID: "cluster-b", Weight: 20}}},
		{ID: backupPoolID, Name: "backup", Priority: 1, Members: []appgslb.PoolMember{
			{ClusterID: "cluster-c", Weight: 100}}},
	}
	if err := store.CreateService(ctx, svc, svc.HealthCheck, pools); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, svc.ID)
	})

	// 主档 + 健康模板读回
	loaded, ok, err := store.GetService(ctx, svc.ID, tenantID)
	if err != nil || !ok {
		t.Fatalf("get service: ok=%v err=%v", ok, err)
	}
	if loaded.ActivePoolID != activePoolID || loaded.HealthCheck.Protocol != "https" ||
		loaded.HealthCheck.Path != "/readyz" || loaded.HealthCheck.IntervalSeconds != 15 ||
		loaded.HealthCheck.TimeoutSeconds != 5 || loaded.HealthCheck.FailureThreshold != 4 {
		t.Fatalf("loaded = %+v", loaded)
	}

	// domain 幂等解析
	byDomain, ok, err := store.GetServiceByDomain(ctx, tenantID, svc.Domain)
	if err != nil || !ok || byDomain.ID != svc.ID {
		t.Fatalf("get by domain: ok=%v id=%s err=%v", ok, byDomain.ID, err)
	}
	if _, ok, err := store.GetServiceByDomain(ctx, "tenant-other", svc.Domain); err != nil || ok {
		t.Fatalf("cross-tenant domain lookup must miss: ok=%v err=%v", ok, err)
	}

	// 成员权重落库
	listed, err := store.ListPools(ctx, svc.ID, tenantID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("pools = %v err=%v", len(listed), err)
	}
	if len(listed[0].Members) != 2 || listed[0].Members[0].Weight != 80 || listed[0].Members[1].Weight != 20 {
		t.Fatalf("members = %+v", listed[0].Members)
	}

	// 集群存在性（基于 runtime_targets，不再依赖 legacy clusters 表）
	existing, err := store.ExistingTenantClusters(ctx, tenantID, []string{"cluster-a"})
	if err != nil {
		t.Fatal(err)
	}
	if existing["cluster-a"] {
		t.Fatal("cluster-a must not exist for fresh tenant")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runtime_targets (id, tenant_id, name, target_type, status, lifecycle_state)
		VALUES ($1, $2, 'cluster-a', 'kubernetes', 'online', 'ACTIVE')`, uuid.NewString(), tenantID); err != nil {
		t.Fatal(err)
	}
	existing, err = store.ExistingTenantClusters(ctx, tenantID, []string{"cluster-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !existing["cluster-a"] {
		t.Fatal("cluster-a runtime target must be accepted")
	}

	// weight-update 完成后成员权重更新（完成路径落库闭环）
	planJSON, _ := json.Marshal(map[string]any{
		"planId": "p", "intentId": "i", "semanticDigest": "d",
		"steps": []map[string]any{{
			"stepId": "apply", "stepType": "gslb_dns_apply",
			"inputs": map[string]any{"serviceId": svc.ID, "domain": svc.Domain,
				"targets": []string{"cluster-a", "cluster-b"},
				"weights": map[string]int{"cluster-a": 50, "cluster-b": 50}},
			"idempotencyKey": "k-w-apply",
		}},
	})
	reqID := uuid.NewString()
	opID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO operations (id, tenant_id, operation_type, status, initiated_by, correlation_id, idempotency_key, plan_digest, total_steps, tags)
		VALUES ($1, $2, 'gslb_weight_update', 'queued', 'tester', $3, $4, 'd', 1, '{}')`,
		opID, tenantID, uuid.NewString(), "gslb-"+reqID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE operation_id = $1`, opID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_steps WHERE operation_id = $1`, opID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE id = $1`, opID)
	})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status, operation_id, created_at, updated_at
		) VALUES ($1, $2, $3, 'gslb.weight-update', 'd', $4, $5, $6, false, 'Approved', $7, now(), now())`,
		reqID, tenantID, svc.ID, string(planJSON), "k-w-"+suffix, uuid.NewString(), opID); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSwitchRequest(ctx, reqID); err != nil {
		t.Fatal(err)
	}
	var weightA, weightB int
	if err := db.QueryRowContext(ctx, `
		SELECT m1.weight, m2.weight FROM gslb_pool_members m1, gslb_pool_members m2
		WHERE m1.pool_id = $1 AND m1.cluster_id = 'cluster-a'
			AND m2.pool_id = $1 AND m2.cluster_id = 'cluster-b'`, activePoolID).
		Scan(&weightA, &weightB); err != nil {
		t.Fatal(err)
	}
	if weightA != 50 || weightB != 50 {
		t.Fatalf("weights after completion = %d/%d", weightA, weightB)
	}
}

// TestPostgresStoreServiceEditReplaceDelete 验证 GSLB 四期服务编辑/池替换/删除：
// updated_at 乐观并发、改名唯一约束、事务内删旧建新 + active 池重指、
// 进行中切换请求探测、CASCADE 级联清理（pools/members/switch_requests）。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/gslb/ -run ServiceEditReplaceDelete
func TestPostgresStoreServiceEditReplaceDelete(t *testing.T) {
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

	now := time.Now().UTC()
	activePoolID := uuid.NewString()
	svc := appgslb.Service{
		ID: uuid.NewString(), TenantID: tenantID, Name: "edit-" + suffix,
		Domain: "edit-" + suffix + ".hnb.cloud", RoutingMode: "dns",
		ActivePoolID: activePoolID, LifecycleState: "Active", RequireApproval: true,
		HealthCheck: appgslb.DefaultHealthCheck(), CreatedAt: now, UpdatedAt: now,
	}
	pools := []appgslb.Pool{
		{ID: activePoolID, Name: "active", Priority: 0, Members: []appgslb.PoolMember{{ClusterID: "cluster-a", Weight: 100}}},
		{ID: uuid.NewString(), Name: "backup", Priority: 1, Members: []appgslb.PoolMember{{ClusterID: "cluster-b", Weight: 100}}},
	}
	if err := store.CreateService(ctx, svc, svc.HealthCheck, pools); err != nil {
		t.Fatal(err)
	}

	loaded, ok, err := store.GetService(ctx, svc.ID, tenantID)
	if err != nil || !ok {
		t.Fatalf("get service: ok=%v err=%v", ok, err)
	}

	// 编辑：name + 健康检查模板生效
	loaded.Name = "edit-renamed-" + suffix
	loaded.HealthCheck.IntervalSeconds = 30
	loaded.HealthCheck.FailureThreshold = 5
	updated, ok, err := store.UpdateService(ctx, loaded, loaded.UpdatedAt)
	if err != nil || !ok {
		t.Fatalf("update service: ok=%v err=%v", ok, err)
	}
	if updated.Name != loaded.Name || updated.HealthCheck.IntervalSeconds != 30 ||
		updated.HealthCheck.FailureThreshold != 5 {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.UpdatedAt.After(loaded.UpdatedAt) {
		t.Fatalf("updatedAt must advance: %v vs %v", updated.UpdatedAt, loaded.UpdatedAt)
	}

	// 乐观并发：过期 updatedAt → ok=false
	updated.Name = "edit-stale-" + suffix
	if _, ok, err := store.UpdateService(ctx, updated, loaded.UpdatedAt); err != nil || ok {
		t.Fatalf("stale update must be rejected: ok=%v err=%v", ok, err)
	}

	// 改名与既有服务冲突 → ErrConflict（409 语义）
	other := appgslb.Service{
		ID: uuid.NewString(), TenantID: tenantID, Name: "other-" + suffix,
		Domain: "other-" + suffix + ".hnb.cloud", RoutingMode: "dns",
		LifecycleState: "Active", RequireApproval: true,
		HealthCheck: appgslb.DefaultHealthCheck(), CreatedAt: now, UpdatedAt: now,
	}
	otherPool := appgslb.Pool{ID: uuid.NewString(), Name: "active", Priority: 0,
		Members: []appgslb.PoolMember{{ClusterID: "cluster-a", Weight: 100}}}
	if err := store.CreateService(ctx, other, other.HealthCheck, []appgslb.Pool{otherPool}); err != nil {
		t.Fatal(err)
	}
	otherLoaded, _, err := store.GetService(ctx, other.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	otherLoaded.Name = "edit-renamed-" + suffix // 与 svc 当前名字冲突
	if _, _, err := store.UpdateService(ctx, otherLoaded, otherLoaded.UpdatedAt); !errors.Is(err, appgslb.ErrConflict) {
		t.Fatalf("duplicate name must map to ErrConflict, err=%v", err)
	}
	if ok, err := store.DeleteService(ctx, other.ID, tenantID); err != nil || !ok {
		t.Fatalf("cleanup other service: ok=%v err=%v", ok, err)
	}

	// 池整组替换：保留 active 池名字，删旧建新 + active 重指
	current, _, err := store.GetService(ctx, svc.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	newPools := []appgslb.Pool{
		{ID: uuid.NewString(), Name: "active", Priority: 0, Members: []appgslb.PoolMember{
			{ClusterID: "cluster-a", Weight: 60}, {ClusterID: "cluster-c", Weight: 40}}},
		{ID: uuid.NewString(), Name: "dr", Priority: 2, Members: []appgslb.PoolMember{{ClusterID: "cluster-d", Weight: 100}}},
	}
	ok, err = store.ReplaceServicePools(ctx, svc.ID, tenantID, newPools, newPools[0].ID, current.UpdatedAt)
	if err != nil || !ok {
		t.Fatalf("replace pools: ok=%v err=%v", ok, err)
	}
	listed, err := store.ListPools(ctx, svc.ID, tenantID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("pools after replace = %v err=%v", len(listed), err)
	}
	if listed[0].Name != "active" || len(listed[0].Members) != 2 ||
		listed[0].Members[0].Weight != 60 || listed[0].Members[1].Weight != 40 {
		t.Fatalf("replaced pools = %+v", listed)
	}
	if listed[1].Name != "dr" || listed[1].Priority != 2 {
		t.Fatalf("replaced pools = %+v", listed)
	}
	afterReplace, _, err := store.GetService(ctx, svc.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if afterReplace.ActivePoolID != newPools[0].ID {
		t.Fatalf("active pool must be re-pointed to %s, got %s", newPools[0].ID, afterReplace.ActivePoolID)
	}
	// 旧池已删除
	var oldPoolCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM gslb_pools WHERE service_id = $1 AND id = $2`, svc.ID, activePoolID).Scan(&oldPoolCount); err != nil {
		t.Fatal(err)
	}
	if oldPoolCount != 0 {
		t.Fatalf("old pools must be removed, count=%d", oldPoolCount)
	}
	// 过期版本的替换 → ok=false
	if ok, err := store.ReplaceServicePools(ctx, svc.ID, tenantID, newPools, newPools[0].ID, current.UpdatedAt); err != nil || ok {
		t.Fatalf("stale replace must be rejected: ok=%v err=%v", ok, err)
	}

	// 进行中切换请求探测
	inflight, err := store.HasInflightSwitchRequests(ctx, svc.ID, tenantID)
	if err != nil || inflight {
		t.Fatalf("inflight before any request = %v err=%v", inflight, err)
	}
	planJSON, _ := json.Marshal(map[string]any{
		"planId": "gslb-plan-del", "intentId": "del123", "semanticDigest": "del123",
		"steps": []any{
			map[string]any{"stepId": "apply", "stepType": "gslb_dns_apply", "dependsOn": []string{}, "inputs": map[string]any{"serviceId": svc.ID}, "idempotencyKey": "del-key-apply"},
		},
	})
	now = time.Now().UTC()
	request := appgslb.SwitchRequest{
		ID: uuid.NewString(), TenantID: tenantID, ServiceID: svc.ID,
		IntentKind: "gslb.failover", IntentDigest: "del123",
		PlanSnapshot: planJSON, IdempotencyKey: "del-key-1",
		CorrelationID: uuid.NewString(), RequireApproval: true,
		Status: appgslb.StatusPendingApproval, ActorID: "actor-a",
		OperationID: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateSwitchRequest(ctx, request, nil, nil); err != nil {
		t.Fatal(err)
	}
	inflight, err = store.HasInflightSwitchRequests(ctx, svc.ID, tenantID)
	if err != nil || !inflight {
		t.Fatalf("inflight with pending request = %v err=%v", inflight, err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, svc.ID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, svc.ID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})

	// 请求终态化后可删：删除级联清理 pools/members/switch_requests
	if err := store.UpdateSwitchRequestStatus(ctx, request.ID, appgslb.StatusRejected, map[string]any{
		"approved_by": "approver-a", "approved_at": time.Now().UTC(),
	}, nil); err != nil {
		t.Fatal(err)
	}
	inflight, err = store.HasInflightSwitchRequests(ctx, svc.ID, tenantID)
	if err != nil || inflight {
		t.Fatalf("inflight after reject = %v err=%v", inflight, err)
	}
	deleted, err := store.DeleteService(ctx, svc.ID, tenantID)
	if err != nil || !deleted {
		t.Fatalf("delete service: ok=%v err=%v", deleted, err)
	}
	if _, ok, err := store.GetService(ctx, svc.ID, tenantID); err != nil || ok {
		t.Fatalf("deleted service must not be readable: ok=%v err=%v", ok, err)
	}
	for _, table := range []string{"gslb_pools", "gslb_pool_members", "gslb_switch_requests", "gslb_read_model"} {
		var count int
		column := "service_id"
		if table == "gslb_pool_members" {
			if err := db.QueryRowContext(ctx, `
				SELECT count(*) FROM gslb_pool_members m
				JOIN gslb_pools p ON p.id = m.pool_id
				WHERE p.service_id = $1`, svc.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
		} else if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, svc.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s rows must cascade away, count=%d", table, count)
		}
	}
	// 重复删除 → ok=false
	if ok, err := store.DeleteService(ctx, svc.ID, tenantID); err != nil || ok {
		t.Fatalf("re-delete must report not found: ok=%v err=%v", ok, err)
	}
}
