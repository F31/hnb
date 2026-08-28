## Context

Phase A 建立了 Web Console 的可编译基线（72 modules 转换 + 6 lazy chunks，122KB main bundle），但 UI 组件和测试基础设施为零。当前 `@hnb/ui-kit` 仅导出一个手写 HNBTable，`@hnb/shell` 无任何测试。插件系统的 7 个 plugin 同样无测试。CI 仅有 `build` 和 `typecheck`，无 `test` 阶段。所有改动只能靠手动验证。

## Goals / Non-Goals

**Goals:**
- Naive UI 作为全局组件库集成到 `@hnb/shell`，`ui-kit` 包装 Naive UI 组件提供 HNB 品牌扩展
- Vitest 单测框架覆盖 `shell` 核心模块（stores、router、plugin-loader、event-bus、layout-manager）和 `ui-kit` 组件
- Playwright E2E 框架覆盖登录、租户选择、仪表盘访问、404/错误页面的核心用户旅程
- 所有测试可在 CI 的 headless 环境中运行

**Non-Goals:**
- 不改写现有页面业务逻辑（登录、租户选择、仪表盘、错误页）
- 不引入 Tauri 或其他桌面框架
- 不要求现有暗色主题与 Naive UI 主题完全一致（Phase C 可统一）
- 不要求所有 plugin 立即有测试（仅 shell 和 ui-kit）
- 不引入 Storybook 或其他组件展示工具

## Decisions

### D1: Naive UI 全局注册 vs 按需自动导入
**决策：按需自动导入（unplugin-auto-import + unplugin-vue-components）**
- 选项 A：全局注册全部组件 → 增大 bundle 体积（~200KB+），不推荐
- 选项 B：手动按需 import → 样板代码多，开发体验差
- 选项 C：unplugin-auto-import API + unplugin-vue-components 解析模板 → 零样板代码，tree-shaking 自然生效，已被 Naive UI 官方推荐
- 选择 C，在 vite.config.ts 中配置 `Components({ resolvers: [NaiveUiResolver()] })` 和 `AutoImport({ imports: ['vue', 'vue-router', 'pinia', 'naive-ui'] })`

### D2: ui-kit 的角色
**决策：ui-kit 作为 Naive UI 组件的品牌包装层**
- 不直接暴露 Naive UI 原始组件，而是通过 ui-kit 提供 HNB 品牌组件（如 `HNBTable` 包装 `NDataTable` + 默认样式）
- ui-kit 的 `naive-ui` 是 peerDependency，由 shell 提供运行时实例
- 允许 plugin 通过 `@hnb/ui-kit` 引入品牌组件，无需直接依赖 Naive UI

### D3: Vitest 使用 jsdom vs happy-dom
**决策：使用 jsdom**
- happy-dom 更快但相容性问题较多（尤其是在 Vue 组件测试中）
- jsdom 生态成熟，与 `@vue/test-utils` 配合稳定
- 配置 `vitest.config.ts` 在 `web/` 根级，覆盖 `shell` 和 `ui-kit`

### D4: 测试文件位置
**决策：co-located（紧邻被测试文件）**
- 单元测试放在 `__tests__/` 目录紧邻源文件，如 `src/stores/__tests__/authStore.test.ts`
- E2E 测试放在 `web/e2e/` 根级目录
- 避免引入 `tests/` 顶层目录造成的 import 路径混乱

### D5: Playwright 配置
**决策：单 project 配置，dev server 直连**
- `web/playwright.config.ts` 配置 `webServer` 指向 `pnpm --filter @hnb/shell dev`
- 使用 `projects` 定义登录态复用（通过 `storageState`）
- 暂不配置多浏览器矩阵（仅 Chromium headless），Phase C 可扩展

## Risks / Trade-offs

- **[Bundle 膨胀]** unplugin-auto-import 可能引入未使用的 Naive UI 组件 → Mitigation: 配置 `AutoImport` 的 `imports` 仅包含实际使用的模块，构建后通过 `vite build --report` 监控
- **[测试维护成本]** 单测覆盖核心模块后，重构时需同步更新测试 → Mitigation: 将测试纳入 CI 门禁，`pnpm test` 失败阻断 merge
- **[Playwright 稳定性]** E2E 测试依赖 dev server 和 mock API → Mitigation: 使用 `page.route()` 拦截 API 请求，不依赖后端服务
- **[Naive UI 版本兼容]** Naive UI 的 breaking change 可能影响 ui-kit 包装层 → Mitigation: 锁定 `naive-ui` 主版本，ui-kit 组件不直接透传 props 以保证接口稳定
- **[非目标：主题统一]** 当前暗色主题与 Naive UI 默认主题不匹配，用户可能感知 UI 不一致 → Mitigation: Phase B 仅引入组件库，主题统一作为 Phase C 任务

## Migration Plan

1. 安装依赖：`naive-ui`、`@vicons/ionicons5`、`unplugin-auto-import`、`unplugin-vue-components`、`vitest`、`@vue/test-utils`、`@playwright/test`、`jsdom`
2. 配置 `vite.config.ts`：添加 AutoImport + Components 插件；Naive UI 按需加载
3. 配置 `vitest.config.ts`：jsdom 环境、coverage 配置
4. 配置 `playwright.config.ts`：dev server、storageState
5. 重写 `HNBTable.vue`：使用 Naive UI `NDataTable`
6. 注册 Naive UI 主题：在 `main.ts` 或 `App.vue` 中通过 `NConfigProvider` 包裹
7. 编写测试用例：核心模块单元测试 + 核心旅程 E2E
8. 更新 `package.json` scripts：`test`、`test:run`、`test:e2e`
9. 验证：`pnpm test:run` + `pnpm test:e2e` + `pnpm build` 全部通过

回滚策略：移除依赖、删除配置文件和测试文件、恢复 `HNBTable.vue` 原始版本。