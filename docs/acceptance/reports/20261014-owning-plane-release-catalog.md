# Owning-plane release material catalog — 2026-10-14

## 结果与范围

统一运营控制台/管理平台、控制面与 worker 安装所需物料及离线副本的发布目录，
落在 owning-plane 上。本轮新增 owning-plane 发布目录端点与存储，并保持与安装器
本地 `release-catalog/v1` 一致的内容身份语义。该目录不做安装执行，只登记已验证
闭包的身份；角色依赖缺失的检查在安装器上传前完成，owning-plane 不持有 bundle 内容。

scope 不含：CLI/控制台对 `GET/POST /v1/releases` 的封装展示（属 7.7）、真机离线
断网验收（7.2/7.3 继续）。

## 交付内容

- 迁移 `database/postgresql/migrations/104_release_catalog_material.sql`：
  `release_catalog_entries`，`role IN ('console','control-plane','worker')`，
  `lock_digest`/`content_digest` 强制 `^sha256:[0-9a-f]{64}$`，
  `UNIQUE (release_id, release_version, role)`。已将迁移手动应用于真实 PostgreSQL
  16：重复角色约束、非法摘要 CHECK 均按预期拒绝。
- `cmd/platform-api/internal/store/release_catalog.go`：`RegisterReleaseMaterial`
  （行锁内决策——同摘要幂等返回既有条目，异摘要返回 `ErrFixedReleaseConflict`）、
  `ListReleaseMaterials`。挂入 `store.Store` 聚合接口；`mysqlStubStore` 仅占位
  not-implemented（平台产品后端仍为 PostgreSQL 单一路径）。
- `cmd/platform-api/internal/api/releases.go` + `server.go` 路由：
  - `POST /v1/releases`（releaseCatalog/execute）：strict JSON，role 枚举、必填、
    sha256 摘要校验；409 `CONFLICT`（固定发布异摘要）、400 `VALIDATION_FAILED`、403。
  - `GET /v1/releases`（releaseCatalog/list）：返回所有登记的角色条目。
- 安装器侧配套（此前已交付，作为 7.6 前置）：`toolkit/release_catalog.go`
  `ValidateRoleClosure` + `ReleaseImportCatalog`，三种安装角色关联同一 ReleaseLock。

## 双入口一致性设计

- 安装器本地目录与 owning-plane 目录都记录同一 `lockDigest`/`contentDigest`；角色
  闭包校验使用同一 `release-lock/v1` 与 `offline-bundle/v1` 身份。owning-plane 登记
  接受的内容身份必须已被安装器 `ValidateRoleClosure` 验证过（上传侧约定）。
- 平台安装前使用本地清单校验；平台运行后 CLI/控制台使用 owning-plane 目录
  （7.7 接入展示）。

## 自动测试与保留证据

在 `cmd/hnbctl`：

```sh
go test -race ./internal/toolkit/ -run 'Offline|ReleaseCatalog|RoleClosure|Prepare' -count=1
go test ./...   # 上轮全量
```

新测试：`release_catalog_test.go`（toolkit）——幂等导入、异摘要固定发布拒绝、
未知角色拒绝、`os-package-index` 缺失按 OS/arch 拒绝、持久化严格读取。

在 `cmd/platform-api`：

```sh
go test ./internal/api/ -run 'ReleaseMaterial' -count=1          # API 表驱动
HNB_TEST_POSTGRES_DSN=... go test ./internal/store/ -run 'TestReleaseCatalogPG' -count=1 -v
go vet ./internal/store/ ./internal/api/
```

- `TestReleaseCatalogPGIdempotentAndFixedReleaseRejected` 真实 PostgreSQL 16 独立
  schema 通过：首插返回 id、同摘要重放 created=false 且 id 不变、异摘要返回
  `ErrFixedReleaseConflict` 且列表不变、新版本可新增条目。
- API 表驱动：201 created、重复 200 幂等、409 固定发布异摘要、400（角色/摘要/必填/
  未知字段）、403 无权限。
- 共享探针库上 `TestRBAC_HighRiskOperationRequiresApproval`、
  `TestResolveSecretReferenceMetadata` 失败源于该库缺少 `execution_plans`/`tenants`
  等更早迁移表（探针库未跑全量迁移），与本次改动无关；不影响 `release_catalog` 相关
  用例。

## 未覆盖

平台服务凭据分发/轮换、真机离线断网验收、UI/CLI 对发布目录的展示封装均仍为独立
任务（7.7 等）。owning-plane 目录仅为结构化登记，不构成断网安装通过证明。

## 7.7 CLI 侧接入

（2026-10-14 附加。7.7 整体未完成，本节约定了 CLI 入口的交付。）

- `toolkit/releases_client.go`：`RegisterReleaseMaterial`（POST `/api/v1/releases`，
  201=新登记、200=重复幂等、409=固定发布异摘要、5xx/超时=不可用）、
  `ListReleaseMaterials`（GET 目录）。
- `cli/release.go`：`release register|list|verify`。
  - register：先本地 `VerifyOfflineBundle`（必须 complete）与
    `ValidateRoleClosure`（角色依赖缺一不可），通过后才提交同一内容身份；
  - list：读取 owning-plane 目录；
  - verify：**纯离线**，拒绝 `--platform-url`/`--token-file`，不可能回退到任何
    网络源；bundle/lock/blob 全在本地验证。
- 测试（`toolkit/releases_client_test.go`、`cli/release_test.go`）：
  离线 verify 成功、verify 拒绝平台参数、register 创建/幂等重放/固定发布 409、
  list 目录解码、无效角色本地拒绝。`go test ./internal/toolkit ./internal/cli -run 'Release'` 全部通过；
  全模块 `go test ./...` 与 `go vet ./...` 通过。

未交付且使 7.7 保持未勾选的部分：控制台前端对同一目录的展示（两入口一致性）、
"包齐备"与"OS/拓扑认证"的并列展示，以及真实断网环境下以同一发布包核对两个入口。