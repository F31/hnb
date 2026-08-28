## ADDED Requirements

### Requirement: [UI-LIB-001] Portal SHALL 基于 Naive UI 提供标准组件库
Portal SHALL 使用 Naive UI 作为底层组件库，`@hnb/ui-kit` SHALL 包装 Naive UI 组件提供 HNB 品牌统一外观。所有 Portal 页面和插件 SHALL 通过 `@hnb/ui-kit` 而非直接使用 Naive UI 组件。

**Traceability:** PHASE-B-001

#### Scenario: 品牌组件替换
- **GIVEN** 一个使用 HNBTable 的页面
- **WHEN** 页面渲染
- **THEN** 表格使用 Naive UI NDataTable 渲染
- **AND** 列排序、筛选、分页由 NDataTable 原生支持

### Requirement: [UI-LIB-002] Portal SHALL 按需加载 Naive UI 组件
Naive UI 组件 SHALL 通过 `unplugin-auto-import` 和 `unplugin-vue-components` 实现开发时自动引入，构建时 tree-shaking 移除未使用组件。

**Traceability:** PHASE-B-002

#### Scenario: 自动引入生效
- **GIVEN** 一个 Vue SFC 模板中使用 `<n-button>`
- **WHEN** 组件编译
- **THEN** 无需手动 `import { NButton } from 'naive-ui'`
- **AND** 构建产物仅包含 NButton 及其依赖

### Requirement: [UI-LIB-003] Portal SHALL 提供主题配置入口
Portal SHALL 在 `App.vue` 或根组件中使用 `NConfigProvider` 包裹，通过 `themeOverrides` 统一 Naive UI 组件样式。

**Traceability:** PHASE-B-003

#### Scenario: 全局主题生效
- **GIVEN** Portal 已加载
- **WHEN** 任意 Naive UI 组件渲染
- **THEN** 组件样式遵循 Portal 主题覆盖配置
- **AND** 主题覆盖不破坏布局伸缩性

### Requirement: [UI-LIB-004] 插件 SHALL 通过 @hnb/ui-kit 使用 Naive UI 组件
插件 SHALL NOT 直接 import `naive-ui`，必须通过 `@hnb/ui-kit` 导入品牌组件。`@hnb/ui-kit` SHALL 在 `package.json` 中将 `naive-ui` 列为 peerDependency。

**Traceability:** PHASE-B-004

#### Scenario: 插件使用 NButton
- **GIVEN** 一个插件使用按钮
- **WHEN** 引入组件
- **THEN** 使用 `import { HNBButton } from '@hnb/ui-kit'` 而非 `import { NButton } from 'naive-ui'`