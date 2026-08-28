## MODIFIED Requirements

### Requirement: [ART-001] 统一 OCI 逻辑入口
镜像、Helm Chart、JAR/WAR、Operator、配置、模型、Prompt、Guardrail、评测、SBOM 和离线包 SHALL 使用租户隔离的统一 ArtifactDescriptor 管理，并 SHALL 通过稳定的逻辑 Endpoint 按 digest 解析数据面位置；上层 SHALL NOT 依赖底层 Bucket 路径。

**Traceability:** ART-08, ART-STO-01, ART-STO-02

#### Scenario: 访问多类型制品
- **GIVEN** 一个 Release 同时包含镜像、Chart 和 SBOM
- **WHEN** Agent 获取该 Release 的 ArtifactDescriptor
- **THEN** 每个描述符返回类型、media type、SHA-256 digest 和统一逻辑引用
- **AND** Agent 从描述符指定的数据面直接获取内容
- **AND** 上层不依赖底层 Bucket 路径

#### Scenario: 跨租户读取描述符
- **GIVEN** 制品属于租户 A
- **WHEN** 租户 B 请求该 ArtifactDescriptor
- **THEN** 系统拒绝访问且不泄露描述符元数据

### Requirement: [ART-002] 摘要锁定与内容寻址
所有 Artifact SHALL 具有经权威后端验证的、小写十六进制 SHA-256 摘要；发布的 Release 和生产 ExecutionPlan SHALL 固定 digest，标签仅用于检索和展示，且 SHALL NOT 参与生产执行解析。

**Traceability:** ART-02, ART-STO-04

#### Scenario: 标签被重新指向
- **GIVEN** 一个生产实例按 digest 部署
- **WHEN** 同名标签后来指向新镜像
- **THEN** 运行实例和回滚点仍引用原 digest
- **AND** 审计能够证明实际部署内容

#### Scenario: 发布包含未验证制品的 Release
- **GIVEN** Release 引用了未验证、缺失或仅有 tag 的制品
- **WHEN** 发布者尝试发布 Release 或生成生产 ExecutionPlan
- **THEN** 系统拒绝请求并列出无效引用

#### Scenario: 生成生产 ExecutionPlan
- **GIVEN** Release 的全部 ArtifactDescriptor 已验证
- **WHEN** 平台生成生产 ExecutionPlan
- **THEN** 计划包含排序稳定的 SHA-256 digest 集合
- **AND** digest 集合参与计划摘要计算

### Requirement: [ART-003] 大文件直传直取
Market/Platform API SHALL NOT 代理大文件正文；上传者、Agent、Helm、容器运行时和模型加载器 SHALL 直接访问 Registry/S3 数据面。App Market SHALL 使用最小权限短期凭据实现直传，并 SHALL 在确认时向权威后端验证 digest 后原子记录 ArtifactDescriptor。

**Traceability:** ART-01, ART-STO-03, DIR-001, DIR-002, DIR-003

#### Scenario: 上传大型模型
- **GIVEN** 发布者准备上传大模型权重
- **WHEN** 发布者请求创建上传会话
- **THEN** App Market 签发短期 push-only 凭据
- **AND** 发布者直接上传到数据面
- **AND** 数据不经过 market-api 或 platform-api

#### Scenario: 确认已上传制品
- **GIVEN** 客户端提交 session、digest 和大小
- **WHEN** 权威后端存在相同 digest 的对象
- **THEN** 系统在一个事务中创建 ArtifactDescriptor 并完成 session
- **AND** 系统清理临时凭据

#### Scenario: 客户端伪造或误报 digest
- **GIVEN** 客户端提交格式无效或权威后端不存在的 digest
- **WHEN** 系统处理确认请求
- **THEN** 系统拒绝确认且不创建 ArtifactDescriptor

#### Scenario: 代理上传被拒绝
- **GIVEN** 客户端尝试通过 Market API 代理上传文件
- **WHEN** 客户端发送文件正文到 `/api/v1/artifacts/upload`
- **THEN** 端点返回 410 Gone 或 404 Not Found
- **AND** 响应提示使用上传会话流程

### Requirement: [ART-004] 存储后端与档位可替换
ArtifactStorageProfile SHALL 支持 Local、PVC、S3 和 OCI 后端，并 SHALL 标识权威角色、服务档位、SecretReference、RPO 与 RTO；Minimal SHALL 不强制对象存储，Lite HA 及以上 SHALL 使用共享权威后端。迁移 SHALL 通过可恢复 Operation 执行且不得改变 ArtifactDescriptor digest 或 Release 引用。

**Traceability:** ART-STO-05, ART-STO-06, ART-STO-07, ART-STO-22, ART-STO-23

#### Scenario: 创建 Minimal 存储档位
- **GIVEN** 环境档位为 Minimal
- **WHEN** 运维创建 Local、PVC 或 OCI profile
- **THEN** 系统接受满足该档位约束的 profile
- **AND** 凭据仅以 SecretReference 保存

#### Scenario: 创建 Lite HA 存储档位
- **GIVEN** 环境档位为 Lite HA 或更高
- **WHEN** 运维选择仅节点本地的权威后端
- **THEN** 系统拒绝配置并要求共享权威后端及明确 RPO/RTO

#### Scenario: 从 Minimal 升级到 Lite HA
- **GIVEN** 已有 Release 使用本地后端
- **WHEN** 运维迁移到共享 S3、PVC 或 OCI 后端
- **THEN** 迁移通过 Operation 执行并可从 checkpoint 恢复
- **AND** Release 和 digest 引用保持不变

### Requirement: [ART-005] 三级分发与缓存可重建
平台 SHALL 以权威 profile 为中心管理区域镜像和边缘缓存分发目标；Mirror/Cache SHALL 非权威、可按水位清理并可通过幂等 Operation 从权威后端重建。控制面 SHALL 记录 desired digest、observed digest、健康和重建状态，但 SHALL NOT 代理制品正文。

**Traceability:** ART-STO-13, ART-STO-14

#### Scenario: 边缘缓存丢失
- **GIVEN** 某站点缓存数据盘损坏但权威副本健康
- **WHEN** 站点恢复网络并请求重建
- **THEN** 系统创建幂等重建 Operation
- **AND** 缓存从权威后端恢复并校验 digest
- **AND** 权威 Release 状态不受影响

#### Scenario: 缓存达到高水位
- **GIVEN** 非权威缓存超过配置的高水位
- **WHEN** 清理任务选择候选副本
- **THEN** 系统仅清理可从权威后端重建且无本地保护锁的副本
- **AND** 不删除权威 ArtifactDescriptor

#### Scenario: 权威后端不可用
- **GIVEN** 权威 profile 不健康
- **WHEN** 分发目标请求同步或重建
- **THEN** Operation 进入可重试失败状态并保留 checkpoint
- **AND** 系统不把镜像或缓存提升为权威事实源

### Requirement: [ART-006] 安全 GC 与引用保护
制品删除 SHALL 经过租户隔离的引用分析、preview、Tombstone、保留期、锁和 Operation；运行实例、Release、回滚点、组合、灾备和离线 Bundle 引用的制品 SHALL NOT 被回收。执行删除前 SHALL 再次检查引用，并 SHALL 记录可审计结果。

**Traceability:** ART-STO-16, ART-STO-17, ART-STO-18

#### Scenario: 回收仍被回滚点引用的镜像
- **GIVEN** 一个旧 digest 仍是有效回滚点
- **WHEN** 运维执行 GC preview 或 execute
- **THEN** 系统列出引用并阻止删除

#### Scenario: 回收无引用制品
- **GIVEN** 一个制品没有保护引用且超过保留要求
- **WHEN** 运维确认 GC
- **THEN** 系统锁定制品、创建 Tombstone 并提交 GC Operation
- **AND** worker 在保留期后重新检查引用再执行幂等删除
- **AND** 系统记录 actor、artifact、digest、operation 和结果

#### Scenario: Tombstone 后出现新引用
- **GIVEN** 制品已 Tombstone 但尚未 sweep
- **WHEN** 新的有效保护引用被创建
- **THEN** 系统取消待删除状态并保留制品

#### Scenario: GC worker 中断
- **GIVEN** GC Operation 在删除过程中中断
- **WHEN** worker 从 checkpoint 重试
- **THEN** 删除操作保持幂等且不会绕过最终引用检查
- **AND** Operation 支持暂停、重试、限速和审计
