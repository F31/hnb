## ADDED Requirements

### Requirement: [TEST-E2E-001] Portal SHALL 提供 Playwright E2E 测试框架
Portal SHALL 在 `web/` 根级配置 Playwright，使用 Chromium headless 模式，覆盖核心用户旅程。E2E 测试 SHALL 通过 `page.route()` 拦截 API 请求，不依赖后端服务。

**Traceability:** PHASE-B-009

#### Scenario: 运行 E2E 测试
- **GIVEN** Playwright 已配置
- **WHEN** 执行 `pnpm test:e2e`
- **THEN** Playwright 启动 dev server
- **AND** 所有 E2E spec 按顺序执行
- **AND** 测试结果输出到终端
- **AND** 失败时截图保存到 `test-results/`

### Requirement: [TEST-E2E-002] 登录流程 SHALL 有 E2E 测试覆盖
登录页面 SHALL 覆盖以下场景：使用有效凭据成功登录、使用无效凭据显示错误、已登录用户自动跳转到租户选择页。

**Traceability:** PHASE-B-010

#### Scenario: 有效登录
- **GIVEN** 用户访问登录页
- **WHEN** 输入有效凭据并提交
- **THEN** 页面跳转到租户选择页
- **AND** localStorage 中存储了 auth token

#### Scenario: 无效登录
- **GIVEN** 用户访问登录页
- **WHEN** 输入无效凭据并提交
- **THEN** 页面显示错误提示
- **AND** URL 仍为登录页

### Requirement: [TEST-E2E-003] 租户选择 SHALL 有 E2E 测试覆盖
租户选择 SHALL 覆盖以下场景：用户在租户列表中选择一个租户后跳转到仪表盘。

**Traceability:** PHASE-B-011

#### Scenario: 选择租户
- **GIVEN** 用户已登录且进入租户选择页
- **WHEN** 点击一个租户
- **THEN** 页面跳转到仪表盘
- **AND** 仪表盘显示该租户上下文

### Requirement: [TEST-E2E-004] 错误页面和导航 SHALL 有 E2E 测试覆盖
错误页面 SHALL 覆盖以下场景：访问不存在的路由显示 404 页面、未认证用户访问受保护路由被重定向到登录页。

**Traceability:** PHASE-B-012

#### Scenario: 404 页面
- **GIVEN** 用户已登录
- **WHEN** 访问 `/nonexistent-route`
- **THEN** 页面显示"页面未找到"或等效错误信息
- **AND** HTTP 状态码（SPA 内）对应 404 路由

#### Scenario: 未认证重定向
- **GIVEN** 用户未登录
- **WHEN** 访问 `/dashboard`
- **THEN** 页面重定向到登录页