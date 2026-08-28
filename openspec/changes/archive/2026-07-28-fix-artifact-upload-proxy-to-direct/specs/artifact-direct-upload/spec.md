## ADDED Requirements

### Requirement: [DIR-001] 上传会话创建
App Market SHALL 提供上传会话创建端点，接收文件名、制品类型和大小，签发 Harbor 临时 Robot Account 凭据，并返回 Harbor 直传 URL 和凭据。

**Traceability:** ART-03, HBR-01

#### Scenario: 创建上传会话成功
- **GIVEN** 客户端准备上传制品
- **WHEN** 客户端请求创建上传会话并指定文件名、制品类型和大小
- **THEN** 系统返回 Harbor 直传 URL、临时 Robot Account 凭据和过期时间
- **AND** 凭据为 push-only 权限，scope 限定于 `hnb` 项目
- **AND** 凭据 TTL 为 1 小时

#### Scenario: 创建上传会话时 Harbor 不可用
- **GIVEN** Harbor 服务不可用
- **WHEN** 客户端请求创建上传会话
- **THEN** 系统返回 503 错误
- **AND** 客户端可重试

### Requirement: [DIR-002] 制品直传 Harbor
客户端 SHALL 使用获取的临时凭据直接上传到 Harbor OCI Registry，不经过 Market API。

**Traceability:** ART-03

#### Scenario: 客户端直传 Harbor 成功
- **GIVEN** 客户端已获取上传会话凭据
- **WHEN** 客户端按照 OCI Distribution Spec 上传 blob 和 manifest
- **THEN** Harbor 返回制品 digest
- **AND** 上传过程不经过 Market API 代理

### Requirement: [DIR-003] 上传完成确认
客户端上传完成后 SHALL 调用确认端点，App Market SHALL 验证 digest 格式、记录 Artifact 元数据并清理临时凭据。

**Traceability:** ART-03

#### Scenario: 确认上传成功
- **GIVEN** 客户端已完成 Harbor 直传
- **WHEN** 客户端提交上传确认请求（含 session_id、digest、size_bytes）
- **THEN** 系统创建 Artifact 记录并返回 artifact_id、digest 和 registry_url
- **AND** 系统删除 Harbor Robot Account 凭据
- **AND** 会话状态标记为 completed

#### Scenario: 确认上传时 digest 格式无效
- **GIVEN** 客户端提交确认请求
- **WHEN** digest 不符合 sha256: 格式
- **THEN** 系统拒绝并返回 400 错误
- **AND** 会话状态标记为 failed

### Requirement: [DIR-004] 过期会话清理
App Market SHALL 定期清理过期未完成的 UploadSession，删除对应的 Harbor Robot Account。

**Traceability:** ART-03

#### Scenario: 过期会话自动清理
- **GIVEN** 一个 UploadSession 超过 TTL 仍未完成
- **WHEN** 后台清理任务执行
- **THEN** 系统删除对应的 Harbor Robot Account
- **AND** 会话状态标记为 expired