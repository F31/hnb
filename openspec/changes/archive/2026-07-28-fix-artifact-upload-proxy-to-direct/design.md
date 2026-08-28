## Context

当前 `cmd/app-market` 的 `/api/v1/artifacts/upload` 端点使用 `r.ParseMultipartForm(100 << 20)` 将文件正文通过 Market API 代理到 Harbor。这违反了架构基线中"Market/Platform API SHALL NOT 代理大文件正文"的冻结约束（ART-003），且带来以下问题：

- Market API 成为数据面瓶颈，内存和带宽压力随文件大小线性增长
- 100MB 硬限制无法支持大模型权重、数据库备份等大型制品
- 无法利用 Harbor 原生 OCI Distribution Spec 的 blob 上传能力

## Goals / Non-Goals

**Goals:**
- 消除 Market API 的制品数据面代理，改为客户端直传 Harbor
- 保持上传安全：临时凭据、最小权限、租户隔离
- 支持任意大小文件（不再有 100MB 限制）
- 上传完成后的元数据记录与现有 Artifact 模型一致

**Non-Goals:**
- 不改变 Harbor 本身配置或部署
- 不涉及下载/删除流程（`Download`/`Delete` 方法可保留）
- 不涉及 Web Portal 上传 UI 改造（仅后端 API 变更，前端适配属于后续 change）
- 不涉及 CLI 上传流程改造（但 CLI 适配属于本 change 任务）

## Decisions

### Decision 1: Harbor Robot Account 作为临时凭据方案

选择 Harbor Robot Account（而非 OAuth2 Token 或直接 Basic Auth）的原因：

| 方案 | 优点 | 缺点 |
|---|---|---|
| **Robot Account** | 原生支持临时性（duration）、最小权限（per-project, push-only）、可审计 | 需 Harbor 2.0+（已满足），需额外 API 调用创建/销毁 |
| Harbor OAuth2 Token | 标准 OAuth2 流程 | Harbor OAuth2 主要用于身份认证，不直接支持 per-project push scope |
| 静态 Basic Auth | 实现简单 | 凭据固定，无法限制 scope 和 TTL，安全风险高 |

Harbor Robot Account API：

```
POST /api/v2.0/robots
{
  "name": "upload-{sessionID}",
  "duration": 3600,
  "level": "project",
  "permissions": [{
    "kind": "project",
    "namespace": "hnb",
    "access": [
      {"resource": "artifact", "action": "push"},
      {"resource": "artifact", "action": "pull"}
    ]
  }]
}
```

响应：
```json
{
  "id": 1,
  "name": "robot$upload-{sessionID}",
  "token": "harbor_robot_token_string"
}
```

### Decision 2: 两步上传流程

```
Client                     App Market                    Harbor
  │                            │                           │
  │  POST /artifacts/session   │                           │
  │ ─────────────────────────► │                           │
  │                            │  POST /api/v2.0/robots    │
  │                            │ ───────────────────────► │
  │                            │ ◄──────────────────────── │
  │ ◄──── {sessionID, URL,    │                           │
  │         robotName, token}  │                           │
  │                            │                           │
  │  PUT /v2/hnb/.../blobs/   │                           │
  │ ─────────────────────────────────────────────────────► │
  │ ◄───────────────────────────────────────────────────── │
  │                            │                           │
  │  POST /artifacts/confirm   │                           │
  │ ─────────────────────────► │                           │
  │                            │  DELETE /api/v2.0/robots  │
  │                            │ ───────────────────────► │
  │ ◄──── {artifactID,         │                           │
  │         digest, registry}  │                           │
```

Step 1: `POST /api/v1/artifacts/session` — 创建上传会话
- 请求：`{filename, artifact_type, size_bytes}`
- 处理：创建 Harbor robot account（push-only, 1h TTL），生成 UploadSession 记录
- 响应：`{session_id, harbor_url, robot_name, robot_token, expires_at}`

Step 2: 客户端直传 Harbor
- 客户端使用 robot 凭据，按照 OCI Distribution Spec 完成 blob + manifest 上传
- 上传 URL 格式：`{harbor_url}/v2/hnb/{repo}/blobs/uploads/`
- 客户端应使用 digest 锁定（`PUT ...?digest=sha256:...`）

Step 3: `POST /api/v1/artifacts/confirm` — 确认上传完成
- 请求：`{session_id, digest, size_bytes}`
- 处理：验证 digest 格式，创建 Artifact 记录，删除 robot account，更新 session 状态
- 响应：`{artifact_id, digest, registry_url}`

### Decision 3: UploadSession 数据模型

新增 `UploadSession` 模型，由 app-market 的 PostgreSQL 持久化：

```go
type UploadSessionStatus string
const (
    SessionPending   UploadSessionStatus = "pending"
    SessionUploading UploadSessionStatus = "uploading"
    SessionCompleted UploadSessionStatus = "completed"
    SessionExpired   UploadSessionStatus = "expired"
    SessionFailed    UploadSessionStatus = "failed"
)

type UploadSession struct {
    ID           string              `json:"id"`
    TenantID     string              `json:"tenant_id"`
    Filename     string              `json:"filename"`
    ArtifactType string              `json:"artifact_type"`
    SizeBytes    int64               `json:"size_bytes"`
    Status       UploadSessionStatus `json:"status"`
    HarborURL    string              `json:"harbor_url"`
    RobotID      int                 `json:"robot_id"`
    RobotName    string              `json:"robot_name,omitempty"`
    ArtifactID   *string             `json:"artifact_id,omitempty"`
    Digest       *string             `json:"digest,omitempty"`
    ExpiresAt    time.Time           `json:"expires_at"`
    CreatedAt    time.Time           `json:"created_at"`
    UpdatedAt    time.Time           `json:"updated_at"`
}
```

### Decision 4: 删除旧代理上传端点

`POST /api/v1/artifacts/upload` 端点及其相关代码将删除：
- `cmd/app-market/main.go` 中 `ParseMultipartForm` 相关 handler
- 删除 `Upload` 方法（保留 `Download` 和 `Delete` 用于现有功能）
- `types.go` 中的 `UploadResult` 类型可保留（用于 `confirm` 响应）

### Decision 5: 使用 pgx 替代 lib/pq

现有 app-market 使用 `lib/pq`，但 `lib/pq` 已进入维护模式。新增 UploadSession 存储时，改用 `pgx` 作为数据库驱动，与项目中其他组件保持一致。或者继续使用 `lib/pq` 以保持一致性。

选择：继续使用 `lib/pq`，避免引入新的数据库依赖。UploadSession 存储使用现有的 `sql.DB` 接口。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|---|---|
| Harbor Robot Account API 版本差异 | 目标 Harbor 2.0+，使用 `/api/v2.0/robots` 标准端点 |
| Robot Account 创建失败导致上传不可用 | 降级策略：返回错误信息，客户端可重试或使用备用 Harbor 实例 |
| 客户端直传 Harbor 暴露内部 Registry 地址 | 客户端应通过 Market API 获取 Harbor URL，生产环境可配置内部/外部地址映射 |
| 上传会话过期未清理 | 后台定时任务扫描过期 `pending` 状态的 session，删除对应 robot account |
| Robot Account 数量超限 | Harbor 有 robot account 配额限制，session 完成后立即删除；设置合理 TTL（1h） |
| 客户端上传到错误 URL 或篡改 digest | confirm 端点验证 digest 格式和大小，但不对 Harbor 做二次验证（信任 digest） |

## Migration Plan

1. 新增 `UploadSession` 模型和 `store.UploadSessionRepo`
2. 新增 `POST /api/v1/artifacts/session` 和 `POST /api/v1/artifacts/confirm` 端点
3. 新增 `pkg/appstore/storage/robot.go` 实现 Harbor Robot Account 客户端
4. 新增后台定时任务清理过期 session
5. 保留旧 `POST /api/v1/artifacts/upload` 端点作为过渡，标记为 deprecated
6. CLI 适配新流程
7. 旧端点移除（后续 change）

**回滚策略：** 保留旧上传端点，新端点出现问题时客户端可回退到旧端点。

## Open Questions

- Harbor Robot Account 的 `namespace` 字段：`hnb` 项目在 Harbor 中是否存在？是否需要自动创建？
- 大文件上传的客户端实现（CLI）是否在本 change 范围内？
- 是否需要支持断点续传（Harbor OCI blob upload 原生支持）？