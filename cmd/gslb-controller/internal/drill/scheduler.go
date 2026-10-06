// Package drill 实现 GSLB 四期定时自动演练：AUTO_DRILL_INTERVAL 开启时，
// 对每个 Active 服务周期性创建 gslb.drill 类型切换请求（只读演练，
// 复用既有 drill 落库路径：演练报告 + Read Model last_drill_*，
// 不产生任何真实 DNS 变更）。
package drill

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
)

// Store 是调度器依赖的存储（接口便于测试）。
type Store interface {
	// ListDrillCandidates 返回未终结的 GSLB 服务（含生命周期状态）。
	ListDrillCandidates(ctx context.Context) ([]store.DrillService, error)
	// EnsureAutoDrill 幂等创建只读演练请求 + 报告；返回是否新建。
	EnsureAutoDrill(ctx context.Context, service store.DrillService, idempotencyKey string) (bool, error)
}

// Scheduler 周期性为 Active 服务触发只读演练。
type Scheduler struct {
	store    Store
	interval time.Duration
	now      func() time.Time
}

func NewScheduler(store Store, interval time.Duration) *Scheduler {
	return &Scheduler{store: store, interval: interval, now: time.Now}
}

// PeriodKey 生成带周期序号的幂等键：同一周期内重试/多实例不重复建单。
func PeriodKey(serviceID string, at time.Time, interval time.Duration) string {
	seq := at.Unix() / int64(interval.Seconds())
	return fmt.Sprintf("auto-drill-%s-%d", serviceID, seq)
}

// RunOnce 执行一轮：仅对 Active 服务创建本周期演练。
func (s *Scheduler) RunOnce(ctx context.Context) {
	services, err := s.store.ListDrillCandidates(ctx)
	if err != nil {
		log.Printf("[auto-drill] list services failed: %v", err)
		return
	}
	at := s.now().UTC()
	for _, svc := range services {
		if svc.LifecycleState != "Active" {
			continue
		}
		created, err := s.store.EnsureAutoDrill(ctx, svc, PeriodKey(svc.ID, at, s.interval))
		if err != nil {
			log.Printf("[auto-drill] drill for service %s failed: %v", svc.ID, err)
			continue
		}
		if created {
			log.Printf("[auto-drill] drill created for service %s (%s)", svc.ID, svc.Domain)
		}
	}
}

// Start 按周期运行，直至 ctx 取消。
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.RunOnce(ctx)
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}
