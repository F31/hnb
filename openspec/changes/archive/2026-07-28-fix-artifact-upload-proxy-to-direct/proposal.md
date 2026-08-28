## Why

当前 `cmd/app-market` 的制品上传接口 (`POST /api/v1/artifacts/upload`) 使用 `ParseMultipartForm(100MB)` 将文件正文通过 Market API 代理到 Harbor，违反架构基线中"Market/Platform API SHALL NOT 代理大文件正文"的冻结约束（ART-003）。代理上传导致 Market API 成为数据面瓶颈，无法支持大模型权重等大型制品的可靠上传，且增加 Market API 的内存和带宽压力。

## What Changes

- 将 `POST /api/v1/artifacts/upload` 从"代理上传"改为"签发 Harbor 临时上传凭据"
- 客户端（CLI/Portal）凭凭据直接上传到 Harbor OCI Registry
- 上传完成后客户端通知 app-market 记录元数据
- 删除 `pkg/appstore/storage/oci.go` 中的 `Upload` 方法（不再需要代理上传）
- 移除 `cmd/app-market/main.go` 中的 `ParseMultipartForm` 依赖

## Capabilities

### New Capabilities

- `artifact-direct-upload`: 定义 Harbor 临时凭据签发、直接上传、上传完成通知的契约

### Modified Capabilities

- `artifact-storage`: 修改 ART-003 场景的验收标准，从"API 不代理"细化为"API 签发临时凭据，客户端直传"（已在主规格中定义，需更新场景描述）

## Impact

- **cmd/app-market**: 移除代理上传 handler，新增凭据签发 + 上传确认 endpoint
- **pkg/appstore/storage**: 删除 `Upload` 方法，新增 Harbor 机器人账户创建客户端
- **pkg/appstore/models**: 新增 `UploadSession` 模型
- **Web Portal**: 文件上传流程改为两步：先获取凭据，再直传 Harbor
- **CLI**: 同样需要适配两步上传流程
- **Harbor**: 需启用机器人账户功能（Harbor 2.0+ 默认支持）