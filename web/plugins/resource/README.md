# @hnb/plugin-resource

资源插件（T1）：容器集群、节点、GPU、网络、存储、GSLB。

## cluster-management 模块

集群管理功能模块位于 `src/pages/cluster-management/`，提供集群列表、详情、注册/创建向导与升级/解除纳管动作。

### 路由

| 路径 | 组件 | 说明 |
|---|---|---|
| `/resource/clusters` | `ClusterList.vue` | 集群列表（服务端分页、状态字典、写动作） |
| `/resource/clusters/:clusterId` | `ClusterDetail.vue` | 集群详情（概览 + 节点 + 配置） |

### 数据来源

只读 Read Model（CQRS，白皮书 §3.5）：

- `GET /api/v1/resources/clusters`：列表（分页 + `keyword/kind/status` 过滤）
- `GET /api/v1/resources/clusters/{id}`：详情
- `GET /api/v1/resources/clusters/{id}/nodes`：节点只读列表
- `GET /api/v1/dictionaries/cluster.status`：状态字典

写动作统一经 RuntimeIntent 提交（白皮书 §3.2 Operation 唯一写入口）：

- `POST /api/v1/runtime-intents`，kind 支持 `CreateKubernetesTarget` / `ImportRuntimeTarget` / `UpgradeRuntimeTarget` / `DeleteRuntimeTarget`，携带 `Idempotency-Key` 与 `X-Correlation-ID`，响应 `RuntimeIntentRecord`，进度通过 Operation Center 跟踪。

所有接口调用经 `@hnb/api-client`，无直接 `fetch`；服务端始终独立校验权限与租户隔离。

### 灰度开关

环境变量 `VITE_FEATURE_RESOURCE_CLUSTER_MGMT`（默认 `true`）控制真实实现是否启用。设为 `false` 时插件降级为占位页面（保留路由可达），详见 `docs/web-resource-cluster-management.md` §10.1。

`VITE_FEATURE_RESOURCE_CLUSTER_MONITORING` 与
`VITE_FEATURE_RESOURCE_CLUSTER_ADVANCED` 默认均为 `false`。监控页已通过平台 BFF
查询中心 Prometheus-compatible 后端，但仅应在 API Server 已配置
`HNB_PROMETHEUS_URL` 且目标集群已写入 `hnb_tenant_id`、`hnb_cluster_id` 标签后开启；
接入细节见 `docs/cluster-monitoring-prometheus.md`。节点组、租户分配和插件管理仍保持
深链占位，不会在页签、侧栏或主菜单中展示。

### 安全约束

- kubeconfig / CloudCore 凭据仅通过 `SecretReference` 引用，前端不接收/回显/持久化明文。
- 菜单/按钮隐藏不构成安全边界，服务端对每个 API 独立校验权限与租户。
- KubeConfig 下载与 cluster-agent 接入会签发或展示凭据，需 `cluster:execute`，
  不继承普通 `cluster:read` 权限。
- STALE 集群写操作按钮置灰，服务端仍按 RT-005 执行风险确认。

### 测试

- 类型检查：`pnpm --filter @hnb/plugin-resource typecheck`
- 构建：`pnpm --filter @hnb/plugin-resource build`
- 端到端：`pnpm --filter hnb-web-console test:e2e -- cluster-management.spec.ts`（mock 全部 API，覆盖列表/详情/二次确认/STALE/权限收回场景）
