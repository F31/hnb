package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// TestSwitchRequestStoreFailoverDecision 验证控制器故障决策（GSLB-005）：
// 全部失健康时创建 PendingApproval 请求 + Outbox 事件，且幂等。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/store/ -run FailoverDecision
func TestSwitchRequestStoreFailoverDecision(t *testing.T) {
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

	tenantID := "tenant-gslbc-" + uuid.NewString()[:8]
	domain := "api-" + uuid.NewString()[:8] + ".hnb.cloud"
	store := NewSwitchRequestStore(db)

	// 创建服务 + active/backup 池
	serviceID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval)
		VALUES ($1, $2, 'svc', $3, 'dns', 'Active', true)`, serviceID, tenantID, domain); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})
	activePoolID := uuid.NewString()
	backupPoolID := uuid.NewString()
	for _, p := range []struct{ id, name string; priority int; members []string }{
		{activePoolID, "active", 0, []string{"cluster-a"}},
		{backupPoolID, "backup", 1, []string{"cluster-b"}},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO gslb_pools (id, service_id, name, priority) VALUES ($1, $2, $3, $4)`,
			p.id, serviceID, p.name, p.priority); err != nil {
			t.Fatal(err)
		}
		for _, member := range p.members {
			if _, err := db.ExecContext(ctx, `
				INSERT INTO gslb_pool_members (id, pool_id, cluster_id, weight, enabled, healthy)
				VALUES ($1, $2, $3, 100, true, true)`, uuid.NewString(), p.id, member); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE gslb_services SET active_pool_id = $2 WHERE id = $1`,
		serviceID, activePoolID); err != nil {
		t.Fatal(err)
	}

	// 决策：创建自动故障转移请求（PendingApproval）
	created, err := store.EnsureFailoverForDomain(ctx, domain)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected failover request to be created")
	}
	active, err := store.HasActiveFailoverRequest(ctx, serviceID)
	if err != nil || !active {
		t.Fatalf("active failover = %v err=%v", active, err)
	}

	var status, intentKind string
	var outboxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT status, intent_kind FROM gslb_switch_requests WHERE service_id = $1`, serviceID).
		Scan(&status, &intentKind); err != nil {
		t.Fatal(err)
	}
	if status != RequestStatusPendingApproval || intentKind != "gslb.failover" {
		t.Fatalf("status=%s kind=%s", status, intentKind)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, serviceID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox events = %d", outboxCount)
	}

	// 幂等：未终态时重复决策不新建
	again, err := store.EnsureFailoverForDomain(ctx, domain)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("duplicate failover request must be suppressed")
	}
	var requestCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM gslb_switch_requests WHERE service_id = $1`, serviceID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d", requestCount)
	}

	// 平台 Operation 行统一接线：自动故障转移请求建立关联 operations 行
	var requestID, operationID, opStatus string
	if err := db.QueryRowContext(ctx, `
		SELECT id, operation_id::text FROM gslb_switch_requests WHERE service_id = $1`, serviceID).
		Scan(&requestID, &operationID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM operations WHERE id = $1`, operationID).Scan(&opStatus); err != nil {
		t.Fatal(err)
	}
	if opStatus != "pending_approval" {
		t.Fatalf("auto failover operation status = %s", opStatus)
	}

	// 执行结果同步：Approved → Dispatched → Succeeded 驱动 Operation 状态机
	for _, step := range []struct{ from []string; to string }{
		{[]string{RequestStatusPendingApproval}, RequestStatusApproved},
		{[]string{RequestStatusApproved}, RequestStatusDispatched},
		{[]string{RequestStatusDispatched}, RequestStatusSucceeded},
	} {
		if err := store.Transition(ctx, requestID, step.from, step.to, ""); err != nil {
			t.Fatal(err)
		}
	}
	var completedSteps, totalSteps int
	if err := db.QueryRowContext(ctx, `
		SELECT status, completed_steps, total_steps FROM operations WHERE id = $1`, operationID).
		Scan(&opStatus, &completedSteps, &totalSteps); err != nil {
		t.Fatal(err)
	}
	if opStatus != "succeeded" || completedSteps != totalSteps || totalSteps != 3 {
		t.Fatalf("operation after succeed: status=%s completed=%d/%d", opStatus, completedSteps, totalSteps)
	}
	var succeededSteps, skippedSteps int
	if err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'succeeded'),
			count(*) FILTER (WHERE status = 'skipped')
		FROM operation_steps WHERE operation_id = $1`, operationID).
		Scan(&succeededSteps, &skippedSteps); err != nil {
		t.Fatal(err)
	}
	if succeededSteps != 2 || skippedSteps != 1 {
		t.Fatalf("operation steps: succeeded=%d skipped=%d", succeededSteps, skippedSteps)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM operation_read_model WHERE operation_id = $1`, operationID).Scan(&opStatus); err != nil {
		t.Fatal(err)
	}
	if opStatus != "succeeded" {
		t.Fatalf("operation read model status = %s", opStatus)
	}

	// GSLB-007：健康投影（切换成功后活跃池已翻转为 backup；其成员 cluster-b
	// 健康 → backup 池为健康池且当前 DNS 目标为 cluster-b）
	if err := store.ProjectReadModel(ctx, domain, []string{"cluster-b"}); err != nil {
		t.Fatal(err)
	}
	var healthyPools, currentTargets []string
	if err := db.QueryRowContext(ctx, `
		SELECT healthy_pools, current_dns_targets FROM gslb_read_model WHERE service_id = $1`, serviceID).
		Scan(pq.Array(&healthyPools), pq.Array(&currentTargets)); err != nil {
		t.Fatal(err)
	}
	if len(healthyPools) != 1 || healthyPools[0] != backupPoolID {
		t.Fatalf("healthy pools = %v", healthyPools)
	}
	if len(currentTargets) != 1 || currentTargets[0] != "cluster-b" {
		t.Fatalf("current targets = %v", currentTargets)
	}

	// 全部失健康 → 投影清空当前目标（不产生任何 DNS 变更）
	if err := store.ProjectReadModel(ctx, domain, nil); err != nil {
		t.Fatal(err)
	}
	currentTargets = nil
	if err := db.QueryRowContext(ctx, `
		SELECT current_dns_targets FROM gslb_read_model WHERE service_id = $1`, serviceID).
		Scan(pq.Array(&currentTargets)); err != nil {
		t.Fatal(err)
	}
	if len(currentTargets) != 0 {
		t.Fatalf("current targets after unhealth = %v", currentTargets)
	}
}

// TestAutoDrillFlow 验证 GSLB 四期定时自动演练（迁移 094 + AUTO_DRILL_INTERVAL）：
// Active 服务幂等创建 drill 请求 + 报告 + Read Model last_drill_*，仅 Active 服务入选，
// 同周期幂等键不重复建单；同时验证 StrictestHealthConfigs 聚合。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/store/ -run AutoDrill
func TestAutoDrillFlow(t *testing.T) {
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

	tenantID := "tenant-drill-" + uuid.NewString()[:8]
	suffix := uuid.NewString()[:8]
	store := NewSwitchRequestStore(db)

	serviceID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval,
			health_protocol, health_path, health_interval_seconds, health_timeout_seconds, health_failure_threshold)
		VALUES ($1, $2, $3, $4, 'dns', 'Active', true, 'https', '/readyz', 15, 5, 4)`,
		serviceID, tenantID, "svc-"+suffix, "drill-"+suffix+".hnb.cloud"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_steps WHERE operation_id IN (SELECT id FROM operations WHERE tenant_id = $1)`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})
	poolID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_pools (id, service_id, name, priority) VALUES ($1, $2, 'active', 0)`, poolID, serviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_pool_members (id, pool_id, cluster_id, weight, enabled, healthy)
		VALUES ($1, $2, $3, 100, true, true)`, uuid.NewString(), poolID, "cluster-drill-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gslb_services SET active_pool_id = $2 WHERE id = $1`,
		serviceID, poolID); err != nil {
		t.Fatal(err)
	}

	// 候选列表包含该服务（Active），终结态服务被排除
	candidates, err := store.ListDrillCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	var candidate DrillService
	for _, svc := range candidates {
		if svc.ID == serviceID {
			found = true
			candidate = svc
		}
		if svc.LifecycleState == "Terminated" || svc.LifecycleState == "Disabled" {
			t.Fatalf("terminal service must not be a candidate: %+v", svc)
		}
	}
	if !found || candidate.LifecycleState != "Active" {
		t.Fatalf("candidate missing: found=%v %+v", found, candidate)
	}

	// 健康配置聚合：读到服务级模板（专用集群名，不受其他服务引用干扰）
	configs, err := store.StrictestHealthConfigs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg, ok := configs["cluster-drill-"+suffix]
	if !ok {
		t.Fatalf("health config missing for drill cluster (configs=%v)", configs)
	}
	if cfg.Path != "/readyz" || cfg.IntervalSeconds != 15 || cfg.TimeoutSeconds != 5 || cfg.FailureThreshold != 4 {
		t.Fatalf("health config = %+v", cfg)
	}

	// 创建自动演练（幂等键带周期序号）
	key := "auto-drill-" + serviceID + "-1"
	created, err := store.EnsureAutoDrill(ctx, candidate, key)
	if err != nil || !created {
		t.Fatalf("auto drill: created=%v err=%v", created, err)
	}
	var status, intentKind, verdict string
	if err := db.QueryRowContext(ctx, `
		SELECT r.status, r.intent_kind, d.verdict
		FROM gslb_switch_requests r
		JOIN gslb_drill_reports d ON d.request_id = r.id
		WHERE r.service_id = $1`, serviceID).Scan(&status, &intentKind, &verdict); err != nil {
		t.Fatal(err)
	}
	if status != "DrillCompleted" || intentKind != "gslb.drill" || verdict != "Ready" {
		t.Fatalf("status=%s kind=%s verdict=%s", status, intentKind, verdict)
	}
	// Read Model 最近演练投影
	var lastVerdict string
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(last_drill_verdict, '') FROM gslb_read_model WHERE service_id = $1`, serviceID).
		Scan(&lastVerdict); err != nil {
		t.Fatal(err)
	}
	if lastVerdict != "Ready" {
		t.Fatalf("read model drill verdict = %s", lastVerdict)
	}

	// 幂等：同 key 重复不新建
	again, err := store.EnsureAutoDrill(ctx, candidate, key)
	if err != nil || again {
		t.Fatalf("duplicate auto drill must be suppressed: again=%v err=%v", again, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM gslb_switch_requests WHERE service_id = $1`, serviceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("request count = %d", count)
	}
	// 下一周期（新 key）可再建
	next, err := store.EnsureAutoDrill(ctx, candidate, "auto-drill-"+serviceID+"-2")
	if err != nil || !next {
		t.Fatalf("next period drill: next=%v err=%v", next, err)
	}
}
