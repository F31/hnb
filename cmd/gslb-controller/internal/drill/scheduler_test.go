package drill

import (
	"context"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
)

type fakeDrillStore struct {
	services []store.DrillService
	keys     []string
}

func (f *fakeDrillStore) ListDrillCandidates(context.Context) ([]store.DrillService, error) {
	return f.services, nil
}

func (f *fakeDrillStore) EnsureAutoDrill(_ context.Context, svc store.DrillService, key string) (bool, error) {
	for _, existing := range f.keys {
		if existing == key {
			return false, nil
		}
	}
	f.keys = append(f.keys, key)
	return true, nil
}

func svcRow(id, lifecycle string) store.DrillService {
	return store.DrillService{
		ServiceRow:     store.ServiceRow{ID: id, TenantID: "tenant-a", Domain: id + ".hnb.cloud"},
		LifecycleState: lifecycle,
	}
}

func TestPeriodKeySequence(t *testing.T) {
	interval := time.Hour
	at := time.Unix(1_700_000_000, 0)
	key1 := PeriodKey("svc-1", at, interval)
	key2 := PeriodKey("svc-1", at.Add(interval), interval)
	if key1 == key2 {
		t.Fatalf("period key must rotate per interval: %s", key1)
	}
	// 同周期内幂等键稳定
	if again := PeriodKey("svc-1", at.Add(time.Minute), interval); again != key1 {
		t.Fatalf("same period must produce same key: %s vs %s", key1, again)
	}
	// 不同服务不同键
	if other := PeriodKey("svc-2", at, interval); other == key1 {
		t.Fatal("period key must include service id")
	}
}

func TestRunOnceOnlyActiveServices(t *testing.T) {
	fake := &fakeDrillStore{services: []store.DrillService{
		svcRow("svc-active", "Active"),
		svcRow("svc-inactive", "Inactive"),
		svcRow("svc-paused", "Paused"),
		svcRow("svc-degraded", "Degraded"),
	}}
	scheduler := NewScheduler(fake, time.Hour)
	scheduler.RunOnce(context.Background())

	if len(fake.keys) != 1 {
		t.Fatalf("drills created = %v", fake.keys)
	}
	if fake.keys[0] != PeriodKey("svc-active", scheduler.now().UTC(), time.Hour) {
		t.Fatalf("unexpected drill key %s", fake.keys[0])
	}
}

func TestRunOnceIdempotentWithinPeriod(t *testing.T) {
	fake := &fakeDrillStore{services: []store.DrillService{svcRow("svc-active", "Active")}}
	scheduler := NewScheduler(fake, time.Hour)
	scheduler.RunOnce(context.Background())
	scheduler.RunOnce(context.Background())

	if len(fake.keys) != 1 {
		t.Fatalf("duplicate drill in same period: %v", fake.keys)
	}
}
