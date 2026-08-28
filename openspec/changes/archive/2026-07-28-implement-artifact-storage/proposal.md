## Why

当前 Artifact Storage 仅具备零散的 OCI 元数据、Harbor 客户端和上传会话，尚不能保证统一描述符、真实摘要验证、可替换存储档位、可重建分发状态与引用安全回收。P1 需要补齐这些控制面能力，并修复上传确认未创建 Artifact 记录而必然触发外键失败的问题，使 Release 和 ExecutionPlan 能可靠固定可审计的制品 digest。

## What Changes

- **Change ID:** `implement-artifact-storage`；能力分级为 **T1 默认交付**；影响 App Market、Artifact Storage、Operation 和 Platform ExecutionPlan 路径。
- 引入统一 `ArtifactDescriptor` 控制面模型和 API，覆盖 OCI 镜像、Chart、JAR/WAR、Operator、配置、模型、Prompt、Guardrail、评测、SBOM 与离线包。
- 修复上传确认事务：验证 Harbor 中的 manifest digest 后原子创建 Artifact 与完成 UploadSession；拒绝无效 SHA-256 和不存在的对象。
- 让 Release/ExecutionPlan 使用经验证的 digest 引用，禁止生产计划依赖可变 tag。
- 引入 `ArtifactStorageProfile`，支持 Local/PVC/S3/OCI 配置、权威后端标识、档位约束及 RPO/RTO 元数据；不在本 change 内实现新的对象存储服务。
- 引入中心、区域、边缘分发目标的控制面状态、健康与重建请求；实际复制继续委托 Harbor/现有 Provider，不自建数据面。
- 引入 Artifact 引用、Tombstone、保留期、锁和 GC preview/execute 流程；执行删除必须通过现有 Operation 链并保留审计事实。
- 提供 PostgreSQL 迁移、API/仓储测试、Harbor mock 集成测试和旧数据回填策略。
- **非目标：** 不部署新的 Registry/S3/缓存中间件，不实现 Harbor 本身的复制引擎，不代理制品正文，不改变 Release -> ExecutionPlan -> Operation 的唯一写入路径。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `artifact-storage`: 明确统一描述符、Harbor 摘要确认、存储档位、分发控制面和安全 GC 的可验证 API 与生命周期行为。

## Impact

- **依赖 change：** `2026-07-28-fix-artifact-upload-proxy-to-direct`；复用 Harbor、PostgreSQL、Operation Store、Transactional Outbox 和 NATS JetStream，不新增中间件。
- **代码与 API：** 影响 `pkg/appstore/`、`cmd/app-market/`、`cmd/platform-api/`、PostgreSQL migrations 和相关契约；新增描述符、profile、distribution、reference 与 GC API。
- **迁移与兼容性：** 新表和约束采用先扩展后回填；现有 `artifacts` 数据转换为 descriptor，无法验证的记录保持不可发布状态。新增 API 向后兼容；已删除的正文代理上传端点不恢复。
- **回滚策略：** 先停止新写路径和 GC worker，再回滚应用；新增表保留以避免丢失引用与审计事实，数据库 destructive rollback 仅用于未承载生产数据的环境。
- **安全风险：** 临时凭据必须最小权限和短 TTL；确认端不得信任客户端 digest/size；GC 必须执行租户隔离、引用分析、锁与审批，不允许直接调用 OCI 删除绕过 Operation。
- **资源预算：** 控制面只保存元数据；后台扫描按批次和速率限制运行，常态数据库查询与 Harbor 请求保持 O(batch)，不引入大文件内存或磁盘缓冲。
- **可观测要求：** 记录 session、digest 验证、profile、distribution rebuild、GC preview/execute 的结构化日志、计数、延迟和失败原因，并关联 tenant、artifact 与 operation ID。
- **退出判据：** 数据库迁移可前滚；上传确认端到端通过；生产计划包含有效 SHA-256；profile 档位校验通过；分发缓存可标记并重建；被 Release/回滚点/组合/灾备/Bundle 引用的制品无法回收；构建、测试和 vet 全部通过。
