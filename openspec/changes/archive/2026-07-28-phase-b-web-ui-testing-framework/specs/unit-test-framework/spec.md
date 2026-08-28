## ADDED Requirements

### Requirement: [TEST-UNIT-001] Portal SHALL 提供 Vitest 单元测试框架
Portal SHALL 在 `web/` 根级配置 Vitest，使用 jsdom 环境，覆盖 `@hnb/shell` 和 `@hnb/ui-kit` 包。测试框架 SHALL 支持 Vue SFC、TypeScript 和 Pinia stores 测试。

**Traceability:** PHASE-B-005

#### Scenario: 运行单测
- **GIVEN** Vitest 已配置
- **WHEN** 执行 `pnpm test:run` 或 `pnpm --filter <package> test:run`
- **THEN** 所有测试文件被执行
- **AND** 测试结果输出到终端
- **AND** 覆盖率报告生成到 `coverage/` 目录

### Requirement: [TEST-UNIT-002] 核心模块 SHALL 有单元测试覆盖
`@hnb/shell` 的以下核心模块 SHALL 有单元测试：AuthStore、ContextStore、NavigationStore、PermissionStore、PluginStore、RouterManager、LayoutManager、EventBus、PluginLoader、NavigationManager。

**Traceability:** PHASE-B-006

#### Scenario: Store 测试示例
- **GIVEN** AuthStore 已初始化
- **WHEN** 调用 `setUser({ username: 'admin' })`
- **THEN** `user` 响应式状态更新为 `{ username: 'admin' }`
- **AND** `isAuthenticated` 计算属性返回 `true`

### Requirement: [TEST-UNIT-003] 测试文件 SHALL 与被测文件 co-located
测试文件 SHALL 放在被测文件所在目录的 `__tests__/` 子目录中，命名格式为 `<fileName>.test.ts`。

**Traceability:** PHASE-B-007

#### Scenario: 测试文件位置
- **GIVEN** 源文件 `src/stores/authStore.ts`
- **WHEN** 创建测试
- **THEN** 测试文件位于 `src/stores/__tests__/authStore.test.ts`

### Requirement: [TEST-UNIT-004] 覆盖率 SHALL 有基础门槛
`@hnb/shell` 核心模块的语句覆盖率 SHALL 不低于 60%，`@hnb/ui-kit` 组件覆盖率 SHALL 不低于 50%。

**Traceability:** PHASE-B-008

#### Scenario: 覆盖率未达标
- **GIVEN** 核心模块覆盖率低于 60%
- **WHEN** 执行 `pnpm test:run --coverage`
- **THEN** 终端输出覆盖率摘要
- **AND** 测试进程可正常退出（非强制门禁，仅监控）