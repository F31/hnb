# artifact-storage

## Purpose
定义 HNB Artifact Storage 的统一 OCI 逻辑模型、内容寻址、存储档位、数据面传输、分发缓存和安全回收生命周期。

## Requirements

### Requirement: [ART-001] 统一 OCI 逻辑入口
镜像、Helm Chart、JAR/WAR、Operator、配置、模型、Prompt、Guardrail、评测、SBOM 和离线包 SHALL 使用统一 ArtifactDescriptor 管理，并 SHOULD 优先通过统一 OCI Registry Endpoint 分发。

**Traceability:** ART-08, ART-STO-01, ART-STO-02

#### Scenario: 访问多类型制品
- **GIVEN** 一个 Release 同时包含镜像、Chart 和 SBOM
- **WHEN** Agent 获取短期凭据后拉取制品
- **THEN** 全部制品从同一逻辑 Endpoint 获取
- **AND** 上层不依赖底层 Bucket 路径

### Requirement: [ART-002] 摘要锁定与内容寻址
所有 Artifact SHALL 具有 SHA-256 摘要；生产 ExecutionPlan SHALL 固定 digest，标签仅用于检索和展示。

**Traceability:** ART-02, ART-STO-04

#### Scenario: 标签被重新指向
- **GIVEN** 一个生产实例按 digest 部署
- **WHEN** 同名标签后来指向新镜像
- **THEN** 运行实例和回滚点仍引用原 digest
- **AND** 审计能够证明实际部署内容

### Requirement: [ART-003] 大文件直传直取
Market/Platform API SHALL NOT 代理大文件正文；上传者、Agent、Helm、容器运行时和模型加载器 SHALL 直接访问 Registry/S3 数据面。

**Traceability:** ART-01, ART-STO-03

#### Scenario: 上传大型模型
- **GIVEN** 发布者上传大模型权重
- **WHEN** 平台签发短期上传凭据
- **THEN** 数据不经过 market-api 或 platform-api
- **AND** 控制面仅记录元数据和状态

### Requirement: [ART-004] 存储后端与档位可替换
ArtifactStorageProfile SHALL 支持 Local/PVC/S3 等后端；Minimal SHALL 不强制对象存储，Lite HA 及以上 SHALL 使用共享权威后端并明确 RPO/RTO。

**Traceability:** ART-STO-05, ART-STO-06, ART-STO-07, ART-STO-22, ART-STO-23

#### Scenario: 从 Minimal 升级到 Lite HA
- **GIVEN** 已有 Release 使用本地后端
- **WHEN** 运维迁移到共享 S3 后端
- **THEN** Release 和 digest 引用保持不变
- **AND** 迁移通过 Operation 执行并可恢复

### Requirement: [ART-005] 三级分发与缓存可重建
平台 SHOULD 支持中心权威仓库、区域镜像和边缘缓存三级分发；Mirror/Cache SHALL 非权威、可按水位清理并可从中心重建。

**Traceability:** ART-STO-13, ART-STO-14

#### Scenario: 边缘缓存丢失
- **GIVEN** 某站点 Registry Mirror 数据盘损坏
- **WHEN** 站点恢复网络连接
- **THEN** 缓存可以从权威仓库重新构建
- **AND** 权威 Release 状态不受影响

### Requirement: [ART-006] 安全 GC 与引用保护
制品删除 SHALL 经过引用分析、Tombstone、保留期、锁和 Operation；运行实例、回滚点、组合、灾备和离线 Bundle 引用的制品 SHALL NOT 被回收。

**Traceability:** ART-STO-16, ART-STO-17, ART-STO-18

#### Scenario: 回收仍被回滚点引用的镜像
- **GIVEN** 一个旧 digest 仍是有效回滚点
- **WHEN** 运维执行 GC 预览
- **THEN** 系统列出引用并阻止删除
- **AND** GC 支持暂停、重试、限速和审计
