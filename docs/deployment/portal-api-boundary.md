# Portal / API / CLI 公开边界（S0 1.2）

状态：**已确认并固定**（2026-10-17）；本文件是后端、Portal、CLI 的协作边界，
不等于任何功能的运行验收或生产认证。

## 1. 仓库、维护者与版本

| 项 | 结论 |
|---|---|
| Portal 仓库 | 与本仓库同仓，根目录 `web/`；不存在独立 Portal 仓库，无“缺仓库”阻塞 |
| 维护者 | `jinfeng105 <jinfeng105@126.com>`（`git log -- web` 当前唯一作者）；未配置 CODEOWNERS |
| 工作区 | pnpm workspace：`shell`、`plugins/*`、`packages/*` |
| 版本 | `hnb-web-console` / `@hnb/shell` / `@hnb/api-client` 均为 `0.1.0` |
| 运行时 | Node.js >= 22.12；Vue 3.5.40；Vite 6.4.3 |
| 构建 | `pnpm -C web build` → `web/shell/dist`；Chart 为 `deploy/charts/hnb/charts/portal` |
| 开发代理 | `/api` 与 `/v1` → `http://localhost:8001`（Console BFF） |

## 2. 固定入口

| 入口 | 命令 |
|---|---|
| 前端类型检查 | `pnpm -C web typecheck` |
| 前端单测 | `pnpm -C web test:run`（Vitest） |
| 前端构建 | `pnpm -C web build` |
| 前端 E2E | `PLAYWRIGHT_BASE_URL=http://<host>:<port> pnpm -C web test:e2e`（需已运行 BFF/Portal） |
| API 边界守卫 | `pnpm -C web boundary:check` |
| 契约测试/校验 | `npm run contracts:test`、`npm run contracts:validate` |
| 契约生成漂移 | `npm run contracts:generate -- --check` |
| CLI | `go test ./cmd/hnbctl/...`；构建 `go build -o hnbctl ./cmd/hnbctl` |

## 3. 公开 API 边界

| 调用方 | 允许 | 禁止 |
|---|---|---|
| Portal Shell / 插件 | 经 `@hnb/api-client` 访问同源 Console BFF `/api/v1`；统一 token、tenant/space/environment/cluster 上下文头、traceId、401 单飞刷新、超时/取消 | 直接 `fetch` 业务 API；直连 Provider、NATS、数据库或 RuntimeTarget |
| Portal 登录引导 | 仅 `authStore` 对 `/api/v1/auth/login`、`logout`、`refresh` 使用原始 `fetch`（此时尚无可用 token） | 用原始 `fetch` 访问任何业务资源 |
| Portal 插件加载 | `PluginLoader` 拉取插件 entry 资产 | 把插件资产加载当作业务 API 通道 |
| Console BFF / apiserver | 消费 `contracts/openapi/console`、`platform`、`storage`、`app-market` 与 `contracts/schema/*`；写操作走 RuntimeIntent/Operation | 由 Portal 直写领域库；绕过权限、租户和 Operation |
| CLI `hnbctl` | 纳管/操作走服务端权威 API（如 `cluster-list/v1`、Operation）；OS/预检/渲染/安装记录在本地完成 | 直连数据库；平台不可达时回退本地伪视图；把“计划”当执行成功 |
| platform-api / Provider | 使用生成的 Go/TS 契约与共享 Schema；按 owning plane 读写 | 复制 IAM 私钥；跨 owning plane 直接写表；私有契约漂移 |

## 4. 契约与版本

- 浏览器面向契约：`contracts/openapi/console/v1/openapi.yaml`（Console BFF）；
  Platform/Storage/App Market 分别有独立 v1 OpenAPI。
- JSON Schema：`contracts/schema/*/v1/*.schema.json`，以 `$id`/路径体现主版本。
- 生成物：`contracts/generated/go`、`contracts/generated/typescript`；不得手改，
  由 `npm run contracts:generate` 生成并由 `--check` 阻断漂移。
- v1 只做向后兼容增量；破坏性变更必须新主版本/新 Schema，不允许覆盖旧文件。

## 5. 已知例外与后续

- `authStore` 的三个 auth 端点保留原始 `fetch`；它们是登录引导边界，不携带业务数据。
- `PluginLoader` 的 `fetch(entry)` 是插件资产加载，不是 API 调用。
- `pnpm -C web boundary:check` 已阻止业务代码新增直接 `fetch`；它只做静态入口守卫，
  不能替代运行时权限、租户隔离或 E2E。
- Playwright E2E 需要实际 BFF/Portal 地址，本次未执行；不能据此声明 S2/S3 UI 验收。
