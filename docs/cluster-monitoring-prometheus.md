# 集群监控：Prometheus remote_write 接入

资源-集群的监控页使用“目标集群采集、中心指标后端查询”的模型：每个 Kubernetes 集群运行 Prometheus Agent 或 Prometheus Operator，采集 kubelet、node-exporter、kube-state-metrics 和可选的 DCGM Exporter；采集端通过 `remote_write` 写入中心的 Prometheus-compatible 存储（例如 Mimir、Thanos Receive 或 VictoriaMetrics）。HNB API Server 只代理预定义查询，不向浏览器暴露 Prometheus 地址、凭据或任意 PromQL。

## 必需标签

每条写入中心后端的样本必须带有稳定的集群标签。其值由 HNB 的集群注册记录提供，不能使用集群显示名称。

| 标签 | 值 |
| --- | --- |
| `hnb_cluster_id` | `runtime_targets.id`，即资源-集群 URL 中的集群 ID |

Prometheus Agent 配置示例（远端地址和认证信息应通过 Secret 注入）：

```yaml
global:
  external_labels:
    hnb_cluster_id: cluster-runtime-target-id

remote_write:
  - url: https://metrics.example.internal/api/v1/write
    # authorization / tls_config 由 Secret 生成，不写入 Git。
```

如由 Prometheus Operator 管理，同样将该标签设为 Prometheus CR 的 external labels；也可以在 remote-write 的 relabel 规则中强制覆盖。必须阻止租户在抓取目标或重写规则中伪造该标签。

共享集群不能由单个 Agent 用 `external_labels` 固定 `hnb_tenant_id`：那会把整个集群的节点、系统与其他租户工作负载错误标给一个租户。租户指标必须从 `tenant_cluster_allocations` 与命名空间绑定映射派生（例如把 `namespace` 与 HNB 受管命名空间标签关联），并只向租户暴露其命名空间工作负载用量；节点总容量、节点告警等集群级指标须有单独的平台运维权限。

## 平台配置与访问路径

为 API Server 设置中心查询端点：

```text
HNB_PROMETHEUS_URL=https://metrics-query.example.internal
```

该地址必须是 API Server 可访问的 Prometheus HTTP API，且仅能经受控网络路径访问。当前实现使用：

- `GET /api/v1/resources/clusters/{id}/monitoring/summary`
- `GET /api/v1/resources/clusters/{id}/monitoring/metrics?start=<RFC3339>&end=<RFC3339>`

两条 API 都要求集群 `read` 权限。服务端先以当前租户验证 `runtime_targets` 归属，再把两个标签写入预定义 PromQL；请求方不能传入查询语句或标签筛选条件。未设置或不可用 `HNB_PROMETHEUS_URL` 时返回 `503`，前端应保持监控功能开关关闭。

前端开关：`VITE_FEATURE_RESOURCE_CLUSTER_MONITORING=true`。仅用于本地演示的 `VITE_CLUSTER_DETAIL_USE_FIXTURES=true` 会绕过 BFF，不能用于生产。

## 指标覆盖范围

核心页依赖 `machine_cpu_cores`、`node_cpu_seconds_total`、`node_memory_MemTotal_bytes`、`node_memory_MemAvailable_bytes`、`kube_node_status_allocatable`、`kube_node_spec_unschedulable`、`kube_namespace_labels` 和 `ALERTS`。GPU 卡片在存在 DCGM Exporter 时读取 `DCGM_FI_DEV_GPU_UTIL` 与 `DCGM_FI_DEV_FB_*`；没有 GPU 指标时可为空序列。

Agent 心跳、连接状态、能力快照和操作事件仍由 HNB cluster-agent 上报到 RuntimeTarget Read Model，不与 Prometheus 时序数据混用。Kubernetes Metrics API 仅适合短时 CPU/内存兜底，不承担趋势、告警与跨集群容量分析。
