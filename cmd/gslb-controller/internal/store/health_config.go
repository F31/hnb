package store

import (
	"context"
	"fmt"
)

// ClusterHealthConfig 是成员集群维度的健康检查模板（GSLB 四期，迁移 094）。
type ClusterHealthConfig struct {
	Path             string
	IntervalSeconds  int
	TimeoutSeconds   int
	FailureThreshold int
}

// StrictestHealthConfigs 按成员集群聚合所有 GSLB 服务的健康检查模板。
// 简化取舍：同一集群被多服务引用时取"最严格"配置——最短 interval/timeout、
// 最高 failureThreshold；path 无法跨服务合并，取 interval 最短（并列时
// timeout 最短）服务的 path。探测周期本身仍由引擎全局 interval 驱动，
// 服务级 interval 只参与聚合选择，不改变探测频率。
func (s *SwitchRequestStore) StrictestHealthConfigs(ctx context.Context) (map[string]ClusterHealthConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pm.cluster_id, s.health_path, s.health_interval_seconds,
			s.health_timeout_seconds, s.health_failure_threshold
		FROM gslb_pool_members pm
		JOIN gslb_pools p ON p.id = pm.pool_id
		JOIN gslb_services s ON s.id = p.service_id
		WHERE pm.enabled = true
			AND s.lifecycle_state IN ('Active', 'Degraded', 'FailingOver')`)
	if err != nil {
		return nil, fmt.Errorf("gslb health configs: %w", err)
	}
	defer rows.Close()

	configs := map[string]ClusterHealthConfig{}
	for rows.Next() {
		var clusterID string
		var cfg ClusterHealthConfig
		if err := rows.Scan(&clusterID, &cfg.Path, &cfg.IntervalSeconds,
			&cfg.TimeoutSeconds, &cfg.FailureThreshold); err != nil {
			return nil, err
		}
		existing, ok := configs[clusterID]
		if !ok {
			configs[clusterID] = cfg
			continue
		}
		if cfg.IntervalSeconds < existing.IntervalSeconds {
			existing.IntervalSeconds = cfg.IntervalSeconds
			existing.Path = cfg.Path
		} else if cfg.IntervalSeconds == existing.IntervalSeconds && cfg.TimeoutSeconds < existing.TimeoutSeconds {
			existing.Path = cfg.Path
		}
		if cfg.TimeoutSeconds < existing.TimeoutSeconds {
			existing.TimeoutSeconds = cfg.TimeoutSeconds
		}
		if cfg.FailureThreshold > existing.FailureThreshold {
			existing.FailureThreshold = cfg.FailureThreshold
		}
		configs[clusterID] = existing
	}
	return configs, rows.Err()
}
