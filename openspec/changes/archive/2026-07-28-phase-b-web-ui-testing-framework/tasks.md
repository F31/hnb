## 1. Naive UI 集成

- [x] 1.1 安装依赖：`naive-ui`、`@vicons/ionicons5`、`unplugin-auto-import`、`unplugin-vue-components` 到 `web/shell` 和 `web/packages/ui-kit`
- [x] 1.2 配置 `vite.config.ts`：添加 `AutoImport`（导入 `vue`/`vue-router`/`pinia`/`naive-ui` API）和 `Components`（`NaiveUiResolver`）插件
- [x] 1.3 在 `App.vue` 中添加 `NConfigProvider` 包裹根组件，配置基础 `themeOverrides`
- [x] 1.4 重写 `HNBTable.vue`：使用 Naive UI `NDataTable` 替代手写表格，保留 `columns`/`data`/`loading` 接口
- [x] 1.5 更新 `ui-kit/package.json`：将 `naive-ui` 设为 peerDependency
- [x] 1.6 更新 `shell/package.json`：添加 `naive-ui` 和 `unplugin-auto-import`、`unplugin-vue-components`
- [x] 1.7 验证：`pnpm --filter @hnb/shell build` 通过，bundle 138KB（gzip 52KB）

## 2. Vitest 单元测试框架

- [x] 2.1 安装依赖：`vitest`、`@vue/test-utils`、`jsdom`、`@vitest/coverage-v8` 到 `web/` 根级
- [x] 2.2 创建 `web/vitest.config.ts`：配置 `jsdom` 环境，`include` 指向 `shell` 和 `ui-kit` 的 `__tests__/` 目录
- [x] 2.3 更新 `web/package.json`：添加 `test:run` 脚本（`vitest run`）和 `test` 脚本（`vitest` watch 模式）
- [x] 2.4 编写 `authStore` 测试：`src/stores/__tests__/authStore.test.ts`，覆盖 `setUser`、`clearAuth`、`isAuthenticated`、`login`/`logout` 行为
- [x] 2.5 编写 `contextStore` 测试：`src/stores/__tests__/contextStore.test.ts`，覆盖 `setTenant`、`currentTenant`、`clearContext` 行为
- [x] 2.6 编写 `pluginStore` 测试：`src/stores/__tests__/pluginStore.test.ts`，覆盖 `registerPlugin`、`getPlugin`、`pluginMap` 响应式
- [x] 2.7 编写 `permissionStore` 测试：`src/stores/__tests__/permissionStore.test.ts`，覆盖 `hasPermission`、`setPermissions` 行为
- [x] 2.8 编写 `navigationStore` 测试：`src/stores/__tests__/navigationStore.test.ts`，覆盖 `setNavigationItems`、`activeItem` 行为
- [x] 2.9 编写 `EventBus` 测试：`src/core/__tests__/event-bus.test.ts`，覆盖 `on`/`emit`/`off` 单例行为
- [x] 2.10 编写 `RouterManager` 测试：`src/core/router/__tests__/RouterManager.test.ts`，覆盖 `init`、`addRoutes`、`getRouter` 行为
- [x] 2.11 编写 `HNBTable` 组件测试：`src/components/__tests__/HNBTable.test.ts`，覆盖 columns 渲染、loading 状态、空数据渲染
- [x] 2.12 验证：`pnpm test:run` 全部通过（49 tests, 8 test files）

## 3. Playwright E2E 测试框架

- [x] 3.1 安装依赖：`@playwright/test` 到 `web/` 根级
- [x] 3.2 创建 `web/playwright.config.ts`：配置 `webServer` 指向 `pnpm --filter @hnb/shell dev`，`testDir` 指向 `e2e/`，Chromium headless 模式
- [x] 3.3 创建 `web/e2e/` 目录，安装 Playwright 浏览器：`npx playwright install chromium`
- [x] 3.4 更新 `web/package.json`：添加 `test:e2e` 脚本（`playwright test`）
- [x] 3.5 编写登录 E2E 测试：`e2e/login.spec.ts`，覆盖有效登录、无效登录
- [x] 3.6 编写租户选择 E2E 测试：`e2e/tenant-select.spec.ts`，覆盖工作空间选择器展示
- [x] 3.7 编写错误页面 E2E 测试：`e2e/errors.spec.ts`，覆盖 404 页面和未认证重定向
- [x] 3.8 验证：`pnpm test:e2e` 全部通过（5 tests, 5 passed）

## 4. 最终验证

- [x] 4.1 运行 `pnpm -r typecheck` 确认全部类型检查通过（10/10 通过）
- [x] 4.2 运行 `pnpm -r build` 确认全部构建通过（8/8 通过）
- [x] 4.3 运行 `pnpm test:run` 确认全部单测通过（49 tests, 8 test files）
- [x] 4.4 运行 `pnpm test:e2e` 确认全部 E2E 通过（5 tests, 5 passed）
- [x] 4.5 运行 `openspec validate --all --strict` 确认规约合规