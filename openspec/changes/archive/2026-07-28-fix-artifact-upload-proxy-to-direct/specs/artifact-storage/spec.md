## MODIFIED Requirements

### Requirement: [ART-003] 大文件直传直取
Market/Platform API SHALL NOT 代理大文件正文；上传者、Agent、Helm、容器运行时和模型加载器 SHALL 直接访问 Registry/S3 数据面。App Market SHALL 通过签发临时 Harbor Robot Account 凭据实现直传，不代理文件正文。

**Traceability:** ART-01, ART-STO-03, DIR-001, DIR-002, DIR-003

#### Scenario: 上传大型模型
- **GIVEN** 发布者准备上传大模型权重
- **WHEN** 发布者请求创建上传会话
- **THEN** App Market 签发 Harbor 临时 Robot Account 凭据
- **AND** 发布者使用临时凭据直接上传到 Harbor
- **AND** 数据不经过 market-api 或 platform-api
- **AND** 控制面仅记录元数据和状态

#### Scenario: 代理上传被拒绝
- **GIVEN** 客户端尝试通过 Market API 代理上传文件
- **WHEN** 客户端发送文件正文到 /api/v1/artifacts/upload
- **THEN** 端点返回 410 Gone 或 404 Not Found
- **AND** 响应提示使用上传会话流程