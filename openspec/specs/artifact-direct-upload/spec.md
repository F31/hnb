# artifact-direct-upload

## Purpose
定义 App Market 的统一制品上传流程。默认路径使用 Artifact Transfer Gateway 承担数据面分片、断点续传、完整性校验和 staging；Harbor/OCI 直传仅作为 legacy/实验兼容路径。Market API 只管理控制面会话、权限、元数据与审计，不代理制品正文。

## Requirements

### Requirement: [DIR-001] 上传会话创建
App Market SHALL 提供上传会话创建端点，接收文件名、制品类型和大小，并返回统一 transfer endpoint、分片策略、断点续传策略和过期时间。若启用 legacy Harbor direct 模式，响应 MAY 附带短期 push-only Robot Account 凭据。

**Traceability:** ART-03, HBR-01

#### Scenario: 创建上传会话成功
- **GIVEN** 客户端准备上传制品
- **WHEN** 客户端请求创建上传会话并指定文件名、制品类型和大小
- **THEN** 系统返回 transfer endpoint、chunk_size、max_concurrency、resume_storage_key 和过期时间
- **AND** 默认策略为 Artifact Transfer Gateway multipart 上传
- **AND** 凭据 TTL 为 1 小时

#### Scenario: 创建上传会话时 Harbor 不可用
- **GIVEN** Harbor 服务不可用
- **WHEN** 客户端请求创建上传会话
- **THEN** 系统仍可返回 Transfer Gateway 会话
- **AND** legacy direct Harbor 凭据 MAY 为空

### Requirement: [DIR-002] 统一 Transfer Gateway 上传
客户端 SHALL 使用 UploadSession 返回的统一 transfer endpoint 上传分片，不需要理解 Harbor、S3、PVC 或 Local 后端差异。

**Traceability:** ART-03

#### Scenario: 客户端通过 Transfer Gateway 上传成功
- **GIVEN** 客户端已获取上传会话
- **WHEN** 客户端按 chunk_size 将文件分片上传到 transfer endpoint
- **THEN** Transfer Gateway 持久化分片并记录断点状态
- **AND** 完成时计算 sha256 digest、登记 Artifact 元数据
- **AND** 上传协议对后端存储类型保持透明

### Requirement: [DIR-003] 上传完成确认
客户端上传完成后 SHALL 调用 transfer complete 端点；Transfer Gateway SHALL 合并分片、计算 digest、记录 Artifact 元数据并清理或标记 staging 状态。legacy Harbor direct 模式 MAY 继续支持 confirm 端点。

**Traceability:** ART-03

#### Scenario: 确认上传成功
- **GIVEN** 客户端已完成全部分片上传
- **WHEN** 客户端提交 complete 请求
- **THEN** 系统创建 Artifact 记录并返回 artifact_id、digest 和 registry_url
- **AND** 会话状态标记为 completed

#### Scenario: 完成上传时分片不完整
- **GIVEN** 客户端提交 complete 请求
- **WHEN** Transfer Gateway 发现分片缺失或大小不匹配
- **THEN** 系统拒绝并返回 409 错误
- **AND** 会话状态标记为 failed

### Requirement: [DIR-004] 过期会话清理
App Market SHALL 定期清理过期未完成的 UploadSession，清理对应 transfer staging 数据；若存在 legacy Harbor Robot Account，也必须删除。

**Traceability:** ART-03

#### Scenario: 过期会话自动清理
- **GIVEN** 一个 UploadSession 超过 TTL 仍未完成
- **WHEN** 后台清理任务执行
- **THEN** 系统删除对应 staging 数据
- **AND** 若存在 Harbor Robot Account，系统删除该凭据
- **AND** 会话状态标记为 expired

### Requirement: [DIR-005] 大文件并发上传与断点续传
客户端上传大文件制品时 SHOULD 支持并发分片上传和断点续传；该能力 MUST 基于统一 Transfer Gateway 协议实现，后端可映射到 shared filesystem/PVC、S3 Multipart 或其他 staging backend。Market API 不应代理制品正文。

**Traceability:** ART-03, HBR-01

#### Scenario: 客户端使用统一会话断点续传
- **GIVEN** 客户端已获取 UploadSession 返回的 transfer endpoint
- **AND** 客户端开始上传分片
- **WHEN** 网络中断或页面刷新导致上传暂停
- **THEN** 客户端可依据本地保存的 endpoint、已完成 part 列表、文件指纹和 session_id 查询或恢复 upload session
- **AND** 客户端只上传未完成分片

#### Scenario: 客户端并发上传受控
- **GIVEN** 客户端准备上传大文件制品
- **WHEN** 文件大小超过客户端配置的分片阈值
- **THEN** 客户端按固定 chunk size 切分并以受控并发数上传
- **AND** 默认并发数 MUST 有上限，避免耗尽浏览器连接池、Transfer Gateway、后端 staging 或租户出口带宽
- **AND** 每个分片失败后 SHOULD 使用指数退避进行有限重试

#### Scenario: UploadSession 返回续传策略
- **GIVEN** 客户端请求创建上传会话
- **WHEN** App Market 创建 UploadSession 成功
- **THEN** 响应 SHOULD 返回 transfer policy，包括 endpoint、strategy、chunk_size、max_concurrency、resumable、expires_at 和 resume_storage_key
- **AND** 客户端 MUST 使用 resume_storage_key 在本地保存断点元数据
- **AND** 客户端不得在本地保存长期有效密钥

#### Scenario: Token 过期后的恢复
- **GIVEN** 客户端存在未完成上传的断点元数据
- **AND** 原 UploadSession 已过期
- **WHEN** 用户尝试继续上传
- **THEN** 客户端 MUST 请求新的 UploadSession
- **AND** 若 transfer session 已不可恢复，客户端 MUST 从第一个未确认 part 重新上传
- **AND** 旧 UploadSession 由后台清理任务删除 staging 数据和可选 Robot Account

#### Scenario: 完成确认前校验完整性
- **GIVEN** 客户端完成全部分片上传
- **WHEN** 客户端调用 complete 端点
- **THEN** Transfer Gateway SHALL 在服务端计算 digest
- **AND** App Market SHALL 校验 session 归属并记录服务端计算出的 digest

### Requirement: [DIR-006] 单节点与分布式高可用部署
Transfer Gateway backend MUST 明确声明部署适用性。local backend 仅适用于单节点或开发环境；多副本/HA 部署 MUST 使用 shared filesystem/PVC、S3 Multipart 或等价共享 staging backend。

**Traceability:** ART-03, HBR-01

#### Scenario: 单节点 local backend
- **GIVEN** App Market 以单副本运行
- **WHEN** Transfer Gateway 使用 local backend
- **THEN** 分片、状态查询和 complete 请求均可访问同一 staging 目录

#### Scenario: 多副本 shared filesystem backend
- **GIVEN** App Market 或 Transfer Gateway 以多副本运行
- **WHEN** 客户端上传分片并被负载均衡到不同副本
- **THEN** 所有副本 MUST 访问同一 staging backend
- **AND** complete 请求 MUST 能读取任意副本写入的分片

#### Scenario: 多副本禁止非共享 local backend
- **GIVEN** 部署配置声明 replica_count 大于 1
- **WHEN** Transfer Gateway backend 为非共享 local
- **THEN** 系统 MUST 在部署校验或启动检查中拒绝该配置或给出阻断级告警
