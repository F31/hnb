package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
	"github.com/F31/hnb/pkg/gslb"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// TestConsumerEndToEndNATS 验证完整执行环（GSLB-005）：
// 命令消息经 NATS domain-events 流投递 → consumer 校验请求状态 →
// 执行器执行 → 请求流转 Succeeded。等价于 relay 投递后的真实路径。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> HNB_TEST_NATS_URL=<url> go test ./internal/consumer/ -run EndToEnd
func TestConsumerEndToEndNATS(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	natsURL := os.Getenv("HNB_TEST_NATS_URL")
	if dsn == "" || natsURL == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN and HNB_TEST_NATS_URL are not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tenantID := "tenant-gslbn-" + uuid.NewString()[:8]
	serviceID := uuid.NewString()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval)
		VALUES ($1, $2, 'svc', $3, 'dns', 'Active', true)`,
		serviceID, tenantID, "e2e-"+uuid.NewString()[:8]+".hnb.cloud"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE id = $1`, serviceID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, serviceID)
	})

	// 准备 Approved 的切换请求 + 执行命令（等价于 apiserver 审批后的 outbox 事件）
	requestStore := store.NewSwitchRequestStore(db)
	requestID := uuid.NewString()
	plan := planForTest()
	planJSON, _ := jsonMarshal(plan)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO gslb_switch_requests (
			id, tenant_id, service_id, intent_kind, intent_digest, plan_snapshot,
			idempotency_key, correlation_id, require_approval, status
		) VALUES ($1, $2, $3, 'gslb.failover', 'digest', $4, $5, $6, true, 'Approved')`,
		requestID, tenantID, serviceID, string(planJSON), "it-key-e2e", uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	// 消费者：真实 NATS 连接 + 真实 store + 计数执行器
	js, cleanupNATS, err := Connect(ctx, natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupNATS()

	executed := make(chan struct{}, 1)
	exec := &countingExecutor{notify: executed}
	c := New(js, requestStore, exec)
	consumerErr := make(chan error, 1)
	go func() { consumerErr <- c.Start(ctx) }()

	// 发布执行命令（relay 等价投递：nats.Publish 到 subject，
	// 流捕获 → consumer 拉取）
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	body, _ := jsonMarshal(map[string]any{
		"requestId": requestID,
		"serviceId": serviceID,
		"plan":      json.RawMessage(planJSON),
	})
	// 等待流与消费者就绪后发布
	time.Sleep(2 * time.Second)
	if err := nc.Publish("hnb.event.gslb.step-requested.v1", body); err != nil {
		t.Fatal(err)
	}

	select {
	case <-executed:
	case err := <-consumerErr:
		t.Fatalf("consumer stopped with error: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("executor did not run within timeout")
	}

	// 请求最终状态 Succeeded
	var status string
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := db.QueryRowContext(ctx, `SELECT status FROM gslb_switch_requests WHERE id = $1`, requestID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == store.RequestStatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %s (want Succeeded)", status)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type countingExecutor struct {
	mu     sync.Mutex
	ran    int
	notify chan struct{}
}

func (c *countingExecutor) ExecutePlan(context.Context, *gslb.Plan) error {
	c.mu.Lock()
	c.ran++
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return nil
}
