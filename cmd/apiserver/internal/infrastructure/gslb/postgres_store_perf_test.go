package gslb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/google/uuid"
)

// TestReadModelQueryP95Budget 是 GSLB Read Model 查询的性能预算采样（GSLB-007 /
// design Assessment：查询读 Read Model，P95 < 200ms）。
// 在 PG16 上批量写入投影行后采样列表/详情查询时延。
// 运行：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/infrastructure/gslb/ -run ReadModelQueryP95 -v
func TestReadModelQueryP95Budget(t *testing.T) {
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

	tenantID := "tenant-gslbperf-" + uuid.NewString()[:8]
	store := NewPostgresStore(db)

	const serviceCount = 200
	var firstServiceID string
	for i := 0; i < serviceCount; i++ {
		serviceID := uuid.NewString()
		if i == 0 {
			firstServiceID = serviceID
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO gslb_services (id, tenant_id, name, domain, routing_mode, lifecycle_state, require_approval)
			VALUES ($1, $2, $3, $4, 'dns', 'Active', true)`,
			serviceID, tenantID, fmt.Sprintf("perf-%d", i), fmt.Sprintf("perf-%d.hnb.cloud", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO gslb_read_model (service_id, tenant_id, domain, lifecycle_state, healthy_pools, current_dns_targets)
			VALUES ($1, $2, $3, 'Active', $4, $5)`,
			serviceID, tenantID, fmt.Sprintf("perf-%d.hnb.cloud", i),
			pq.Array([]string{uuid.NewString()}), pq.Array([]string{"cluster-a", "cluster-b"})); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM gslb_services WHERE tenant_id = $1`, tenantID)
	})

	p95 := func(samples []time.Duration) time.Duration {
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[int(float64(len(samples))*0.95)-1]
	}

	const iterations = 200
	listSamples := make([]time.Duration, 0, iterations)
	getSamples := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if _, err := store.ListReadModels(ctx, tenantID); err != nil {
			t.Fatal(err)
		}
		listSamples = append(listSamples, time.Since(start))

		start = time.Now()
		if _, ok, err := store.GetReadModel(ctx, firstServiceID, tenantID); err != nil || !ok {
			t.Fatalf("get read model: ok=%v err=%v", ok, err)
		}
		getSamples = append(getSamples, time.Since(start))
	}

	listP95 := p95(listSamples)
	getP95 := p95(getSamples)
	t.Logf("ListReadModels P95 = %v (n=%d, services=%d)", listP95, iterations, serviceCount)
	t.Logf("GetReadModel P95 = %v (n=%d)", getP95, iterations)

	const budget = 200 * time.Millisecond
	if listP95 > budget {
		t.Fatalf("ListReadModels P95 %v exceeds budget %v", listP95, budget)
	}
	if getP95 > budget {
		t.Fatalf("GetReadModel P95 %v exceeds budget %v", getP95, budget)
	}
}
