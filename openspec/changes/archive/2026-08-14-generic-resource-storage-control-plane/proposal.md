## Why

当前资源侧存储仅为占位页，容器侧则直接通过高权限 Kubernetes proxy 管理 StorageClass、PV 和 PVC；这种实现既无法统一异构存储供给，也无法提供细粒度授权、审计和安全回收。Change ID `generic-resource-storage-control-plane` 将建立 T1 工作负载存储控制平面，使平台管理员能够统一发现和管理驱动安装、存储系统、服务规格及其集群映射，同时保持应用管理员只消费 Kubernetes 存储抽象。

## What Changes

- 新增 `StorageDriverPackage`、`StorageDriverInstallation`、`StorageBackend`、`WorkloadStorageOffering` 和 `StorageClassBinding` 等边界清晰的领域模型。
- 将“资源 → 存储”建设为供给侧入口，分为存储总览、存储系统、存储服务、驱动与连接器、告警规则；Phase 1 仅提供真实发现结果的只读清单。
- 扩展 RuntimeTarget 观察数据，结构化发现 StorageClass、CSIDriver、CSINode、CSIStorageCapacity、VolumeAttachment 和可选快照 API，并携带 freshness 与来源。
- 保留“容器 → 存储”作为消费侧 StorageClass/PVC/PV/Snapshot 视图，通过 `StorageClassBinding` 与资源侧服务规格关联。
- 后续写操作统一使用专用存储 API、细粒度权限和 `ExecutionPlan → Operation`；安装、升级、扩容、回收等危险操作不得继续依赖任意路径 proxy。
- **BREAKING**：废弃通过删除 PV `spec.claimRef` 实现的通用“回收”；后续仅允许 Provider 专属、可审计的数据清理或手动释放工作流。
- `ArtifactStorageProfile` 继续归 App Market 所有，不并入工作负载存储模型；S3/MinIO 桶服务不伪装为普通 CSI/PV 能力。

## Capabilities

### New Capabilities
- `workload-storage-control-plane`: 工作负载存储驱动、后端系统、服务规格、StorageClass 映射、容量健康和受治理操作。

### Modified Capabilities
- `runtime-target`: 增加结构化存储资源与 CSI 健康发现要求。
- `portal-experience`: 明确资源侧供给视图、容器侧消费视图及兼容路由行为。
- `alert-notification`: 增加基于稳定资源引用和 Provider 指标适配器的存储告警目标。

## Impact

### Classification And Planes

- 分级：T1 默认交付；Provider 专属池、磁盘健康和对象存储连接器为 T2。
- 影响平面：Platform Core、Operation、Runtime Target/Agent、Provider/Extension、Portal、Observability；不改变 App Market 的制品存储所有权。
- 依赖 change：无活动 change；依赖现有 `runtime-target`、`provider-conformance`、`alert-notification` 和 `composition-operation` 能力。

### Code And Contracts

- 新增版本化存储 OpenAPI/schema、PostgreSQL read model、apiserver 专用路由、cluster-agent 发现项和资源插件页面。
- 调整 `web/plugins/resource`、`web/plugins/container`、DB 导航注册和权限；容器旧路由在迁移期保持兼容。
- 不引入新数据库或中间件，复用 PostgreSQL Operation/Read Model、Transactional Outbox、NATS JetStream 和现有 SecretReference。

### User Value And Non-Goals

- 用户价值：跨 Ceph、NFS、云盘、本地盘和其他 CSI 后端获得一致的清单、服务目录、映射、容量与告警入口。
- 非目标：Phase 1 不安装驱动、不创建 StorageClass、不回收数据、不实现对象桶服务，也不承诺 CSI 能提供后端总容量或性能指标。

### Compatibility, Security, And Migration

- 兼容性：保留 `/container/instances/storage`，新资源页先只读；稳定后通过重定向与查询参数保留上下文。
- 安全：专用 IAM、租户/工作空间/集群/命名空间校验、agent 路径 allowlist、观察与执行 ServiceAccount 分离、SecretReference、Operation 审批与审计。
- 迁移：先导入现有 StorageClass 与 CSI 观察事实，再创建 Offering/Binding；禁止把现有 fixture 当作迁移事实。
- 回滚：禁用新导航和专用 API，保留 read model 数据并恢复旧路由；不回滚或删除集群内 Kubernetes 资源。

### Resources, Observability, And Exit Criteria

- 资源预算：Phase 1 每集群发现请求有界并分页，观察数据保留 TTL；禁止以 PVC/PV/volumeHandle 作为无界 Prometheus label。
- 可观测：记录发现延迟、freshness、失败原因、驱动健康、API/Operation 成功率及审计 correlation ID。
- 退出判据：多种后端的只读发现可用；专用权限与 proxy allowlist 生效；Offering 到多集群 StorageClass 映射可追踪；危险写操作全部通过 Operation；旧 PV 回收入口移除；路由迁移具备回滚验证。
