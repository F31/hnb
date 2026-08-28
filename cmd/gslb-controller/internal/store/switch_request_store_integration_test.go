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

	// GSLB-007：健康投影（cluster-a 健康 → active 池为健康池且为目标）
	if err := store.ProjectReadModel(ctx, domain, []string{"cluster-a"}); err != nil {
		t.Fatal(err)
	}
	var healthyPools, currentTargets []string
	if err := db.QueryRowContext(ctx, `
		SELECT healthy_pools, current_dns_targets FROM gslb_read_model WHERE service_id = $1`, serviceID).
		Scan(pq.Array(&healthyPools), pq.Array(&currentTargets)); err != nil {
		t.Fatal(err)
	}
	if len(healthyPools) != 1 || healthyPools[0] != activePoolID {
		t.Fatalf("healthy pools = %v", healthyPools)
	}
	if len(currentTargets) != 1 || currentTargets[0] != "cluster-a" {
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
