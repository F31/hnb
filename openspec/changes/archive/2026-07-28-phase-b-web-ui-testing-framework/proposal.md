## Why

Phase A 清理了 Web Shell 的致命缺陷，建立了可编译、可构建的基线。但当前 Web Console 仍缺乏组件库和测试基础设施：`ui-kit` 仅含一个手写 HNBTable，无法支撑复杂业务表单/表格/弹窗/菜单等场景；零测试覆盖导致每次改动只能靠手动验证。Phase B 引入 Naive UI 作为标准组件库，同时搭建 Vitest 单测和 Playwright E2E 测试框架，为后续所有 Portal 功能开发提供 UI 组件基础和测试安全网。

## What Changes

- **引入 Naive UI**：作为全局组件库注册到 Vue 3 应用，替换 `ui-kit` 中自绘的 HNBTable 为 Naive UI 的 `NDataTable`，`ui-kit` 改为包装/扩展 Naive UI 组件
- **搭建 Vitest 单元测试**：在 `web/shell` 和 `web/packages/ui-kit` 中配置 Vitest + `@vue/test-utils`，为核心模块（stores、router manager、plugin loader、event bus）和 UI 组件编写基础单元测试
- **搭建 Playwright E2E 测试**：在 `web/` 根级配置 Playwright，为登录、租户选择、仪表盘访问等核心用户旅程编写 E2E 测试，配置 CI 可运行的 headless 模式
- **shell 构建脚本调整**：`build` 命令不再阻塞于 `vue-tsc --noEmit`（分离到 `typecheck` 脚本），vite 构建不依赖类型检查通过

## Capabilities

### New Capabilities
- `ui-component-library`: 基于 Naive UI 的 Portal 标准组件库，包括全局注册、主题定制、按需加载和组件扩展规范
- `unit-test-framework`: Vitest 单元测试框架，覆盖 Vue 组件、Pinia stores、工具函数和核心管理器
- `e2e-test-framework`: Playwright E2E 测试框架，覆盖核心用户旅程和页面导航

### Modified Capabilities
- `portal-experience`: 组件库替换后，Portal 的 UI 组件来源从自绘组件变为 Naive UI 标准组件，但 UX 行为需求不变，无需修改 requirement 级行为

## Impact

- **web/packages/ui-kit**：`HNBTable.vue` 改为使用 Naive UI `NDataTable`，`ui-kit` 依赖新增 `naive-ui`，`ui-kit` 的 `package.json` 添加 `naive-ui` 为 peerDependency
- **web/shell**：`package.json` 新增 `naive-ui` 依赖，`main.ts` 注册 Naive UI 全局组件，`vite.config.ts` 添加 Naive UI 按需加载样式
- **web/package.json**：新增 `@playwright/test` 和 `vitest` 等 devDependencies
- **所有 Plugin**：`package.json` 新增 `naive-ui` 依赖（或从 shell 透传，取决于包设计）
- **CI/CD**：新增 `test` 和 `test:e2e` 脚本，CI pipeline 需要安装 Playwright 浏览器依赖
- **pillar**：新增 `vitest.config.ts`、`playwright.config.ts`、`test/` 目录
- **非目标**：不引入 Tauri、不替换 Vue 3 Composition API 写法、不改动现有页面业务逻辑、不要求 Naive UI 主题与现有暗色主题完全一致