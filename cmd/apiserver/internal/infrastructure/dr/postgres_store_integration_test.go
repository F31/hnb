package dr

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	drapp "github.com/F31/hnb/cmd/apiserver/internal/application/dr"
)

// TestPostgresStoreSwitchRunFlow 验证迁移 083 上的受控写路径：
// 保护组/成员读写、切换运行创建（同事务 operations/operation_read_model/Outbox）、
// 状态推进同步 Operation 行、GSLB 池目标解析与子请求终态聚合。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/dr/ -run PostgresStore
func TestPostgresStoreSwitchRunFlow(t *testing.T) {
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

	tenantID := "tenant-dr-" + uuid.NewString()[:8]
	suffix := uuid.NewString()[:8]
	store := NewPostgresStore(db)
	now := time.Now().UTC()

	group := drapp.Group{
		ID: uuid.NewString(), TenantID: tenantID, Name: "region-" + suffix,
		PrimaryRegion: "cn-east", StandbyRegion: "cn-north",
		LifecycleState: "Ready", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM dr_protection_groups WHERE id = $1`, group.ID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, group.ID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operation_read_model WHERE tenant_id = $1`, tenantID)
		_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE tenant_id = $1`, tenantID)
	})

	// 组读取与租户隔离
	if got, ok, err := store.GetGroup(ctx, group.ID, tenantID); err != nil || !ok || got.Name != group.Name {
		t.Fatalf("get group: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetGroup(ctx, group.ID, "tenant-other"); err != nil || ok {
		t.Fatalf("cross-tenant group read must be denied: ok=%v err=%v", ok, err)
	}

	// 成员：流量层 + 数据层引用
	gslbMember := drapp.Member{
		ID: uuid.NewString(), GroupID: group.ID, MemberType: drapp.MemberGSLBService,
		RefID: uuid.NewString(), Name: "traffic-api", CreatedAt: now,
	}
	dataMember := drapp.Member{
		ID: uuid.NewString(), GroupID: group.ID, MemberType: drapp.MemberDataLayer,
		RefID: "postgres-main", Name: "data-pg", CreatedAt: now,
	}
	if err := store.AddMember(ctx, gslbMember); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, dataMember); err != nil {
		t.Fatal(err)
	}
	members, err := store.ListMembers(ctx, group.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %v err=%v", members, err)
	}

	// 创建切换运行：同事务 operations / operation_read_model / outbox
	run := drapp.SwitchRun{
		ID: uuid.NewString(), GroupID: group.ID, TenantID: tenantID,
		Direction: drapp.DirectionFailover, Status: drapp.RunDataLayerPending,
		IdempotencyKey: "it-key-" + suffix, CorrelationID: uuid.NewString(),
		OperationID: uuid.NewString(), TrafficRequestIDs: []string{},
		Reason: "integration", ActorID: "subject-it", CreatedAt: now, UpdatedAt: now,
	}
	event := drapp.OutboxEvent{
		MessageID: uuid.NewString(), MessageType: drapp.EventSwitchInitiated,
		SchemaVersion: "v1", Subject: drapp.EventSwitchInitiated,
		TenantID: tenantID, ActorID: "subject-it", CorrelationID: run.CorrelationID,
		IdempotencyKey: "dr-" + run.ID + "-initiated", AggregateID: group.ID,
		Payload: map[string]any{"runId": run.ID},
	}
	if err := store.CreateRun(ctx, run, []drapp.OutboxEvent{event}); err != nil {
		t.Fatal(err)
	}

	var opType, opStatus string
	if err := db.QueryRowContext(ctx,
		`SELECT operation_type, status FROM operations WHERE id = $1`, run.OperationID).
		Scan(&opType, &opStatus); err != nil {
		t.Fatalf("operation row: %v", err)
	}
	if opType != "switchover" || opStatus != "in_progress" {
		t.Fatalf("operation = %s/%s, want switchover/in_progress", opType, opStatus)
	}
	var readModelStatus string
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM operation_read_model WHERE operation_id = $1`, run.OperationID).
		Scan(&readModelStatus); err != nil {
		t.Fatalf("operation read model: %v", err)
	}
	if readModelStatus != "in_progress" {
		t.Fatalf("read model status = %s", readModelStatus)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM outbox_events WHERE aggregate_id = $1 AND subject = $2`,
		group.ID, drapp.EventSwitchInitiated).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("outbox count = %d err=%v", outboxCount, err)
	}

	// 幂等键读取
	if got, ok, err := store.GetRunByKey(ctx, group.ID, run.IdempotencyKey); err != nil || !ok || got.ID != run.ID {
		t.Fatalf("get run by key: ok=%v err=%v", ok, err)
	}

	// 状态推进到终态：Operation 行同步 succeeded + completed_at
	requestID := uuid.NewString()
	if err := store.UpdateRun(ctx, run.ID, drapp.RunCompleted, map[string]any{
		"traffic_request_ids": []string{requestID},
	}, nil); err != nil {
		t.Fatal(err)
	}
	var completedAt sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT status, completed_at FROM operations WHERE id = $1`, run.OperationID).
		Scan(&opStatus, &completedAt); err != nil {
		t.Fatal(err)
	}
	if opStatus != "succeeded" || !completedAt.Valid {
		t.Fatalf("operation terminal = %s completed=%v", opStatus, completedAt.Valid)
	}
	got, ok, err := store.GetRun(ctx, run.ID, tenantID)
	if err != nil || !ok {
		t.Fatalf("get run: ok=%v err=%v", ok, err)
	}
	if got.Status != drapp.RunCompleted || len(got.TrafficRequestIDs) != 1 || got.TrafficRequestIDs[0] != requestID {
		t.Fatalf("run = %+v", got)
	}
}

// TestPostgresStoreGSLBPoolResolution 验证流量层目标池解析与子请求状态聚合。
func TestPostgresStoreGSLBPoolResolution(t *testing.T) {
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

	tenantID := "tenant-dr-" + uuid.NewString()[:8]
	suffix := uuid.NewString()[:8]
	store := NewPostgresStore(db)

	serviceID := uuid.NewString()
	activePoolID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, active_pool_id, lifecycle_state, require_approval)
		VALUES ($1, $2, $3, $4, 'dns', $5, 'Active', true)`,
		serviceID, tenantID, "api-"+suffix, "api-"+suffix+".hnb.cloud", activePoolID); err != nil {
		t.Fatal(err)
	}
	backupPoolID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_pools (id, service_id, name, priority) VALUES
		($1, $3, 'active', 0), ($2, $3, 'backup', 1)`, activePoolID, backupPoolID, serviceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
	})

	// failover 目标 = 优先级最高的非活跃池；switchback 目标 = 优先级最低主池
	if pool, ok, err := store.GetGSLBBackupPool(ctx, serviceID, ""); err != nil || !ok || pool != backupPoolID {
		t.Fatalf("backup pool = %s ok=%v err=%v, want %s", pool, ok, err, backupPoolID)
	}
	if pool, ok, err := store.GetGSLBPrimaryPool(ctx, serviceID); err != nil || !ok || pool != activePoolID {
		t.Fatalf("primary pool = %s ok=%v err=%v, want %s", pool, ok, err, activePoolID)
	}

	// 子 gslb 请求状态聚合
	requestID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status
		) VALUES ($1, $2, $3, 'gslb.failover', 'digest-it', '{}', $4, $5, true, 'Succeeded')`,
		requestID, tenantID, serviceID, "it-req-"+suffix, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	statuses, err := store.TrafficRequestStatuses(ctx, []string{requestID})
	if err != nil || len(statuses) != 1 || statuses[0] != "Succeeded" {
		t.Fatalf("statuses = %v err=%v", statuses, err)
	}
}
