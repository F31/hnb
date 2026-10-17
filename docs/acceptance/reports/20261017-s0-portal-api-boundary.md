# S0 1.2 Portal / API / CLI 边界确认

- Run ID：`s0-portal-api-boundary-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirements：UXLOOP-001、DEVEX-001
- 日期：2026-10-17
- 结论：**边界确认通过**；Portal 仓库/维护者/版本/测试入口已固定。

## 1. 环境与范围

| 项 | 值 |
|---|---|
| 仓库 | 同仓 `web/`（pnpm workspace） |
| 维护者 | `jinfeng105 <jinfeng105@126.com>`（当前唯一作者；无 CODEOWNERS） |
| 版本 | `hnb-web-console` / `@hnb/shell` / `@hnb/api-client` `0.1.0` |
| Node / Vue / Vite | >=22.12 / 3.5.40 / 6.4.3 |
| 后端契约 | `contracts/openapi/{console,platform,storage,app-market}/v1`；`contracts/schema/*` |
| 范围 | 仓库、维护者、构建版本、单测/构建/契约入口、Portal/BFF/CLI/Provider 边界 |

## 2. 验证命令与结果

| 命令 | 结果 |
|---|---|
| `pnpm -C web typecheck` | 通过（shell + 7 plugins） |
| `pnpm -C web test:run` | **65 files / 342 tests passed** |
| `pnpm -C web build` | 通过，输出 `web/shell/dist` |
| `pnpm -C web boundary:check` | 通过：业务请求只经 `@hnb/api-client` |
| `npm run contracts:test` | 28/28（本轮此前已通过；契约未改） |
| `npm run contracts:validate` | 81 schemas，compatibility=checked（本轮此前已通过） |

## 3. 边界修正

- 移除 `web/plugins/dashboard/src/pages/ApprovalList.vue` 对 `/api/v1/operations` 的直接
  `fetch`，改为插件 `ctx.apiClient` 注入的 `@hnb/api-client`。
- 新增 `web/scripts/check-api-boundary.mjs` 与 `pnpm -C web boundary:check`，扫描
  `shell/src`、`plugins`、`packages` 中业务代码的直接 `fetch`；仅允许
  `@hnb/api-client` 自身、登录引导 `authStore` 和插件资产 `PluginLoader`。
- 边界详情见 `docs/deployment/portal-api-boundary.md`。

## 4. 明确未覆盖

- Playwright E2E 需要实际 BFF/Portal 地址，本轮未执行；S2/S3 的 UI 旅程仍需独立验收。
- 未构建/推送 Portal 镜像，未在 Kubernetes 部署；不提升发布或生产资格。
- 维护者来自当前 git 作者历史；未建立 CODEOWNERS/组织级 owner 流程。
- 契约测试/门禁为提交前结果；本轮边界改动未修改 OpenAPI/JSON Schema。

## 5. 证据身份

| 文件 | 说明 |
|---|---|
| `docs/deployment/portal-api-boundary.md` | 固定边界正文 |
| `web/scripts/check-api-boundary.mjs` | 静态边界守卫 |
| `web/plugins/dashboard/src/api.ts` | 插件 API client 注入 |
| `web/plugins/dashboard/src/pages/ApprovalList.vue` | 移除直接 fetch |
| `web/package.json` | `boundary:check` 入口 |

源文件摘要以提交 `51ab3c4` 为基线；上述边界文件在提交后新增/修改，工作树身份与本报告
一并保留，不代表生产认证。
