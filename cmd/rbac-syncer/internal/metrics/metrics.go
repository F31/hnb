package metrics

import (
	"sync"
	"time"

	"k8s.io/klog/v2"
)

var (
	mu sync.RWMutex

	syncLatencies  []time.Duration
	syncCount      int
	syncFailed     int
	managedRBBindings int
	syncErrors     []syncError
	maxErrorLog    = 100
)

type syncError struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Namespace string    `json:"namespace,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
}

func RecordSyncLatency(d time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	syncLatencies = append(syncLatencies, d)
	if len(syncLatencies) > 1000 {
		syncLatencies = syncLatencies[len(syncLatencies)-1000:]
	}
}

func RecordSyncResult(success bool) {
	mu.Lock()
	defer mu.Unlock()
	syncCount++
	if !success {
		syncFailed++
	}
}

func RecordManagedRoleBindings(count int) {
	mu.Lock()
	defer mu.Unlock()
	managedRBBindings = count
}

func RecordSyncError(errMsg, namespace, userID string) {
	mu.Lock()
	defer mu.Unlock()
	syncErrors = append(syncErrors, syncError{
		Timestamp: time.Now(),
		Message:   errMsg,
		Namespace: namespace,
		UserID:    userID,
	})
	if len(syncErrors) > maxErrorLog {
		syncErrors = syncErrors[len(syncErrors)-maxErrorLog:]
	}
}

func GetSyncLatencyP99() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	if len(syncLatencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(syncLatencies))
	copy(sorted, syncLatencies)
	sortDurations(sorted)

	idx := int(float64(len(sorted)) * 0.99)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func GetSyncLatencyP50() time.Duration {
	mu.RLock()
	defer mu.RUnlock()
	if len(syncLatencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(syncLatencies))
	copy(sorted, syncLatencies)
	sortDurations(sorted)

	return sorted[len(sorted)/2]
}

func GetSyncCountTotal() int {
	mu.RLock()
	defer mu.RUnlock()
	return syncCount
}

func GetSyncFailedTotal() int {
	mu.RLock()
	defer mu.RUnlock()
	return syncFailed
}

func GetManagedRoleBindings() int {
	mu.RLock()
	defer mu.RUnlock()
	return managedRBBindings
}

func GetRecentErrors() []syncError {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]syncError, len(syncErrors))
	copy(result, syncErrors)
	return result
}

func sortDurations(d []time.Duration) {
	for i := 0; i < len(d); i++ {
		for j := i + 1; j < len(d); j++ {
			if d[j] < d[i] {
				d[i], d[j] = d[j], d[i]
			}
		}
	}
}

func LogMetricsSummary() {
	mu.RLock()
	defer mu.RUnlock()
	klog.Infof("Metrics summary: total_syncs=%d failed=%d managed_rb=%d p50_latency=%s p99_latency=%s",
		syncCount, syncFailed, managedRBBindings, GetSyncLatencyP50(), GetSyncLatencyP99())
}
