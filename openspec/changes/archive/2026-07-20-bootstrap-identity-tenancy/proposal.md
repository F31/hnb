## Why

企业多租户是 HNB Cloud 的核心隔离模型。TENANT-001 至 TENANT-004 已在主规格中定义，但缺少租户上下文传播、跨租户访问控制、RBAC 和审批策略、以及 SecretReference 凭据管理的实现设计。没有该实现，多租户的部署、市场、AI 和 Edge 场景无法安全隔离。

## What Changes

- 实现 Tenant Context 在 API、数据库、缓存、事件、审计、Provider 调用和可观测数据中的全链路传播，包含 namespace 上下文。
- 实现跨租户默认拒绝的访问控制中间件，支持显式授权对象建立共享，授权作用域支持 tenant/project/namespace 三级。
- 实现 RBAC 角色模型（平台管理员、租户管理员、项目管理员、运维人员、发布者、只读用户）及审批策略绑定。
- 实现 SecretReference 凭据服务，确保运行凭据仅由平台凭据服务或目标侧工作负载身份按最小权限解析。
- 新增数据库迁移创建租户上下文传播、授权、Namespace 映射和凭据引用表。
- 扩展 Portal API 添加租户管理、项目/环境/Namespace 管理、角色管理、审批策略配置界面。

## Capabilities

### New Capabilities
- `identity-tenancy`: 定义企业多租户体系中租户、项目、环境、身份、角色、权限与审批上下文在 API、事件、Provider 和可观测链路中的传播、隔离和最小凭据暴露行为。

### Modified Capabilities
- `portal-experience`: 增加租户管理、角色管理、审批策略配置界面。

## Impact

- **代码:** Tenant Context Middleware、RBAC Enforcer、Authorization Service、SecretReference Service、Tenant Management API、Portal UI 组件。
- **API/事件:** 新增租户/角色/权限/审批管理 API，扩展现有 API 请求和事件添加租户上下文验证。
- **数据:** 新增租户上下文传播、授权、角色、审批策略和凭据引用表；数据库迁移需支持回滚。
- **依赖:** 复用 PostgreSQL，不新增强制中间件；SecretReference 依赖平台凭据服务。
- **资源:** Minimal 使用内置 RBAC 模型；Enterprise 可对接外部 IdP。
- **运维:** 增加租户创建、角色分配、审批策略配置和凭据轮换 Runbook。