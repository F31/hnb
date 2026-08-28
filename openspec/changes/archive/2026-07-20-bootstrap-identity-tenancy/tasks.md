## 1. Contracts and Schema

- [x] 1.1 `[TENANT-005]` 定义 Tenant Context 传播的 API Schema 和事件契约（TenantContext、AuthorizationRequest、NamespaceRef 等），包含 namespace_id 上下文，生成 Go/TypeScript SDK；证据：Schema lint、兼容检查和生成物测试。
- [x] 1.2 `[TENANT-007]` 定义 RBAC 角色、权限矩阵和审批策略的版本化 API Schema；证据：策略契约测试及示例配置。
- [x] 1.3 `[TENANT-008]` 定义 SecretReference 凭据管理的 API 和事件 Schema；证据：Schema lint 和兼容检查。

## 2. Database and Migration

- [x] 2.1 `[TENANT-005]` 创建 Tenant、Project、Environment、Namespace 表、索引及租户约束的 expand 数据库迁移（Environment 增加 project_id FK，Namespace 支持多 namespace 场景）；证据：空库和存量升级测试。
- [x] 2.2 `[TENANT-007]` 创建 Role、UserRole、ApprovalPolicy 表及索引；证据：并发、唯一性和策略查询测试。
- [x] 2.3 `[TENANT-008]` 创建 SecretReference 表，含加密存储、版本管理和轮换策略；证据：迁移、回滚/前向修复和数据保留测试。
- [x] 2.4 `[TENANT-005][TENANT-006]` 实施数据库租户隔离策略，确保所有查询默认包含 tenant_id 过滤（含 namespace 表）；证据：跨租户/跨项目拒绝测试。

## 3. Tenant Context Middleware

- [x] 3.1 `[TENANT-005]` 实现 Tenant Context Middleware，从 JWT 或 API Key 提取租户信息（含 namespace_id）并注入 Go context；证据：全链路传播集成测试。
- [x] 3.2 `[TENANT-006]` 实现跨租户访问控制中间件，默认拒绝跨租户请求，支持显式授权对象；证据：跨租户拒绝和授权对象测试。

## 4. Authorization Service and RBAC

- [x] 4.1 `[TENANT-007]` 实现 RBAC 策略引擎（Casbin 或等效），支持六种角色和角色继承，以及 tenant/project/namespace 三级作用域；证据：角色权限矩阵单元测试。
- [x] 4.2 `[TENANT-007]` 实现审批策略引擎，绑定 Operation 类型到审批角色；证据：审批流和 PendingApproval 状态测试。
- [x] 4.3 `[TENANT-007]` 实现 Tenant Management API（CRUD 租户、项目、环境、namespace、角色、用户角色分配）；证据：API 集成测试。

## 5. SecretReference Service

- [x] 5.1 `[TENANT-008]` 实现 SecretReference 凭据服务，支持 AES-256-GCM 加密存储和 SecretReference 格式解析；证据：加密存储和引用解析测试。
- [x] 5.2 `[TENANT-008]` 实现 Secret 轮换和版本管理；证据：轮换和版本回滚测试。

## 6. Portal UI

- [x] 6.1 `[UX-005]` 实现 Vue 租户管理页面（列表、创建、编辑、详情及租户级项目/环境管理，含 namespace 管理视图）；证据：组件和浏览器 E2E 测试。
- [x] 6.2 `[UX-005]` 实现 Vue 角色管理、用户角色分配和审批策略配置页面；证据：表单 Schema、权限和敏感字段遮蔽测试。
- [x] 6.3 `[UX-005]` 实现 SecretReference 管理页面（创建、查看、轮换、删除）；证据：组件和权限测试。

## 7. Security, Reliability, and Observability

- [x] 7.1 `[TENANT-005][TENANT-006]` 覆盖 Tenant Context 全链路传播的租户隔离和越权审计（含跨项目/跨 namespace 场景）；证据：跨租户安全测试报告。
- [x] 7.2 `[TENANT-008]` 扫描 SecretReference 存储、日志、链路和审计，确认无明文凭据泄露；证据：敏感信息测试报告。
- [x] 7.3 `[TENANT-005][TENANT-007]` 暴露 Tenant Context Middleware 延迟、授权决策延迟、租户数、项目数、环境数、namespace 数、角色数等指标；证据：仪表盘、告警和示例链路。
- [x] 7.4 `[TENANT-008]` 注入 Secret 服务故障，验证降级行为和数据不丢失；证据：故障矩阵和恢复报告。

## 8. Rollout, Recovery, and Documentation

- [x] 8.1 `[TENANT-005]` 以影子模式部署 Tenant Context Middleware，比较请求上下文和访问日志，不阻断请求；证据：差异报告。
- [x] 8.2 `[UX-005]` 按 Portal-only、租户管理、角色管理、审批策略的灰度顺序，并验证每阶段回滚；证据：灰度和回滚记录。
- [x] 8.3 `[TENANT-005][TENANT-006][TENANT-007][TENANT-008]` 编写租户、项目/环境/namespace、角色、审批策略、SecretReference 管理 Runbook；证据：文档评审和桌面演练。
- [x] 8.4 `[TENANT-005][TENANT-006][TENANT-007][TENANT-008]` 执行完整租户创建、项目/环境/namespace 管理、角色分配、审批策略、SecretReference 管理、跨租户拒绝 E2E；证据：Requirement 映射报告。
- [x] 8.5 `[TENANT-005][TENANT-006][TENANT-007][TENANT-008]` 运行 `openspec validate --all --strict`、完成 verify、同步规格并归档；证据：零阻断校验、verify 和 sync/archive 记录。