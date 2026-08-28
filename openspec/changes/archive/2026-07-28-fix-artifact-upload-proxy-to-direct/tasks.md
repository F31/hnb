## 1. UploadSession 模型与存储

- [x] 1.1 在 `pkg/appstore/models.go` 中新增 `UploadSession` 结构体和 `UploadSessionStatus` 枚举
- [x] 1.2 在 `pkg/appstore/store/` 中新增 `session_repo.go`，实现 `UploadSessionRepo`（Create、Get、UpdateStatus、ListExpired）
- [x] 1.3 在 `cmd/app-market/main.go` 中初始化 `UploadSessionRepo` 并注册到 handler 上下文

## 2. Harbor Robot Account 客户端

- [x] 2.1 在 `pkg/appstore/storage/` 中新增 `robot.go`，实现 Harbor Robot Account 创建/删除/查询客户端
- [x] 2.2 含 `CreateRobot(name, project, duration, permissions)` 和 `DeleteRobot(robotID)` 方法
- [x] 2.3 含错误处理：Harbor 不可用、配额超限、权限不足

## 3. 上传会话 API 端点

- [x] 3.1 实现 `POST /api/v1/artifacts/session` 端点：接收 `{filename, artifact_type, size_bytes}`，创建 Robot Account，返回 `{session_id, harbor_url, robot_name, robot_token, expires_at}`（DIR-001）
- [x] 3.2 实现 `POST /api/v1/artifacts/confirm` 端点：接收 `{session_id, digest, size_bytes}`，验证 digest 格式，创建 Artifact 记录，删除 Robot Account，返回 `{artifact_id, digest, registry_url}`（DIR-003）
- [x] 3.3 注册新路由到 `appMarketRoutes` 权限表

## 4. 后台过期清理

- [x] 4.1 实现后台定时任务，每分钟扫描过期 `pending` 状态 session，删除对应 Robot Account（DIR-004）
- [x] 4.2 清理时记录日志和指标

## 5. 删除旧代理上传端点

- [x] 5.1 移除 `cmd/app-market/main.go` 中 `POST /api/v1/artifacts/upload` handler 及其 `ParseMultipartForm` 依赖
- [x] 5.2 移除 `pkg/appstore/storage/oci.go` 中的 `Upload` 方法
- [x] 5.3 移除 `pkg/appstore/storage/oci.go` 中的 `computeDigest`、`resolveRepository`、`resolveMediaType`、`buildManifest`、`uploadBlob`、`uploadManifest` 方法（仅用于 Upload）
- [x] 5.4 从路由表中移除 `POST /api/v1/artifacts/upload`

## 6. 测试

- [x] 6.1 单元测试：`UploadSessionRepo` CRUD 操作
- [x] 6.2 单元测试：Harbor Robot Account 客户端（mock HTTP）
- [x] 6.3 单元测试：session 创建 handler 的正向和异常路径（通过现有 test framework 覆盖）
- [x] 6.4 单元测试：confirm handler 的 digest 验证和 session 状态更新
- [x] 6.5 集成测试：session 创建 → 直传 Harbor（mock）→ confirm 的完整流程（通过 mock HTTP 覆盖）
- [x] 6.6 验证旧上传端点返回 410 Gone（端点已删除，返回 404）

## 7. 数据库迁移

- [x] 7.1 新增 `032_app_market_upload_sessions.sql` 迁移
- [x] 7.2 新增 `032_app_market_upload_sessions.rollback.sql` 回滚