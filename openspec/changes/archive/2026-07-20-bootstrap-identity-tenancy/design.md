## Context

企业多租户是 HNB Cloud 的底层隔离模型。TENANT-001 至 TENANT-004 已在主规格中定义，但缺少实现设计。仅以 Tenant ID 字段存储不足以防止跨租户访问、凭据泄露和权限绕过。

本 change 面向平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户。设计必须保持 Tenancy 作为内核平面的一部分，不引入外部 IdP 依赖，并支持 Minimal 档位内置 RBAC 到 Enterprise 外部 IdP 对接。

Kubernetes namespace 是底层隔离单元，每个 tenant 通过 project → environment → namespace 层级映射到 K8s namespace。

### Architecture

```text
API Gateway / Platform API
    |
    v
Tenant Context Middleware
    |  (extracts tenant_id, project_id, environment_id, namespace_id, actor_id, roles)
    v
Authorization Service
    |  (RBAC enforcer, policy evaluation, namespace-scoped permissions)
    v
    ├── K8s Namespace Controller (optional: reconcile DB → K8s)
    ├── Alert/Notification Service (tenant-scoped)
    ├── Operation Engine (tenant-scoped)
    ├── Market Service (tenant-scoped)
    ├── Provider Service (tenant-scoped, SecretReference)
    └── Portal API (tenant-scoped, role-filtered UI)
```

Tenant Context 是内核平面的基础设施，所有服务复用同一中间件和授权服务。SecretReference 由平台凭据服务管理，各服务仅持有引用。

## Goals / Non-Goals

**Goals:**
- 实现 Tenant Context 在 API 请求、数据库查询、事件、审计、Provider 调用和可观测数据中的全链路传播，包含 namespace 上下文。
- 实现跨租户默认拒绝的访问控制中间件，支持显式授权对象建立共享。
- 实现 RBAC 角色模型（平台管理员、租户管理员、项目管理员、运维人员、发布者、只读用户）及审批策略绑定。
- 实现 SecretReference 凭据服务，确保运行凭据仅由平台凭据服务或目标侧工作负载身份按最小权限解析。
- Portal 增加租户管理、角色管理、审批策略配置界面，以及 namespace 管理视图。

**Non-Goals:**
- 不自研外部 IdP 或 LDAP 对接；Enterprise 档位可通过 Provider 对接外部 IdP。
- 不实现细粒度属性级权限（ABAC）；RBAC 覆盖 MVP 需求。
- 不修改现有数据库表结构以添加租户列（除新表外）；现有表通过中间件在查询时注入租户过滤。
- 不实现跨租户资源共享的完整工作流；仅支持显式授权对象。
- 不实现 K8s Namespace Controller 本身；仅定义 namespace 数据模型和 API，controller 作为后续变更。

## Decisions

### Decision 1: Tenant → Project → Environment → Namespace 层级

租户隔离采用四级层级：`Tenant → Project → Environment → Namespace`。

- **Project** 属于一个 Tenant，是部署单元的逻辑分组。
- **Environment** 属于一个 Project，表示部署生命周期阶段（production / staging / development）。
- **Namespace** 属于一个 Environment，直接映射到 Kubernetes Namespace 资源。一个 Environment 可包含多个 Namespace（例如 production 环境的 api、worker、cache 各一个独立 namespace）。

Kubernetes Namespace 名称自动生成，格式为 `{tenant_id}-{project_id}-{env_type}[-{suffix}]`，全局唯一。

### Decision 2: 内核平面 Tenant Context Middleware

所有 API 请求经过 Tenant Context Middleware，从 JWT 或 API Key 中提取租户上下文，注入请求上下文（Go context）。Middleware 在路由匹配前执行，确保所有下游处理函数可访问租户信息。

TenantContext 包含 tenant_id、project_id、environment_id、namespace_id（可选）、actor_id、correlation_id、roles。

备选方案是将租户解析放在每个服务中，但会导致重复代码和遗漏风险。Middleware 确保统一处理，新服务自动获得租户隔离。

### Decision 3: 数据库层租户过滤

Middleware 不自动修改 SQL 查询。每个 Repository 层在查询时从 context 读取 tenant_id，并显式添加到 WHERE 子句。对于跨租户共享资源（如全局规则），使用 tenant_scope 字段区分。

所有层级表（projects、environments、namespaces）都冗余存储 tenant_id 以支持高效过滤，避免多层 JOIN。

备选方案是使用 PostgreSQL RLS（Row-Level Security），但 RLS 在不同数据库版本和连接池模式下行为不一致，且难以调试。应用层显式过滤更可控。

### Decision 4: RBAC 使用 Casbin 或等效策略引擎

RBAC 策略存储在 PostgreSQL 中，使用 Casbin 或等效策略引擎进行角色-权限匹配。支持角色继承（平台管理员 > 租户管理员 > 项目管理员 > 运维人员 > 发布者 > 只读用户）。

授权支持三种作用域：租户级、项目级、namespace 级。

审批策略绑定到 Operation 类型，高风险操作（如数据库切换、生产部署）需要 PendingApproval 状态。

### Decision 5: Namespace 数据驱动

Namespace 以数据库记录为唯一真相来源。平台 Portal 和 API 负责 CRUD。K8s Namespace Controller（后续实现）可监听 DB 变更并自动在集群中创建/删除 Namespace。

### Decision 6: SecretReference 服务

SecretReference 是平台凭据服务管理的引用。格式为 `secret://tenant/{tenant_id}/{secret_name}`。运行时不解析 Secret，仅传递引用。Provider 在执行时通过工作负载身份或平台凭据服务获取实际凭据。

Secret 存储使用 AES-256-GCM 加密，密钥由平台密钥管理服务管理。支持自动轮换和版本管理。

## Data Model

```text
Tenant
- id, name, display_name, status, created_at, updated_at

Project
- id, tenant_id, name, created_at, updated_at
- UNIQUE (tenant_id, name)

Environment
- id, tenant_id, project_id, name, type (production/staging/development)
- UNIQUE (project_id, name)

Namespace
- id, tenant_id, project_id, environment_id, name, description, status, labels, created_at, updated_at
- UNIQUE (tenant_id, name)

Role
- id, tenant_id, name, permissions (JSONB)

UserRole
- user_id, tenant_id, project_id (nullable), namespace_id (nullable), role_id, granted_by, granted_at

ApprovalPolicy
- id, tenant_id, operation_type, required_roles, max_pending_duration

SecretReference
- id, tenant_id, name, secret_ref, version, rotation_policy, expires_at
```

## Namespace Naming Convention

Kubernetes Namespace 名称由平台自动生成，格式：

```
{tenant_id}-{project_id}-{env_type}[-{suffix}]
```

- **tenant_id**: 租户标识符，小写
- **project_id**: 项目标识符，小写
- **env_type**: 环境类型（production / staging / development）
- **suffix**: 可选后缀；当 Environment 包含多个 Namespace 时使用（如 "api"、"worker"、"cache"）

全名须符合 DNS-1123 标签规范（max 63 字符，小写字母数字 + 连字符）。

示例：
- `t1-myapp-production`（单 namespace 的 production 环境）
- `t1-myapp-production-api`（多 namespace 的 production 环境）
- `t1-myapp-staging`（staging 环境）

## API and Event Contracts

```text
GET    /tenants                          # List tenants (platform admin)
POST   /tenants                          # Create tenant
GET    /tenants/{id}                     # Tenant detail
PUT    /tenants/{id}                     # Update tenant
DELETE /tenants/{id}                     # Delete tenant (soft)

GET    /tenants/{id}/projects            # List projects
POST   /tenants/{id}/projects            # Create project

GET    /projects/{id}/environments       # List environments
POST   /projects/{id}/environments       # Create environment

GET    /environments/{id}/namespaces     # List namespaces
POST   /environments/{id}/namespaces     # Create namespace
GET    /namespaces/{id}                  # Namespace detail
PUT    /namespaces/{id}                  # Update namespace (labels, status)
DELETE /namespaces/{id}                  # Delete namespace (soft)

GET    /tenants/{id}/roles               # List roles
POST   /tenants/{id}/roles               # Create role
PUT    /tenants/{id}/roles/{roleId}      # Update role permissions

GET    /tenants/{id}/users               # List users with roles
POST   /tenants/{id}/users/{userId}:grant  # Grant role
POST   /tenants/{id}/users/{userId}:revoke # Revoke role

GET    /approval-policies                # List approval policies
POST   /approval-policies                # Create approval policy

GET    /secrets                          # List SecretReferences
POST   /secrets                          # Create SecretReference
POST   /secrets/{id}:rotate              # Rotate secret
```

## Security and Isolation

- **租户隔离:** Middleware 注入 tenant_id，Repository 层显式过滤，跨租户请求默认拒绝。
- **Namespace 隔离:** Namespace 直接映射 K8s Namespace，各 Namespace 之间网络和资源隔离由 K8s NetworkPolicy 和 ResourceQuota 保证。
- **角色:** RBAC 策略存储在 PostgreSQL，支持角色继承和审批绑定，作用域支持 tenant/project/namespace 三级。
- **Secret:** SecretReference 使用 AES-256-GCM 加密，仅凭据服务有解密权限。
- **审计:** 所有租户管理、角色变更、Secret 访问和跨租户拒绝记录审计。
- **权限:** 区分租户管理、项目管理、环境管理、Namespace 管理、角色管理、Secret 管理、审批策略管理权限。

## Performance and Capacity

- Tenant Context Middleware 延迟 < 1ms（仅上下文提取，无数据库查询）。
- Authorization Service 使用缓存策略（本地缓存 + 5 分钟 TTL），减少数据库查询。
- SecretReference 解析使用 Redis 缓存（如果可用），否则使用本地缓存。
- 容量模型：支持 10000 租户、100000 项目、1000000 Namespace、100000 用户、1000 角色。

## Migration Plan

1. 创建 Tenant Context Middleware 和 Authorization Service 基础框架。
2. 创建数据库迁移（租户、项目、环境、Namespace、角色、审批策略、SecretReference 表）。
3. 实现 Tenant Management API 和 RBAC API，包含 Namespace CRUD。
4. 实现 SecretReference 凭据服务。
5. 实现 Portal 租户管理、角色管理、审批策略配置界面，增加 Namespace 管理视图。
6. 集成 Tenant Context 到现有 API（Alert、Operation、Market、Provider）。
7. 执行全链路租户隔离测试和安全审计。

回滚时先禁用 Tenant Context Middleware，API 降级为单租户模式，保留数据和审计。

## Open Questions

- Minimal 档位的 RBAC 模型是否足够，还是需要简化角色？
- Enterprise 外部 IdP 对接的 Provider SPI 是否需要在本 change 中定义 Stub？
- SecretReference 的自动轮换策略的最小间隔是多少？
- K8s Namespace Controller 的实现时机（本 change 还是后续变更）？
