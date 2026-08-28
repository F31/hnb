# HNB Web Console 微内核插件化架构设计 V3.5

> 基于 V3.0 架构设计和阶段性页面开发实践，统一菜单、路由、插件、权限与缓存模型：菜单由数据库集中管理，经后端 API 按用户、租户、空间和权限动态生成；前端取消菜单 API 与静态 Manifest fallback 双模式，保留插件 Bundle 的本地/远程交付能力。同步完善插件生命周期、动态路由、缓存失效、租户切换隔离及扩展自动注册机制，2026-07-28。

---

## 1. 设计原则

1. **微内核** — Shell 仅保留认证、上下文、导航、路由、权限、插件运行时和基础布局，业务能力由插件提供。
2. **导航单一来源** — 数据库是菜单和导航配置的权威来源，前端仅通过 `/api/v1/navigation/menus` 获取最终导航树，不再从 Manifest 回退生成菜单。
3. **服务端权限裁剪** — 菜单 API 根据用户、租户、空间、角色、插件状态、License 和 Feature Flag 生成最小可见导航；业务 API 继续执行独立授权。
4. **配置驱动扩展** — 新增、排序、隐藏、分组和租户授权可通过插件注册与数据库配置完成，原则上不修改 Shell 代码。
5. **插件交付与菜单来源解耦** — 插件 Bundle 可采用 Local Bundle 或受控 Remote Bundle，但两种交付方式共用同一个 Plugin Registry 和 Navigation API。
6. **按需部署** — 每个插件可独立安装、启用、禁用和卸载；最小部署仅包含 Shell、Dashboard 和必要后端服务。
7. **前后端统一注册** — Backend Extension 安装时，事务性注册后端 API、前端插件、路由资源、菜单资源、权限资源和能力声明。
8. **零耦合** — 插件之间禁止直接引用，通过 Shell EventBus、公共服务契约和统一 API Client 通信。
9. **拒绝默认放行** — 菜单、路由和业务操作在权限、插件或上下文未就绪时默认拒绝，禁止因加载异常显示未授权能力。
10. **缓存可验证、可失效** — 缓存必须绑定用户、租户、空间、权限版本、插件目录版本和导航版本，并支持权限变更主动失效。
11. **产品与制品分离** — 应用市场管理产品、版本、依赖和生命周期；Harbor 仅保存不可变 OCI 制品。
12. **应用与服务分治** — 单体/微服务应用按 Helm Release 或 GitOps 管理；数据库和中间件按 Operator + CR 管理。
13. **控制器与实例分离** — Operator 是控制器，服务实例是其持续调谐的数据库或中间件工作负载。
14. **版本可追溯** — 发布版本绑定 OCI Digest、签名、SBOM 和兼容矩阵，生产环境禁止漂移标签。
15. **租户与仓库解耦映射** — 平台租户通过映射关系关联一个或多个 Harbor Project，不将租户表直接绑定单一仓库名称。
16. **生命周期保护** — 插件、租户、服务实例和 Harbor Project 的停用、升级、删除均需检查依赖、引用、审计和回滚条件。

---

## 2. 总体架构

```text
┌──────────────────────────────────────────────────────────────────────────┐
│                         HNB Web Console                                  │
│                                                                          │
│  ┌──────────────────────── Shell Kernel ──────────────────────────────┐  │
│  │ Auth │ Layout │ Context │ Navigation │ Router │ Plugin │ EventBus │  │
│  │ Mgr  │ Mgr    │ Mgr     │ Manager    │ Mgr    │ Mgr    │          │  │
│  └───────────────────────────────────┬─────────────────────────────────┘  │
│                                      │                                    │
│                         GET /api/v1/navigation/menus                       │
│                                      │                                    │
│  ┌───────────────────────────────────▼─────────────────────────────────┐  │
│  │                     HNB Backend Control Plane                       │  │
│  │ IAM │ Navigation Service │ Plugin Registry │ Capability │ License  │  │
│  │ API Server │ Extension Manager │ Marketplace │ Operation Worker    │  │
│  └───────────────┬───────────────────────┬─────────────────────────────┘  │
│                  │                       │                                │
│        ┌─────────▼─────────┐   ┌─────────▼─────────┐                      │
│        │ PostgreSQL/MySQL  │   │ Redis（可选缓存） │                      │
│        │ Plugin/Menu/RBAC  │   │ Navigation Cache │                      │
│        └───────────────────┘   └───────────────────┘                      │
│                                                                          │
│  ┌────────────────────────── Plugin Layer ─────────────────────────────┐  │
│  │ Dashboard │ Application │ Container │ Resource │ Service │ AI │System│  │
│  │ Local Bundle 或受控 Remote Bundle；均由 Plugin Registry 解析入口    │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
```

核心数据流：

```text
Extension 安装/平台初始化
→ Plugin Registry 注册插件版本与 Bundle
→ Navigation Registry 注册菜单、路由和权限资源
→ 管理员配置租户可用插件、菜单排序和功能开关
→ 用户登录并选择 tenant/space
→ Navigation Service 服务端计算可见导航树
→ Redis 命中或数据库查询
→ Web Console 注册可信路由并按需加载插件组件
```

### 2.1 导航、插件与权限控制面

```text
Plugin Registry                 Navigation Registry                 IAM/RBAC
───────────────                 ───────────────────                 ────────
插件 ID/版本                     菜单树/排序/图标                    用户/角色
Bundle 来源与 Digest             routeName/path                     租户/空间
组件导出映射                     pluginId/componentKey              权限集合
状态/兼容矩阵                    permissionCode                     权限版本
       │                                  │                              │
       └──────────────────────────────────┼──────────────────────────────┘
                                          ▼
                                Navigation Policy Engine
                                          ▼
                                /api/v1/navigation/menus
```

职责边界：

- **Plugin Registry** 决定插件是否存在、版本、状态、Bundle 来源、兼容性和组件导出。
- **Navigation Registry** 决定导航如何展示及路由如何映射，但不保存任意可执行 JavaScript 表达式。
- **IAM/RBAC** 决定当前用户在当前租户和空间拥有的权限。
- **Navigation Service** 在服务端完成联合过滤、排序、树构建、版本生成和缓存。
- **Web Console** 不再次做权威权限计算，只执行展示防御、路由守卫和插件加载。
- **业务 API** 必须重新校验权限，菜单隐藏不构成安全边界。

### 2.2 应用市场、Harbor 与运行时控制平面

```text
┌──────────────────────────── HNB 应用市场 ────────────────────────────┐
│ 产品目录 │ 分类检索 │ 版本/依赖 │ 参数 Schema │ 权限审批 │ 发布状态 │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ ProductVersion 绑定 OCI Digest
                                ▼
┌──────────────────────────── Harbor OCI Registry ─────────────────────┐
│ 容器镜像 │ Helm Chart │ Operator/CRD Chart │ 前端插件 │ SBOM/签名 │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ 拉取与校验
               ┌────────────────┴────────────────┐
               ▼                                 ▼
┌─────────────────────────┐          ┌──────────────────────────────┐
│ Application Installer   │          │ Cloud Service Installer      │
│ Helm Release / GitOps   │          │ Operator Manager + Adapter   │
└────────────┬────────────┘          └──────────────┬───────────────┘
             │                                      │
             ▼                                      ▼
   单体/微服务应用实例                    CRD + Operator + Custom Resource
                                                    │
                                                    ▼
                                        数据库/中间件服务实例
```

关键边界：

- **应用市场不是 Harbor 的 UI 替代品**：市场维护产品语义、业务分类、租户可见性、审批、计量和安装参数。
- **Harbor 不是产品数据库**：Harbor 保存 OCI 制品及其版本、Digest、签名、扫描结果和复制状态。
- **运行实例不存入 Harbor**：数据库数据、PVC 内容、运行状态和租户密钥保存在 Kubernetes、持久化存储及密钥系统中。
- **同一产品可引用多个制品**：一个云原生服务产品通常同时引用 CRD Chart、Operator Chart、实例 Chart、控制器镜像、引擎镜像和前端插件。

### 2.3 租户、应用市场与 Harbor Project 联动

```text
┌──────────────────────────── HNB Tenant/IAM ──────────────────────────┐
│ Tenant │ Organization │ Space │ Role │ Quota │ Subscription │ Audit │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ 策略计算与映射
                                ▼
┌────────────────────── Tenant Artifact Governance ────────────────────┐
│ Project Mapper │ Access Broker │ Quota Manager │ Robot Manager       │
│ Secret Sync    │ Reference Guard │ Lifecycle Controller              │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ 管理 Harbor API / 凭据 / 授权
                                ▼
┌──────────────────────────── Harbor Registry ─────────────────────────┐
│ 平台共享 Project                 │ 租户私有 Project                   │
│ hnb-platform / operators /       │ tenant-t001 / tenant-t001-prod    │
│ cloud-services / marketplace     │ tenant-t002 / tenant-t002-prod    │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ Digest 引用与拉取授权
                                ▼
               Kubernetes 集群 / CI Pipeline / 应用安装器
```

核心关系不是严格的一对一，而是：

```text
Tenant 1 ───── N TenantArtifactProjectBinding N ───── 1 HarborProject
```

其中：

- 一个租户可以拥有一个默认私有 Project，并按合规要求增加 `nonprod`、`prod` 或环境级 Project。
- 多个租户可通过授权关系只读访问同一个平台共享 Project。
- 一个 Harbor Project 具有明确的所有者域、信任域、配额、保留策略和凭据策略。
- 平台的 Space、Project、Environment 不默认等同于 Harbor Project；它们通过制品策略解析目标 Project，避免仓库数量失控。
- 平台租户显示名称可修改，但 Harbor Project 名称使用稳定、不可变的 `tenantId` 派生。

---

## 3. Shell 微内核职责

Shell 是唯一常驻前端核心，保持简单并避免承载服务端业务规则。

| 模块 | 职责 | 不负责 |
|---|---|---|
| **Auth Manager** | 登录、登出、Token 刷新、会话恢复 | 不向插件暴露原始 Refresh Token |
| **Layout Manager** | 顶部导航、侧边栏、内容区布局 | 不组装或裁剪菜单权限 |
| **Context Manager** | tenantId、spaceId、environmentId、clusterId 切换 | 不允许仅修改单字段造成跨租户残留 |
| **Navigation Manager** | 调用菜单 API、保存导航版本、处理刷新和降级 | 不读取 Manifest 生成菜单 |
| **Router Manager** | 注册/卸载动态路由、认证和权限守卫 | 不把数据库中的任意字符串直接执行为组件 |
| **Plugin Manager** | 插件发现、加载、激活、停用和故障隔离 | 不负责菜单查询和权限过滤 |
| **Permission Store** | 保存服务端返回的权限集合，提供前端快速判断 | 不替代后端授权 |
| **Capability Manager** | 查询后端能力和插件运行条件 | 不决定导航最终可见性 |
| **Event Bus** | 跨插件事件通信 | 禁止插件直接互相 import |

推荐目录：

```text
shell/
├── core/
│   ├── auth/
│   ├── context/
│   ├── navigation/
│   ├── permission/
│   ├── plugin/
│   ├── capability/
│   ├── event-bus/
│   └── router/
├── layout/
├── stores/
│   ├── authStore.ts
│   ├── contextStore.ts
│   ├── navigationStore.ts
│   ├── pluginStore.ts
│   └── permissionStore.ts
├── App.vue
├── main.ts
└── vite.config.ts
```

前端不再保留以下逻辑：

```text
/api/v1/menus 请求失败 → 从 plugin-manifest.json 生成静态菜单
```

API 不可用时只能：

1. 使用与当前用户、租户、空间和版本完全匹配的最后成功缓存；或
2. 展示最小安全导航和“导航服务暂不可用”状态。

禁止回退为包含全部插件能力的静态菜单。

---

## 4. API 驱动的动态导航与插件加载

### 4.1 菜单唯一加载模式

```text
Web Console
→ GET /api/v1/navigation/menus
→ Navigation Service
→ Redis Cache（可选）
→ Database + IAM + Plugin Registry + License/Feature Policy
→ 返回权限过滤后的导航树
```

前端 `NavigationManager.loadMenus()` 只保留一种主路径：

```typescript
async function loadMenus(context: NavigationContext): Promise<NavigationResponse> {
  const response = await navigationApi.getMenus(context, {
    ifNoneMatch: navigationStore.etag,
  })

  if (response.status === 304) {
    return navigationStore.current
  }

  navigationStore.replace(response.data)
  return response.data
}
```

### 4.2 插件 Bundle 交付方式

菜单来源单一，不等于插件 Bundle 只能有一种交付方式。V3.5 保留：

| 方式 | 说明 | 推荐场景 |
|---|---|---|
| **Local Bundle** | 插件代码随前端镜像或离线扩展包交付 | 默认生产模式、内网、离线、国产化环境 |
| **Remote Bundle** | 按 Plugin Registry 中已签名且列入白名单的入口加载 | 第三方行业插件、跨团队独立发布、高级扩展场景 |

两种 Bundle 方式必须共用以下规则：

- 插件是否启用由数据库和 Plugin Registry 决定。
- 菜单和权限由 Navigation API 决定。
- Route 只引用 `pluginId + componentKey`，不直接接受任意远程脚本地址。
- Remote Bundle 必须校验域名白名单、Digest/签名、平台兼容版本和超时策略。

### 4.3 初始化流程

```text
1. Auth Manager 恢复会话
2. Context Manager 确认 tenantId/spaceId
3. 获取 Session Context：权限版本、插件目录版本、Feature Flag
4. Navigation Manager 请求 /api/v1/navigation/menus
5. Plugin Manager 根据返回结果解析必需插件
6. Plugin Loader 加载 Local/Remote Bundle
7. Router Manager 以 routeName + componentKey 注册路由
8. Plugin Manager 调用 onActivate
9. Layout Manager 渲染菜单
```

菜单 API 返回成功但插件 Bundle 加载失败时：

- 仅该插件对应导航项进入 `unavailable` 状态或被临时隐藏；
- 核心 Shell 与其他插件继续工作；
- 上报 `plugin:error`，禁止整站白屏；
- 不应重新请求静态 Manifest 菜单作为替代。

### 4.4 插件生命周期

加载和激活必须分离：

```text
discovered → loading → loaded → activating → active
                                      │
                                      └→ error
active → deactivating → inactive → unloaded
```

最小接口：

```typescript
interface PluginInstance {
  onActivate?(context: PluginRuntimeContext): Promise<void> | void
  onDeactivate?(context: PluginRuntimeContext): Promise<void> | void
  onContextChange?(context: HNBContext): Promise<void> | void
}
```

约束：

- `loadLocalPlugin()` 和 `loadRemotePlugin()` 只完成模块解析和实例创建。
- `PluginManager.activate()` 统一调用 `onActivate()`，并保证幂等、超时和错误隔离。
- `onDeactivate()` 必须释放事件订阅、定时器、WebSocket、Store watch、路由和临时资源。
- 租户切换、插件禁用、插件升级和用户退出均需触发停用流程。

### 4.5 动态路由安全映射

数据库可以保存：

```text
routeName
path
pluginId
componentKey
permissionCode
meta
```

数据库不得保存或返回：

```text
任意 import() 表达式
未校验的 JavaScript URL
可执行脚本片段
任意组件源码路径
```

前端通过 Plugin Registry 建立可信映射：

```typescript
const component = pluginRegistry.resolveComponent(
  route.pluginId,
  route.componentKey,
)
```

路由异常分别进入：

```text
未登录         → /login?redirect=...
未选择租户     → /tenant-select
权限不足       → /403
插件未启用     → /plugin-unavailable
插件加载失败   → /plugin-error
路由不存在     → /404
```

---

## 5. Plugin Registry、Navigation Registry 与数据模型

### 5.1 Plugin Registry

插件注册表保存插件运行所需的稳定元数据：

```sql
CREATE TABLE frontend_plugins (
    id                     VARCHAR(64) PRIMARY KEY,
    name                   VARCHAR(128) NOT NULL,
    display_name           VARCHAR(128) NOT NULL,
    version                VARCHAR(64) NOT NULL,
    tier                   VARCHAR(8) NOT NULL,
    bundle_mode            VARCHAR(16) NOT NULL,   -- LOCAL / REMOTE
    bundle_ref             VARCHAR(512) NOT NULL,
    bundle_digest          VARCHAR(128),
    entry_key              VARCHAR(128),
    status                 VARCHAR(32) NOT NULL,   -- INSTALLED / ENABLED / DISABLED / ERROR
    platform_version_range VARCHAR(128),
    enabled_by_default     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMP NOT NULL,
    updated_at             TIMESTAMP NOT NULL
);
```

### 5.2 菜单资源

```sql
CREATE TABLE navigation_menus (
    id                    VARCHAR(64) PRIMARY KEY,
    parent_id             VARCHAR(64),
    plugin_id             VARCHAR(64),
    code                  VARCHAR(128) NOT NULL UNIQUE,
    title                 VARCHAR(128) NOT NULL,
    title_key             VARCHAR(128),
    icon                  VARCHAR(64),
    route_name            VARCHAR(128),
    path                  VARCHAR(256),
    permission_code       VARCHAR(128),
    menu_type             VARCHAR(16) NOT NULL,  -- GROUP / MENU / LINK
    target                VARCHAR(16) DEFAULT 'SELF',
    order_num             INT NOT NULL DEFAULT 0,
    visible               BOOLEAN NOT NULL DEFAULT TRUE,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    metadata_json         JSON,
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL,
    FOREIGN KEY (parent_id) REFERENCES navigation_menus(id),
    FOREIGN KEY (plugin_id) REFERENCES frontend_plugins(id)
);
```

### 5.3 路由资源

```sql
CREATE TABLE navigation_routes (
    id                    VARCHAR(64) PRIMARY KEY,
    plugin_id             VARCHAR(64) NOT NULL,
    route_name            VARCHAR(128) NOT NULL UNIQUE,
    path                  VARCHAR(256) NOT NULL,
    component_key         VARCHAR(128) NOT NULL,
    permission_code       VARCHAR(128),
    redirect_path         VARCHAR(256),
    keep_alive            BOOLEAN NOT NULL DEFAULT FALSE,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    metadata_json         JSON,
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL,
    FOREIGN KEY (plugin_id) REFERENCES frontend_plugins(id)
);
```

菜单引用 `route_name`，而不是复制可执行组件信息。Route 再通过 `plugin_id + component_key` 找到插件导出的可信组件。

### 5.4 插件、租户与权限关系

建议至少包含：

```text
frontend_plugins
navigation_menus
navigation_routes
permission_resources
tenant_plugin_bindings
role_permission_bindings
feature_flags
license_entitlements
navigation_versions
```

```sql
CREATE TABLE tenant_plugin_bindings (
    id          VARCHAR(64) PRIMARY KEY,
    tenant_id   VARCHAR(64) NOT NULL,
    plugin_id   VARCHAR(64) NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    config_json JSON,
    updated_at  TIMESTAMP NOT NULL,
    UNIQUE (tenant_id, plugin_id)
);
```

### 5.5 数据一致性约束

注册和发布前必须校验：

- menu `parent_id` 存在且无循环引用；
- menu `route_name` 指向有效 Route；
- Route `plugin_id` 指向已安装插件；
- `component_key` 存在于插件导出清单；
- menu code、route name 和 path 不冲突；
- 权限代码存在于 Permission Registry；
- 菜单层级建议不超过三层，数据模型可支持更深层级；
- 核心路由和核心菜单命名空间不得被第三方插件覆盖。

### 5.6 扩展注册事务

```text
Extension 安装
→ 校验签名/Digest/兼容性
→ 注册 frontend_plugins
→ 注册 navigation_routes
→ 注册 navigation_menus
→ 注册 permission_resources
→ 注册 backend APIs / capabilities
→ 建立租户默认启用策略
→ bump pluginCatalogVersion/navigationVersion
→ 清理相关缓存
→ COMMIT
```

任何步骤失败应回滚本次注册，避免出现“菜单已显示但组件不存在”或“路由存在但权限资源缺失”。

---

## 6. Context、缓存与刷新机制

### 6.1 核心上下文

```typescript
interface HNBContext {
  tenantId: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
}
```

导航计算至少依赖：

```text
userId + tenantId + spaceId
+ permissionVersion
+ pluginCatalogVersion
+ navigationVersion
+ licenseVersion
+ locale
```

### 6.2 API 设计

```http
GET /api/v1/navigation/menus?tenantId=t001&spaceId=s001&locale=zh-CN
If-None-Match: "nav-t001-u001-v42"
```

响应：

```json
{
  "apiVersion": "navigation.hnb.io/v1",
  "etag": "nav-t001-u001-v42",
  "generatedAt": "2026-07-28T20:00:00+09:00",
  "context": {
    "tenantId": "t001",
    "spaceId": "s001"
  },
  "versions": {
    "permission": "p18",
    "pluginCatalog": "pc12",
    "navigation": "n42"
  },
  "plugins": [
    {
      "id": "container",
      "version": "1.2.0",
      "bundleMode": "LOCAL",
      "bundleRef": "/modules/container/index.js",
      "bundleDigest": "sha256:..."
    }
  ],
  "menus": [
    {
      "id": "container.workloads",
      "title": "工作负载",
      "titleKey": "menu.container.workloads",
      "icon": "workload",
      "routeName": "container.workloads",
      "path": "/container/workloads",
      "pluginId": "container",
      "order": 10,
      "children": []
    }
  ],
  "routes": [
    {
      "name": "container.workloads",
      "path": "/container/workloads",
      "pluginId": "container",
      "componentKey": "Workloads",
      "permission": "container:workload:view"
    }
  ]
}
```

没有变化时返回 `304 Not Modified`。

### 6.3 服务端缓存

推荐 Redis Key：

```text
hnb:navigation:{userId}:{tenantId}:{spaceId}:{permissionVersion}:{pluginCatalogVersion}:{navigationVersion}:{locale}
```

建议：

- TTL 5～30 分钟作为兜底，而不是唯一失效机制；
- 角色授权、插件启停、License、Feature Flag、菜单配置变更后主动删除相关 Key；
- 大规模租户可使用版本号失效，避免扫描删除全部用户缓存；
- 缓存值不得跨租户复用；
- 缓存命中和生成耗时纳入可观测性。

### 6.4 浏览器缓存

前端只保存最后成功的导航快照和 ETag。缓存键至少包含：

```text
userId / tenantId / spaceId / locale / appBuildVersion
```

本地缓存仅用于：

- 页面刷新时减少闪烁；
- 短暂后端故障时展示与当前身份完全匹配的旧导航；
- 携带 ETag 发起条件请求。

不得用于绕过服务端权限重新计算。

### 6.5 租户切换原子流程

```text
1. 锁定新导航和业务请求
2. Abort 旧租户未完成请求
3. deactivate 旧租户插件
4. 注销旧动态路由
5. 清空旧菜单、权限和业务 Store
6. 设置新 tenantId/spaceId
7. 加载新 Session Context
8. 请求新 Navigation API
9. 加载并激活新插件
10. 注册新路由并恢复导航
```

必须防止租户 A 的迟到响应覆盖租户 B 状态，可使用 `AbortController + contextGeneration`。

---

## 7. 插件划分

### 7.1 Dashboard Plugin（T0）

```
首页
├── 平台总览
├── 审批待办
└── 最近操作
```

### 7.2 Application Plugin（T1）

```
应用工厂
├── 应用管理
│   ├── 单体应用
│   └── 微服务应用
├── 环境管理
├── 统一应用市场
│   ├── 单体应用产品
│   ├── 微服务应用产品
│   ├── 云原生服务产品
│   ├── 已订阅/已安装
│   └── 产品版本与升级
├── 应用模板
└── 可观测性（内置能力）
    ├── 应用分析（APM：调用链 + 日志 + 异常事件）
    ├── 全链路拓扑（服务依赖关系图）
    ├── 智能守护（监控策略 + 告警规则）
    └── 时空回溯（历史状态回放）
```

统一应用市场负责产品发现和交付入口，但不同产品类型采用不同的安装器：

- 单体应用 → `Application Installer` → 一个 Helm Release 为主。
- 微服务应用 → `Application Installer` → 组合 Chart/GitOps Application 管理多个组件。
- 云原生服务 → `Cloud Service Installer` → 确保 Operator 就绪后创建服务 CR。

安装完成后的日常管理入口不同：业务应用进入“应用管理”，数据库和中间件进入“云原生服务”。

### 7.3 Container Plugin（T1）

```
容器
├── 集群实例
│   ├── 工作负载
│   ├── 命名空间
│   ├── 存储资源
│   ├── 访问管理
│   ├── 配置管理
│   ├── 日志查询（内置能力）
│   └── 事件查询（内置能力）
└── 容器安全
    ├── 安全总览
    ├── 安全防护
    ├── 安全报告
    └── 安全配置
```

### 7.4 Resource Plugin（T1）

```
资源
├── 容器集群（管理）
├── 节点管理
├── GPU资源
├── 网络（CNI 提供商 + 策略）
├── 存储（StorageClass/PV/PVC）
├── Agent 管理
├── GSLB
└── 集群监控（内置能力）
```

### 7.5 Service Plugin（T1）+ Sub-plugins（动态加载）

```
云原生服务
├── 服务目录
├── 服务实例
│   ├── 数据服务
│   │   ├── MySQL（sub-plugin，按需加载）
│   │   ├── PostgreSQL（sub-plugin，按需加载）
│   │   └── Redis（sub-plugin，按需加载）
│   ├── 消息服务
│   │   ├── Kafka（sub-plugin，按需加载）
│   │   └── RabbitMQ（sub-plugin，按需加载）
│   ├── 微服务治理
│   └── 网关服务
│       ├── API Gateway
│       └── AI Gateway
├── 备份与恢复
├── 升级与扩缩容
└── Operator 管理（平台管理员）
    ├── CRD/控制器状态
    ├── 兼容矩阵
    ├── 管理范围
    └── 升级与回滚
```

术语约束：

- **Operator 实例**：运行在 Kubernetes 中的控制器 Deployment/Pod，不是数据库本身。
- **服务实例**：由 Custom Resource 声明、由 Operator 创建和持续调谐的具体数据库或中间件集群。
- **一个 Operator 管理多个服务实例**：默认按集群或管理域安装一次，不随每个服务实例重复安装。
- **服务进程运行在容器内，数据保存在 PVC/外部持久化存储**：Harbor 只保存软件制品，不保存数据库实际数据。

每个 sub-plugin 提供安装注册描述（安装时写入 Plugin Registry 与 Navigation Registry）：
```json
{
  "name": "mysql",
  "parent": "service",
  "displayName": "MySQL",
  "enabledByDefault": false,
  "capabilities": {
    "required": ["database-service"]
  },
  "dependencies": {
    "backend": ["mysql-provider"]
  }
}
```

### 7.6 AI Plugin（T2，可选）

```
AI
├── 模型仓库
├── 推理服务
├── Agent
└── 向量数据库
```

### 7.7 System Plugin（T1）

```
系统
├── 系统设置
├── 用户管理
├── 角色管理
├── 租户管理
├── 组织管理
├── 操作审批
├── 操作审计
├── 扩展管理
└── 制品仓库管理
    ├── Harbor 实例
    ├── Project 映射
    ├── Robot Account
    ├── 配额与保留策略
    └── 制品引用与清理
```


## 8. 统一应用市场与 Harbor 制品分层

### 8.1 核心模型：产品、制品、安装实例三层分离

| 层级 | 核心对象 | 负责内容 | 不负责内容 |
|---|---|---|---|
| **产品层** | Product / ProductVersion | 名称、分类、说明、版本、依赖、兼容矩阵、参数 Schema、租户可见性、审批与计量 | 不直接保存大文件 |
| **制品层** | OCI Artifact | 镜像、Helm Chart、Operator/CRD Chart、插件 Bundle、SBOM、签名 | 不表达租户订阅和运行状态 |
| **实例层** | ApplicationInstance / ServiceInstance | 集群、命名空间、配置、状态、端点、凭据引用、升级记录 | 不作为制品发布到 Harbor |

应用市场数据库是产品目录的权威索引，Harbor 是制品内容的权威来源。每个 `ProductVersion` 必须把制品引用固定到 Digest：

```yaml
artifacts:
  - role: deploy-chart
    ref: oci://harbor.hnb.local/hnb-applications/order-system
    version: 2.3.0
    digest: sha256:...
```

标签用于人类识别，Digest 用于安装、审计和回滚。生产环境不得仅引用 `latest`、`stable` 等可漂移标签。

### 8.2 三类产品的本质区别

| 对比维度 | 单体应用 | 微服务应用 | 云原生服务 |
|---|---|---|---|
| 产品类型 | `MONOLITH_APP` | `MICROSERVICE_APP` | `CLOUD_NATIVE_SERVICE` |
| 业务定位 | 租户业务工作负载 | 由多个服务组成的业务系统 | 被多个应用复用的平台能力 |
| 主要安装对象 | Deployment/StatefulSet + Service | 多个 Deployment/StatefulSet、Service、Ingress 等 | CRD、Operator 和服务 Custom Resource |
| 运行控制器 | Kubernetes 原生控制器 | Kubernetes 原生控制器/GitOps 控制器 | 领域 Operator + Kubernetes 原生控制器 |
| 安装实例关系 | 通常一个产品安装对应一个 Helm Release | 一个组合 Release 或一个 GitOps Application 管理多个组件 | 一个 Operator 管理多个服务实例 |
| 生命周期 | 部署、升级、回滚、扩缩容 | 编排部署、依赖顺序、灰度、整体升级 | 初始化、主从/选举、备份、恢复、故障切换、版本升级 |
| 数据处理 | 可有 PVC，通常由应用自行负责 | 各组件自行声明存储 | 强调持久化、备份恢复和删除保护 |
| 平台入口 | 应用管理 | 应用管理 | 云原生服务/服务实例 |
| 典型产品 | Java 单体、WordPress | 电商系统、CRM 微服务套件 | PostgreSQL、Redis、Kafka、RabbitMQ |

判断标准不是“是否运行在 Kubernetes”，而是：

> 是否向多个业务应用提供可复用的基础服务，并由专用控制器持续执行领域级生命周期管理。

因此，一个采用 Helm 部署到 Kubernetes 的普通业务系统仍然是应用；只有引入 Operator 也不自动等于云原生服务，必须同时满足服务化交付、独立服务实例模型和领域生命周期管理。

### 8.3 单体应用制品包

推荐制品结构：

```text
Monolith Application ProductVersion
├── product-manifest.yaml              # 市场元数据，可镜像为 OCI 通用制品
├── application-chart.tgz              # 主部署 Chart
├── application-image                  # 单一主镜像，或少量辅助镜像
├── values.schema.json                 # 参数校验与表单生成
├── default-values.yaml
├── sbom.spdx.json
└── signature / provenance
```

安装语义：

```text
选择产品版本 → 参数校验 → 拉取并验签 → Helm install/upgrade
→ 创建 ApplicationInstance → 注册访问地址和可观测性
```

管理原则：

- 一个安装实例通常对应一个 Helm Release。
- 应用镜像与 Chart 分别版本化，但 ProductVersion 固定两者的 Digest 组合。
- JAR/WAR 等传统包不直接作为 Kubernetes 运行单元时，应先由构建流水线生成镜像；确需保存原始包时，将其作为 `source-binary` OCI 通用制品管理。

### 8.4 微服务应用制品包

推荐制品结构：

```text
Microservice Application ProductVersion
├── product-manifest.yaml
├── umbrella-chart.tgz / gitops-bundle
├── charts/
│   ├── gateway-chart
│   ├── order-chart
│   ├── payment-chart
│   └── user-chart
├── images.lock.yaml                   # 所有组件镜像 Digest 锁定
├── topology.yaml                      # 服务依赖与拓扑
├── rollout-plan.yaml                  # 安装顺序、健康门禁、灰度策略
├── values.schema.json
├── observability-profile.yaml
├── sbom/
└── signatures/
```

管理原则：

- 产品版本必须形成“组件版本集合”，不能只记录一个模糊的应用版本。
- 推荐使用 Umbrella Chart 管理中小规模套件；复杂系统可由 Argo CD/Flux 的 Application/ApplicationSet 管理，但市场仍只暴露一个产品安装入口。
- 组件可以独立构建，但生产发布必须生成不可变的 `images.lock.yaml`，保证重装和回滚得到同一组镜像。
- 数据库、Redis、Kafka 等依赖不应打包成业务应用的内嵌副本，优先声明为 `serviceDependencies`，由平台绑定已有云原生服务实例或按策略新建。

### 8.5 云原生服务制品包

云原生服务是组合制品，不建议将全部内容塞进一个 Helm Chart：

```text
Cloud Native Service ProductVersion
├── product-manifest.yaml
├── crd-chart.tgz                      # 可独立升级的 CRD
├── operator-chart.tgz                 # Controller/RBAC/Webhook
├── instance-chart.tgz                 # 生成 Custom Resource 的实例模板
├── operator-image
├── engine-images/                     # PostgreSQL/Redis/Kafka 等
├── provider-adapter-image             # HNB 后端适配器，可选
├── frontend-sub-plugin                # HNB Service 子插件，可选
├── backup-restore-profile.yaml
├── compatibility-matrix.yaml
├── values.schema.json
├── sbom/
└── signatures/
```

必须明确三种安装对象：

1. **CRD 包**：集群级 API 定义，生命周期独立于普通 Helm Release。
2. **Operator 包**：每集群或管理域安装一次，运行控制器 Pod。
3. **服务实例包**：每次创建数据库/中间件实例时安装，主要生成 Custom Resource、监控和网络策略。

服务实例不是 Operator 实例。以下关系应在平台数据模型中保持：

```text
OperatorInstallation 1 ───── N ServiceInstance
ServiceInstance 1 ───── 1 CustomResource
CustomResource 1 ───── N Workload/PVC/Service
```

### 8.6 Harbor Project 与 Repository 规划

Harbor 的隔离、配额和成员权限以 Project 为主要边界；Repository 用于组织某个产品下的具体 OCI 制品。推荐先按信任域、所有权和生命周期划分 Project，再按产品与制品角色划分 Repository。

```text
harbor.hnb.local
├── hnb-platform/                      # 平台核心组件，仅平台发布
│   ├── frontend-shell
│   ├── apiserver
│   └── marketplace
├── hnb-operators/                     # 平台共享 Operator/CRD
│   ├── postgresql/crds
│   ├── postgresql/operator
│   ├── redis/operator
│   └── kafka/operator
├── hnb-cloud-services/                # 平台官方云原生服务
│   ├── postgresql/instance-chart
│   ├── postgresql/engine
│   ├── redis/instance-chart
│   └── kafka/instance-chart
├── hnb-marketplace/                   # 平台公共应用产品
├── hnb-plugins/                       # 前后端扩展 Bundle
├── hnb-base-images/                   # 经批准的基础镜像
├── hnb-quarantine/                    # 待扫描、待审核制品
├── tenant-t001/                       # 租户 T001 默认私有 Project
│   ├── applications/order-system/chart
│   ├── applications/order-system/images/api
│   ├── applications/crm-suite/chart
│   └── custom-operators/example
└── tenant-t002/                       # 租户 T002 默认私有 Project
```

不建议仅按“镜像仓库”和“Chart 仓库”拆分 Project，否则同一产品的权限、复制、保留和生命周期会被割裂。制品类型通过 Repository 路径、OCI Media Type 和 `ArtifactRole` 区分。

平台共享 Project 的原则：

- 由平台管理员或供应链流水线发布。
- 普通租户只读引用，不复制 Operator 和公共服务制品到各租户 Project。
- 同一 ProductVersion 在所有租户中引用相同 OCI Digest。
- 漏洞修复、下架、签名和 SBOM 由平台统一治理。

租户私有 Project 的原则：

- 保存租户自研单体应用、微服务应用、私有 Chart、镜像和经审批的自定义 Operator。
- 设置租户级存储配额、保留策略、不可变标签、扫描策略和 Robot Account。
- 租户不能覆盖 `hnb-operators`、`hnb-cloud-services` 等平台官方制品。
- 默认一个租户一个私有 Project；严格生产隔离时再拆分 `nonprod/prod`。

### 8.7 租户与 Harbor Project 映射模型

采用独立映射实体，不在租户表中直接写死一个 Harbor Project 名称：

```text
HarborInstance 1 ───── N HarborProject
Tenant 1 ───── N TenantArtifactProjectBinding
HarborProject 1 ───── N TenantArtifactProjectBinding
```

映射关系包括两种语义：

- **OWNED**：租户拥有并管理的私有 Project。
- **GRANTED**：租户被授权访问的平台共享或跨租户共享 Project。

推荐数据表：

```sql
CREATE TABLE tenant_artifact_projects (
    id                    VARCHAR(64) PRIMARY KEY,
    tenant_id             VARCHAR(64) NOT NULL,
    harbor_instance_id    VARCHAR(64) NOT NULL,
    harbor_project_id     BIGINT,
    harbor_project_name   VARCHAR(128) NOT NULL,

    relation_type         VARCHAR(16) NOT NULL,   -- OWNED / GRANTED
    project_scope         VARCHAR(32) NOT NULL,
    environment_type      VARCHAR(32),
    access_level          VARCHAR(16) NOT NULL,   -- PULL / PUSH / MANAGE
    visibility            VARCHAR(16) NOT NULL DEFAULT 'private',

    storage_quota_bytes   BIGINT,
    status                VARCHAR(32) NOT NULL,
    credential_policy_id  VARCHAR(64),
    created_at            TIMESTAMP NOT NULL,
    updated_at            TIMESTAMP NOT NULL,

    UNIQUE (harbor_instance_id, harbor_project_name, tenant_id)
);
```

`project_scope` 建议取值：

```text
PLATFORM_SHARED
TENANT_PRIVATE
TENANT_NONPROD
TENANT_PROD
ENVIRONMENT_PRIVATE
CROSS_TENANT_SHARED
```

默认映射示例：

```text
Tenant T001
├── OWNED  → tenant-t001             # 私有应用和 Chart
├── OWNED  → tenant-t001-prod        # 可选，严格生产隔离
├── GRANTED → hnb-marketplace        # 平台公共应用，只读
├── GRANTED → hnb-operators          # 平台 Operator，只读
└── GRANTED → hnb-cloud-services     # 平台云原生服务，只读
```

Project 命名规则：

```text
默认私有：tenant-{immutableTenantId}
非生产：tenant-{immutableTenantId}-nonprod
生产：tenant-{immutableTenantId}-prod
```

禁止直接使用租户中文名称、组织名称或可变简称作为 Project 主键。

### 8.8 制品可见性与目标 Project 解析

产品可见性与 Harbor Project 权限是两个相互关联但不同的控制面：

| 可见范围 | 市场语义 | 典型制品位置 |
|---|---|---|
| `PLATFORM_PUBLIC` | 所有有效租户可见 | `hnb-marketplace`、`hnb-cloud-services` |
| `PLATFORM_AUTHORIZED` | 指定套餐、许可或租户可见 | 平台共享受控 Project |
| `TENANT_PRIVATE` | 仅所属租户可见 | `tenant-{tenantId}` |
| `TENANT_SHARED` | 指定多个租户可见 | 所有者 Project + 显式授权 |
| `INTERNAL` | 仅平台运维和系统组件可见 | `hnb-platform`、`hnb-operators` |

产品查询必须同时满足：

```text
ProductVisibility 允许
AND TenantSubscription 有效
AND HarborProjectBinding 可访问
AND ProductVersion 已发布
AND 制品签名/扫描/兼容状态通过
```

发布时由 `ArtifactPlacementPolicy` 决定目标 Project：

```text
官方 Operator / CRD                 → hnb-operators
官方云原生服务实例 Chart/引擎镜像   → hnb-cloud-services
平台公共应用                         → hnb-marketplace
平台插件与核心组件                   → hnb-plugins / hnb-platform
租户自研应用                         → tenant-{tenantId}
租户生产隔离制品                     → tenant-{tenantId}-prod
待审核第三方制品                     → hnb-quarantine
```

环境不直接决定产品版本，而是解析允许部署的 Digest。开发、测试、生产晋级应移动或授权同一 Digest，不得重新构建同一版本。

### 8.9 Harbor 访问与 Robot Account

普通租户用户默认不直接登录 Harbor，而由 HNB Artifact Service 代理访问：

```text
User → HNB API/IAM → Artifact Service → Harbor API
```

优点：

- 统一平台 RBAC、审批和审计。
- 隐藏 Harbor Repository 结构和底层凭据。
- 便于执行配额、签名、扫描和产品状态门禁。
- 后续可以替换或接入多个 Harbor 实例。

CI/CD、集群运行时等机器访问使用 Project 级 Robot Account，并按用途拆分：

```text
robot$tenant-t001-ci          # Push + Pull，用于租户构建流水线
robot$tenant-t001-runtime     # Pull Only，用于 Kubernetes 拉取
robot$tenant-t001-scan        # Read/Scan，用于安全审计
robot$tenant-t001-promotion   # 源 Pull + 目标 Push，用于制品晋级
```

凭据管理原则：

- 密钥保存到 Vault、Kubernetes Secret 或企业密钥系统。
- 平台数据库仅保存 `robotAccountId`、名称、Secret 引用、过期时间和状态。
- Robot Account 应设置有效期、自动轮换和最小权限。
- Runtime 凭据按租户允许的 Namespace 同步为 `imagePullSecret`，不得全局扩散。
- 一个万能 Robot Account 不得同时承担 CI、生产拉取和平台管理。

### 8.10 租户与 Project 生命周期联动

租户创建流程：

```text
创建租户
→ 生成不可变 tenantId
→ 根据 ArtifactPolicy 创建默认私有 Harbor Project
→ 设置 Private、配额、保留、扫描和不可变标签策略
→ 创建 CI/Runtime Robot Account
→ 保存 TenantArtifactProjectBinding
→ 向授权 Namespace 同步 Pull Secret
→ 赋予平台共享 Project 的只读 GRANTED 关系
→ 状态 READY
```

建议状态机：

```text
PENDING
→ CREATING_PROJECT
→ CONFIGURING_POLICY
→ CREATING_ROBOT_ACCOUNT
→ SYNCING_SECRET
→ READY

异常：CREATE_FAILED / POLICY_FAILED / SECRET_SYNC_FAILED / ACCESS_FAILED
```

租户停用和删除采用两阶段机制：

```text
ACTIVE
→ SUSPENDED：禁止新 Push、上架和部署，保留运行实例拉取能力
→ RETENTION：检查实例、制品引用、共享授权、审计和保留期
→ ARCHIVED：导出元数据或转存制品
→ DELETED：确认无引用后删除 Project 与凭据
```

Project 删除前必须检查：

- 是否存在运行中的 `ApplicationInstance` 或 `ServiceInstance`。
- Kubernetes 工作负载是否仍引用该 Project 的镜像 Digest。
- 是否存在未完成发布、回滚或制品晋级任务。
- 是否存在跨租户共享产品或授权关系。
- 是否满足审计、备份和监管保留期限。
- 是否已导出 ProductVersion、SBOM、签名及制品清单。

租户停用期间，建议撤销 Push 权限但暂时保留 Runtime Pull 权限，防止 Pod 重建失败。禁止“删除租户即立即删除 Harbor Project”。

### 8.11 应用市场元数据模型

```yaml
apiVersion: marketplace.hnb.io/v1
kind: ProductVersion
metadata:
  productId: postgresql-ha
  version: 17.2.1
spec:
  productType: CLOUD_NATIVE_SERVICE
  category: database
  owner:
    scope: PLATFORM
  visibility:
    scope: PLATFORM_AUTHORIZED
  artifactPlacement:
    projectScope: PLATFORM_SHARED
  runtime:
    target: kubernetes
    installer: operator
  artifacts:
    - role: crd-chart
      ref: oci://harbor.hnb.local/hnb-operators/postgresql/crds
      digest: sha256:...
    - role: operator-chart
      ref: oci://harbor.hnb.local/hnb-operators/postgresql/operator
      digest: sha256:...
    - role: instance-chart
      ref: oci://harbor.hnb.local/hnb-cloud-services/postgresql/instance-chart
      digest: sha256:...
  compatibility:
    kubernetes: ">=1.30"
    storageCapabilities: [snapshot]
    architectures: [amd64, arm64]
  lifecycle:
    supportsBackup: true
    supportsRestore: true
    supportsRollingUpgrade: true
    deletionPolicyDefault: Retain
  release:
    channel: stable
    status: published
```

建议最少维护以下核心实体：

```text
Product
ProductVersion
ArtifactReference
DependencyConstraint
CompatibilityRule
InstallProfile
Subscription
Installation
OperatorInstallation
ApplicationInstance
ServiceInstance
ReleaseApproval
HarborInstance
HarborProject
TenantArtifactProjectBinding
RobotAccountBinding
ArtifactPlacementPolicy
ProductVisibilityGrant
ArtifactReferenceUsage
```

### 8.12 分类、检索与界面呈现

应用市场统一入口采用一级产品类型和二级业务分类：

```text
应用市场
├── 应用
│   ├── 单体应用
│   └── 微服务应用
└── 云原生服务
    ├── 数据库
    ├── 缓存
    ├── 消息队列
    ├── 网关
    ├── 微服务治理
    ├── 存储
    └── AI 基础服务
```

产品卡片必须显式展示：

- 产品类型和安装器类型。
- 当前稳定版本、支持架构和 Kubernetes 版本。
- 所需集群能力，如 StorageClass、VolumeSnapshot、GPU、LoadBalancer。
- 是否安装平台级 Operator，以及 Operator 的管理范围。
- 数据持久化、备份能力和删除策略。
- 安全扫描、签名、SBOM 和发布审批状态。

禁止把“PostgreSQL Operator Chart”和“PostgreSQL 高可用服务”作为两个同等级商品暴露给普通租户。前者是实现制品，后者才是可订阅产品。

### 8.13 版本、发布与晋级

版本采用四层绑定：

```text
ProductVersion
├── Chart Version
├── Application/Engine Version
├── Operator/CRD Version（仅云原生服务）
└── OCI Digest Set
```

发布流程：

```text
构建 → 单元测试 → Chart lint/template → 策略检查 → 镜像漏洞扫描
→ 生成 SBOM → 制品签名 → 推送 quarantine → 集成验证
→ 发布审批 → 晋级 test/prod Project → 市场上架
```

晋级必须复制或提升同一 Digest，不得在不同环境重新构建同一版本。已发布版本默认启用不可变标签；存在运行实例引用的版本只能废弃，不应直接删除。

### 8.14 安全、权限与审计

- CI/CD 使用 Project 级 Robot Account，按最小权限授予 push、pull、scan 等能力；CI、Runtime、Promotion 账号必须分离。
- 运行集群只授予 pull 权限；Operator 安装权限仅授予平台管理员或受控自动化服务账号。
- 平台共享 Project 与租户私有 Project 分别执行独立 RBAC、配额、保留和复制策略。
- 产品可见性、租户订阅与 Harbor Project 授权必须同时校验，任何单一条件均不能单独放行。
- Harbor 对容器镜像执行漏洞扫描和拉取策略；Helm Chart 还应在流水线中执行 Schema 校验、渲染、Kubernetes API 校验和策略检查。
- 生产安装前校验签名、Digest、发布状态和兼容矩阵。
- Secret 不写入 Chart、values 或市场数据库明文，使用 Kubernetes Secret、External Secrets 或企业密钥管理系统引用。
- 所有上架、撤回、安装、升级、回滚和删除操作写入统一审计日志。

---

## 9. Operator 云原生服务生命周期机制

### 9.1 组件职责

```text
Marketplace Service
  └── 产品与版本、订阅、审批、制品引用

Cloud Service Installer
  └── 预检、Operator 确保、实例创建、回滚编排

Operator Manager
  └── CRD/Operator 安装、范围、健康、兼容和升级

Service Adapter
  └── HNB 统一模型与第三方 Operator CRD 双向转换

Third-party Operator
  └── 领域级 Reconcile、故障恢复、备份和升级
```

HNB 核心服务不得直接耦合 CloudNativePG、Strimzi、Redis Operator 等私有 CRD。统一 `ServiceInstance` 模型通过 Adapter 转换为具体 CR，并将底层状态归一为 HNB 状态。

### 9.2 安装流程

```text
1. 用户在应用市场选择云原生服务产品
2. Marketplace 校验订阅、配额、审批和产品状态
3. Installer 校验 Kubernetes、架构、存储、快照、网络等能力
4. Operator Manager 查询目标集群是否已安装兼容 Operator
5. 未安装：按 CRD → Operator 顺序安装；已安装：复用或执行受控升级
6. 创建 ServiceInstance 记录并生成目标 Operator 的 Custom Resource
7. Operator 调谐并创建 Pod/StatefulSet、Service、PVC、Secret 等
8. Adapter 汇总状态、连接端点、监控、备份能力和事件
9. 实例 Ready 后向用户返回连接信息的 Secret 引用
```

### 9.3 状态模型

```text
PENDING_APPROVAL
→ PREFLIGHTING
→ PREPARING_OPERATOR
→ CREATING
→ INITIALIZING
→ RUNNING
→ DEGRADED / FAILED
→ UPGRADING / SCALING / BACKING_UP / RESTORING
→ DELETING
→ DELETED / RETAINED
```

平台状态来源包括：Custom Resource `status.conditions`、工作负载状态、PVC/快照状态、Operator 事件以及 HNB 安装任务状态。不得只以 Pod Running 判断数据库可用。

### 9.4 升级顺序

云原生服务升级需区分四种升级：

1. **CRD 升级**：先兼容验证，必要时启用 Conversion Webhook 和存量 CR 迁移。
2. **Operator 升级**：按兼容矩阵升级控制器，确认其可同时管理存量 CR 版本。
3. **实例 Chart 升级**：升级 HNB 参数模板、监控和策略资源。
4. **数据库/中间件引擎升级**：由 Operator 按领域规则执行滚动升级、数据迁移或主从切换。

默认顺序：

```text
备份/快照 → CRD → Operator → Adapter/实例模板 → 服务实例引擎
```

Operator 升级和服务实例升级必须解耦，禁止因某个租户升级一个数据库实例而重复安装或替换整个集群的 Operator。

### 9.5 删除与数据保护

服务删除策略至少提供：

- `Delete`：删除 CR 及关联工作负载，PVC 按策略处理。
- `RetainData`：删除计算资源，保留 PVC/快照和恢复元数据。
- `BackupThenDelete`：完成可验证备份后再删除。
- `Orphan`：解除平台管理但保留底层资源，仅限管理员。

生产数据库默认 `RetainData` 或 `BackupThenDelete`。删除前执行依赖检查、审批和 Finalizer 保护，禁止简单地把 `helm uninstall` 等同于数据库安全删除。

### 9.6 Operator 管理范围

支持三种模式：

| 模式 | 说明 | 适用场景 |
|---|---|---|
| Cluster Scope | 一个 Operator 管理全集群多个 Namespace | 标准平台、资源效率最高 |
| Namespace Scope | Operator 仅管理指定 Namespace | 强隔离、第三方 Operator 权限受限 |
| Management Domain | 一个 Operator 管理一组租户/Namespace | 大规模平台，在效率与隔离间平衡 |

默认采用 Cluster Scope 或 Management Domain。只有 Operator 本身不支持安全多租户、版本冲突明显或合规要求严格时，才为租户独立部署控制器。


---

## 10. 权限与导航策略

### 10.1 服务端权限评估流程

```text
用户身份
→ 当前 tenantId / spaceId / environmentId
→ 角色与直接授权
→ 租户插件启用关系
→ License / Subscription
→ Feature Flag / Capability
→ 菜单 permissionCode
→ Route permissionCode
→ 生成最终 NavigationResponse
```

V3.5 不再由 Shell 遍历全部 Manifest 并自行计算最终菜单。Shell 仅消费服务端结果，并保留前端防御性判断。

### 10.2 三层授权边界

| 层次 | 作用 | 是否安全边界 |
|---|---|---|
| 菜单过滤 | 控制入口是否展示 | 否 |
| 路由守卫 | 阻止无权限页面加载，改善体验 | 否 |
| 后端 API 授权 | 决定读取或变更业务资源是否允许 | **是** |

即使菜单和路由未暴露，所有业务 API 仍必须校验 `tenantId + resource scope + permission`。

### 10.3 权限资源示例

```text
container:view
container:workload:view
container:workload:create
container:workload:update
container:workload:delete
plugin:view
plugin:manage
navigation:view
navigation:manage
navigation:publish
```

高风险导航配置和插件启停应支持审批和审计。

### 10.4 权限变化传播

```text
角色/权限变更
→ IAM bump permissionVersion
→ 发布 permission:updated 事件
→ Navigation Cache 版本失效
→ Web Console 重新获取菜单
→ 当前路由重新校验
→ 无权限时进入 /403 或安全首页
```

WebSocket/SSE 可作为实时通知优化；没有实时通道时，在 Token 刷新、租户切换和关键操作后重新获取版本。

### 10.5 平台角色与 Harbor 权限映射

| HNB 角色/主体 | Harbor 访问方式 | 建议权限 |
|---|---|---|
| 平台超级管理员 | HNB 后端服务账号 | Harbor 系统管理，仅限受控服务调用 |
| 平台制品管理员 | HNB Console 代理 | 共享 Project 管理、发布、扫描和复制 |
| 租户管理员 | HNB Console 代理 | 管理本租户产品、配额和凭据策略 |
| 租户开发者 | HNB Console 或 CI Robot | 租户私有 Project Push/Pull |
| 租户运维人员 | HNB Console | Pull、部署、扫描结果查看 |
| 普通租户用户 | HNB Console | 不直接访问 Harbor |
| Kubernetes 集群 | Runtime Robot | 仅 Pull 指定 Project |
| CI/CD Pipeline | CI Robot | 指定 Project Push/Pull |
| Promotion Worker | Promotion Robot | 源 Project Pull、目标 Project Push |

平台角色不直接等于 Harbor 内部角色。Artifact Service 应把平台权限转换为受控 API 操作，避免用户绕过产品审批、签名和审计流程。

### 10.6 多层制品授权判定

```text
用户身份与租户角色
→ 当前 tenantId/spaceId/environmentId
→ 产品可见性与订阅
→ TenantArtifactProjectBinding
→ 操作权限（pull/push/manage/promote）
→ 制品状态、签名、扫描与环境策略
→ Harbor 执行
```

权限建议：

```text
artifact:project:view
artifact:project:manage
artifact:repository:view
artifact:push
artifact:pull
artifact:promote
artifact:delete
artifact:robot:manage
artifact:quota:manage
marketplace:product:publish
marketplace:product:share
```

### 10.7 租户隔离边界

- 租户 A 不得通过菜单 API、导航缓存、Repository 路径或 Harbor API 看到租户 B 的私有资源。
- Navigation Service、Artifact Service 和业务 API 的所有查询都必须以认证上下文中的 tenantId 为准，不信任前端任意传入的租户 ID。
- Space 和 Environment 只能缩小访问范围，不能扩大租户权限。
- 平台级插件和共享制品由租户只读使用，租户不得覆盖同名核心能力。
- 跨租户共享必须通过显式授权关系建立，并支持撤销和审计。

---

## 11. 前后端扩展与自动注册

### 11.1 Extension Manifest

Extension Manifest 用于安装阶段注册扩展能力，不作为浏览器菜单的运行时来源。

```yaml
apiVersion: extension.hnb.io/v1
kind: Extension
metadata:
  name: postgresql-service-extension
  version: 3.5.0
spec:
  extensionType: cloud-service-provider

  backend:
    services:
      - name: postgresql-provider-adapter
        image:
          ref: harbor.hnb.local/hnb-plugins/postgresql-provider@sha256:...
    apiRoutes:
      - /api/v1/cloud-services/postgresql

  frontend:
    plugin:
      id: postgresql-service
      displayName: PostgreSQL
      tier: T1
      bundleMode: LOCAL
      bundle:
        ref: harbor.hnb.local/hnb-plugins/postgresql-ui@sha256:...
      exports:
        - key: InstanceList
        - key: InstanceDetail
        - key: CreateInstance

  registration:
    permissions:
      - database:postgresql:view
      - database:postgresql:create
      - database:postgresql:manage
    routes:
      - name: service.postgresql.instances
        path: /service/postgresql/instances
        componentKey: InstanceList
        permission: database:postgresql:view
    menus:
      - code: service.postgresql
        parentCode: service.data
        title: PostgreSQL
        routeName: service.postgresql.instances
        permission: database:postgresql:view
        order: 20

  marketplace:
    products:
      - productId: postgresql-ha
        productType: CLOUD_NATIVE_SERVICE
        version: 17.2.1

  operator:
    scope: cluster
    crdChart:
      ref: oci://harbor.hnb.local/hnb-operators/postgresql/crds@sha256:...
    controllerChart:
      ref: oci://harbor.hnb.local/hnb-operators/postgresql/operator@sha256:...
    instanceChart:
      ref: oci://harbor.hnb.local/hnb-cloud-services/postgresql/instance-chart@sha256:...

  capabilities:
    provides: [database-service.postgresql, backup, restore]
    requires: [kubernetes, persistent-volume]

  compatibility:
    platform: ">=3.5"
    kubernetes: ">=1.30"
    architectures: [amd64, arm64]

  enabledByDefault: false
```

### 11.2 云原生服务扩展安装流程

```text
1. 校验 Extension 签名、Digest、平台版本和集群能力
2. 部署后端 Provider Adapter
3. 注册后端 API 和 Capability
4. 写入 Plugin Registry
5. 校验插件导出组件清单
6. 写入 Permission Registry、Route Registry 和 Menu Registry
7. 注册 Marketplace ProductVersion
8. Operator Manager 安装或关联 CRD + Operator
9. bump pluginCatalogVersion/navigationVersion
10. 清理导航缓存并发布 navigation:updated
```

步骤 4～6 必须位于同一数据库事务或可补偿工作流中。

### 11.3 插件禁用与卸载

禁用插件：

```text
标记 tenant_plugin_bindings.enabled=false
→ bump navigationVersion
→ 失效相关缓存
→ 前端重新拉取导航
→ Router Manager 卸载路由
→ Plugin Manager 调用 onDeactivate
```

卸载插件前必须检查：

- 是否仍有菜单、路由或权限绑定；
- 是否存在运行中的应用或服务实例；
- 是否被其他插件依赖；
- 是否存在未完成任务或升级；
- 是否需要保留审计和配置数据。

### 11.4 能力发现

Capability Manager 综合判断：

- Extension 是否启用且健康；
- 所需 OCI 制品是否可拉取并通过签名验证；
- 目标集群是否安装兼容 CRD/Operator；
- 目标集群是否具备 StorageClass、Snapshot、GPU、LoadBalancer 等能力；
- 当前租户是否具有订阅、配额和操作权限；
- 当前租户是否拥有或获授权访问制品所在 Harbor Project；
- Runtime Pull Secret 和 Robot Account 是否有效；
- ProductVersion Digest 是否仍受保护且未被撤销。

Capability 影响服务端导航策略，但不由前端自行决定最终菜单。

---

## 12. 模块通信

### 12.1 规则

- 插件禁止 import 其他插件；
- 插件禁止访问其他插件内部状态；
- 跨插件通信通过 Shell EventBus 或后端事件；
- 导航变化由后端版本和事件驱动，插件不得直接向 Layout Manager 注入永久菜单。

### 12.2 EventBus 事件表

```typescript
'context:changing'       // 上下文切换开始
'context:changed'        // 上下文切换完成
'auth:logged-in'
'auth:logged-out'
'permission:updated'
'navigation:updated'     // 服务端导航版本变化
'navigation:reload'
'plugin:loaded'
'plugin:activated'
'plugin:deactivated'
'plugin:error'
'application:created'
'application:deleted'
'operation:submitted'
'operation:approved'
'operation:completed'
'cluster:added'
'cluster:removed'
'alert:fired'
'tenant:artifact-ready'
'artifact:project-bound'
'artifact:promoted'
'artifact:access-revoked'
```

### 12.3 导航刷新示例

```typescript
eventBus.on('navigation:updated', async ({ version }) => {
  if (version !== navigationStore.version) {
    await navigationManager.reload()
    await routerManager.reconcile(navigationStore.routes)
  }
})
```

---

## 13. 部署方案

### 13.1 部署档位

| 档位 | 组件 | 场景 |
|---|---|---|
| 最小 | Shell + Dashboard + IAM + Navigation Service + 数据库 | POC、演示、轻量运维 |
| 标准 | 最小档 + Application + Container + Resource + System + Redis | 生产核心运维 |
| 完整 | 标准档 + Service + AI + Marketplace + Harbor + Operator Manager | 企业级全功能 |

### 13.2 Helm Chart 集成

```yaml
frontend:
  enabled: true
  shell:
    replicaCount: 2
    image:
      repository: hnb/frontend-shell
    navigation:
      endpoint: /api/v1/navigation/menus
      requestTimeout: 5s
      allowLastKnownGoodCache: true
      staticManifestFallback: false

pluginRegistry:
  enabled: true
  defaultBundleMode: local
  remoteBundles:
    enabled: false
    requireSignature: true
    allowedDomains: []

navigationService:
  enabled: true
  database:
    type: postgresql
  cache:
    enabled: true
    type: redis
    ttl: 15m
  etag:
    enabled: true
  invalidation:
    publishEvents: true

extensions:
  mysql-extension:
    enabled: false
  kafka-extension:
    enabled: false
```

### 13.3 Harbor、应用市场与租户 Project 配置

```yaml
marketplace:
  enabled: true
  catalogDatabase:
    type: postgresql
  artifactRegistry:
    type: harbor
    endpoint: https://harbor.hnb.local
    adminCredentialSecretRef: hnb-harbor-admin
    requireDigest: true
    verifySignature: true
    enforceScanPolicy: true

artifactGovernance:
  enabled: true
  accessMode: proxy-first
  tenantProjects:
    enabled: true
    defaultPattern: "tenant-{tenantId}"
    defaultScope: TENANT_PRIVATE
    defaultQuota: 100Gi
    splitProductionProject: false
  robotAccounts:
    createCI: true
    createRuntime: true
    rotateBeforeExpiryDays: 15
  secretSync:
    enabled: true
  deletionProtection:
    checkRuntimeReferences: true
    retentionDays: 30
    archiveBeforeDelete: true

operatorManager:
  enabled: true
  installPolicy: admin-controlled
  defaultScope: cluster
```

### 13.4 轻量化建议

- 小规模环境可不部署 Redis，Navigation Service 使用进程内短缓存和 ETag，但数据库仍是唯一来源。
- 标准和多副本环境建议使用 Redis，避免各实例缓存不一致。
- Local Bundle 仍可全部打入一个前端镜像，动态性来自数据库注册和租户策略，而不是必须使用远程微前端。
- Remote Bundle 默认关闭，仅在经过签名、来源白名单和兼容验证后启用。
- 离线环境通过扩展安装包导入 Bundle 和注册描述，安装器写入数据库。
- 禁止通过 ConfigMap 中的静态菜单清单覆盖数据库导航。

### 13.5 可用性与降级

```text
Navigation API 正常      → 返回实时/缓存导航
Redis 不可用             → 回源数据库并告警
数据库短暂不可用         → 返回服务端最后可用缓存（如存在）
前端请求失败             → 使用严格匹配的 Last Known Good 快照
无可信快照               → 最小安全页面，不展示业务插件菜单
```

---

## 14. TypeScript 类型定义

### 14.1 插件运行时接口

```typescript
interface HNBPluginModule {
  id: string
  version: string
  create(context: PluginRuntimeContext): Promise<PluginInstance> | PluginInstance
}

interface PluginInstance {
  onActivate?(context: PluginRuntimeContext): Promise<void> | void
  onDeactivate?(context: PluginRuntimeContext): Promise<void> | void
  onContextChange?(context: HNBContext): Promise<void> | void
}

interface PluginRuntimeContext {
  pluginId: string
  auth: ReadonlyAuthContext
  context: Readonly<HNBContext>
  permission: PermissionReader
  eventBus: EventBus
  apiClient: ScopedApiClient
  router: ScopedPluginRouter
  logger: PluginLogger
  abortSignal: AbortSignal
}
```

插件不接收原始 Refresh Token，也不能直接修改全局 Router 和导航 Store。

### 14.2 Plugin Registry 类型

```typescript
interface PluginDescriptor {
  id: string
  version: string
  displayName: string
  tier: 'T0' | 'T1' | 'T2'
  bundleMode: 'LOCAL' | 'REMOTE'
  bundleRef: string
  bundleDigest?: `sha256:${string}`
  entryKey?: string
  status: 'INSTALLED' | 'ENABLED' | 'DISABLED' | 'ERROR'
  exports: string[]
  platformVersionRange?: string
}
```

### 14.3 导航与路由类型

```typescript
interface NavigationResponse {
  apiVersion: 'navigation.hnb.io/v1'
  etag: string
  generatedAt: string
  context: {
    tenantId: string
    spaceId?: string
  }
  versions: {
    permission: string
    pluginCatalog: string
    navigation: string
  }
  plugins: PluginDescriptor[]
  menus: MenuItem[]
  routes: DynamicRoute[]
}

interface MenuItem {
  id: string
  code: string
  title: string
  titleKey?: string
  icon?: string
  routeName?: string
  path?: string
  pluginId?: string
  order: number
  target?: 'SELF' | 'BLANK'
  children: MenuItem[]
}

interface DynamicRoute {
  name: string
  path: string
  pluginId: string
  componentKey: string
  permission?: string
  redirect?: string
  meta?: {
    title?: string
    keepAlive?: boolean
    activeMenuCode?: string
  }
}
```

层级关系以 API 返回的 `children` 为前端唯一表达；数据库内部使用 `parent_id`，避免前端同时维护 `parentId` 和 `children` 两套权威关系。

### 14.4 Store 类型

```typescript
interface NavigationStore {
  readonly etag?: string
  readonly version?: string
  readonly menus: MenuItem[]
  readonly routes: DynamicRoute[]
  readonly plugins: PluginDescriptor[]
  readonly status: 'idle' | 'loading' | 'ready' | 'degraded' | 'error'

  replace(response: NavigationResponse): void
  clear(): void
}

interface PermissionStore {
  readonly version?: string
  readonly permissions: ReadonlySet<string>
  has(permission?: string): boolean
  hasAll(permissions: string[]): boolean
  clear(): void
}
```

### 14.5 Context 类型

```typescript
interface HNBContext {
  tenantId: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
}

interface ContextManager {
  readonly current: Readonly<HNBContext>
  switchContext(next: HNBContext): Promise<void>
  reset(): Promise<void>
}
```

### 14.6 应用市场与制品类型

```typescript
type ProductType =
  | 'MONOLITH_APP'
  | 'MICROSERVICE_APP'
  | 'CLOUD_NATIVE_SERVICE'

type InstallerType = 'helm' | 'gitops' | 'operator'

type ArtifactRole =
  | 'deploy-chart'
  | 'umbrella-chart'
  | 'crd-chart'
  | 'operator-chart'
  | 'instance-chart'
  | 'container-image'
  | 'frontend-plugin'
  | 'backend-adapter'
  | 'product-manifest'
  | 'sbom'
  | 'signature'

interface ArtifactReference {
  role: ArtifactRole
  ref: string
  version?: string
  digest: `sha256:${string}`
  mediaType?: string
  architectures?: string[]
}

interface ProductVersion {
  productId: string
  version: string
  productType: ProductType
  category: string
  installer: InstallerType
  artifacts: ArtifactReference[]
  dependencies: ProductDependency[]
  compatibility: CompatibilityRule
  installSchemaRef?: string
  releaseStatus: 'draft' | 'testing' | 'published' | 'deprecated' | 'revoked'
}

interface OperatorInstallation {
  id: string
  clusterId: string
  operatorName: string
  scope: 'cluster' | 'namespace' | 'management-domain'
  namespace: string
  crdVersion: string
  controllerVersion: string
  status: 'installing' | 'ready' | 'degraded' | 'upgrading' | 'failed'
}

interface ServiceInstance {
  id: string
  productVersionId: string
  operatorInstallationId: string
  clusterId: string
  namespace: string
  customResourceRef: KubernetesObjectRef
  phase: string
  endpointSecretRef?: KubernetesObjectRef
  deletionPolicy: 'Delete' | 'RetainData' | 'BackupThenDelete' | 'Orphan'
}
```


### 14.7 租户制品治理类型

```typescript
type ProjectScope =
  | 'PLATFORM_SHARED'
  | 'TENANT_PRIVATE'
  | 'TENANT_NONPROD'
  | 'TENANT_PROD'
  | 'ENVIRONMENT_PRIVATE'
  | 'CROSS_TENANT_SHARED'

type ProjectRelation = 'OWNED' | 'GRANTED'
type ArtifactAccessLevel = 'PULL' | 'PUSH' | 'MANAGE'

type ProductVisibilityScope =
  | 'PLATFORM_PUBLIC'
  | 'PLATFORM_AUTHORIZED'
  | 'TENANT_PRIVATE'
  | 'TENANT_SHARED'
  | 'INTERNAL'

interface HarborInstance {
  id: string
  name: string
  endpoint: string
  status: 'ready' | 'degraded' | 'offline'
  credentialSecretRef: string
}

interface HarborProject {
  id: string
  harborInstanceId: string
  projectName: string
  scope: ProjectScope
  ownerTenantId?: string
  isPublic: boolean
  quotaBytes?: number
  status: 'creating' | 'ready' | 'suspended' | 'archived' | 'failed'
}

interface TenantArtifactProjectBinding {
  id: string
  tenantId: string
  harborProjectId: string
  relation: ProjectRelation
  accessLevel: ArtifactAccessLevel
  environmentType?: 'nonprod' | 'prod'
  status: 'pending' | 'ready' | 'suspended' | 'revoked' | 'failed'
}

interface RobotAccountBinding {
  id: string
  tenantId?: string
  harborProjectId: string
  purpose: 'ci' | 'runtime' | 'scan' | 'promotion'
  secretRef: string
  expiresAt?: string
  status: 'active' | 'rotating' | 'expired' | 'revoked'
}

interface ProductVisibility {
  scope: ProductVisibilityScope
  ownerTenantId?: string
  allowedTenantIds?: string[]
  requiredSubscriptions?: string[]
}

interface ArtifactReferenceUsage {
  artifactDigest: `sha256:${string}`
  harborProjectId: string
  referencedBy: Array<{
    resourceType: 'ProductVersion' | 'ApplicationInstance' | 'ServiceInstance'
    resourceId: string
  }>
  deletionProtected: boolean
}
```

---

## 15. 目录结构

```text
web/
├── shell/
│   ├── core/
│   │   ├── auth/
│   │   ├── context/
│   │   ├── navigation/
│   │   │   ├── NavigationManager.ts
│   │   │   ├── NavigationCache.ts
│   │   │   └── index.ts
│   │   ├── permission/
│   │   ├── plugin/
│   │   │   ├── PluginManager.ts
│   │   │   ├── PluginLoader.ts
│   │   │   ├── LocalBundleResolver.ts
│   │   │   ├── RemoteBundleResolver.ts
│   │   │   └── PluginRegistry.ts
│   │   ├── capability/
│   │   ├── event-bus/
│   │   └── router/
│   │       ├── RouterManager.ts
│   │       ├── RouteReconciler.ts
│   │       └── index.ts
│   ├── layout/
│   │   ├── AppHeader.vue
│   │   ├── AppSidebar.vue
│   │   ├── RecursiveMenu.vue
│   │   └── AppLayout.vue
│   ├── stores/
│   │   ├── authStore.ts
│   │   ├── contextStore.ts
│   │   ├── navigationStore.ts
│   │   ├── permissionStore.ts
│   │   └── pluginStore.ts
│   ├── App.vue
│   ├── main.ts
│   ├── vite.config.ts
│   └── Dockerfile
│
├── plugins/
│   ├── dashboard/
│   ├── application/
│   ├── container/
│   ├── resource/
│   ├── service/
│   │   └── sub-plugins/
│   │       ├── mysql/
│   │       ├── postgresql/
│   │       ├── redis/
│   │       ├── kafka/
│   │       └── rabbitmq/
│   ├── ai/
│   └── system/
│
├── packages/
│   ├── types/
│   ├── api-client/
│   ├── plugin-sdk/
│   └── ui-kit/
│
├── scripts/
│   ├── build-all.sh
│   ├── validate-plugin-exports.mjs
│   └── dev.sh
└── pnpm-workspace.yaml
```

不再在 Shell 中维护 `public/config/plugin-manifest.json` 作为菜单来源。Local Bundle 的入口可在构建产物索引或 Plugin Registry 中登记。

### 15.1 后端新增模块建议

```text
backend/
├── navigation/
│   ├── api/                         # /api/v1/navigation/menus
│   ├── application/                 # 导航计算与树构建
│   ├── domain/                      # Menu/Route/NavigationPolicy
│   ├── repository/                  # 数据库访问
│   ├── cache/                       # Redis/本地缓存
│   ├── version/                     # ETag 与版本失效
│   └── events/                      # navigation:updated
├── plugin-registry/
│   ├── catalog/
│   ├── registration/
│   ├── compatibility/
│   ├── bundle-verifier/
│   └── lifecycle/
├── iam/
│   ├── permission/
│   ├── role/
│   └── policy/
├── marketplace/
├── artifact-registry/
├── application-installer/
├── cloud-service/
└── operation-worker/
```

### 15.2 前端模块职责约束

```text
NavigationManager：只请求、缓存和刷新 NavigationResponse
PluginLoader：只解析 Bundle 和创建实例
PluginManager：负责 activate/deactivate 和状态管理
RouterManager：只根据可信映射注册、卸载和守卫路由
RecursiveMenu：纯展示组件，不访问权限 API
PermissionStore：保存服务端权限结果并提供快速判断
```

---

## 16. V3.5 的关键增强

| 项目 | V3.0 | V3.5 |
|---|---|---|
| **菜单来源** | 插件 Manifest 生成菜单，并考虑 API/fallback | 数据库为唯一权威来源，统一通过 Navigation API 返回 |
| **双模式边界** | 插件加载和菜单来源概念混合 | 仅 Bundle 交付保留 Local/Remote；菜单不再双模式 |
| **权限过滤** | Shell 遍历 Manifest 过滤 | 服务端按用户、租户、空间、插件、License 联合裁剪 |
| **前端职责** | Plugin Loader 同时涉及菜单、路由和插件 | Navigation、Plugin、Router 三个管理器职责拆分 |
| **动态扩展** | 新插件依赖 Manifest 和构建配置 | Extension 安装自动写入 Plugin/Menu/Route/Permission Registry |
| **缓存** | tenantId 粗粒度前端缓存 | 用户+租户+空间+多版本缓存键、ETag、Redis 和主动失效 |
| **生命周期** | onActivate/onDeactivate 定义但流程不完整 | 加载与激活分离，支持幂等、超时、停用和资源释放 |
| **动态路由** | 插件直接注册 RouteConfig | 数据库存 routeName/componentKey，前端通过可信导出表解析 |
| **租户切换** | context:changed 后插件重新初始化 | 原子切换、请求中止、路由卸载、Store 清理和防迟到覆盖 |
| **失败降级** | API 失败可 fallback Manifest | 仅使用严格匹配的 Last Known Good；否则最小安全页面 |
| **数据库模型** | 无统一导航资源模型 | 增加 Plugin、Menu、Route、TenantPlugin、NavigationVersion 等实体 |
| **制品与服务治理** | 已形成 Harbor/Operator 模型 | 保留 V3.0 成果并与插件注册、导航刷新联动 |

---

## 17. 落地路线图

| 阶段 | 内容 | 产出 |
|---|---|---|
| **Phase 0** | 完成 `/api/v1/navigation/menus`、Menu/Route/Plugin 表、基础权限过滤 | 数据库/API 单一菜单闭环 |
| **Phase 1** | 重构前端 NavigationManager、RouterManager、PluginManager；删除 Manifest fallback | Shell 职责精简、路由可注册和卸载 |
| **Phase 2** | 完成权限版本、插件目录版本、ETag、Redis 缓存和主动失效 | 多租户高效导航加载 |
| **Phase 3** | 完成插件生命周期、租户原子切换、错误边界和可观测性 | 生产级插件运行时 |
| **Phase 4** | Extension 自动注册插件、菜单、路由、权限和 Capability | 新功能安装后无需修改 Shell 代码 |
| **Phase 5** | Dashboard、Application、Container、Resource、System 核心插件迁移 | 核心控制台闭环 |
| **Phase 6** | Marketplace Catalog + Harbor + ProductVersion/Digest | 应用制品上架、安装、升级和回滚 |
| **Phase 7** | Tenant Project Mapper、Robot、Secret Sync 和引用保护 | 多租户制品治理闭环 |
| **Phase 8** | Operator Manager + PostgreSQL/Redis/MySQL/Kafka/RabbitMQ 扩展 | 云原生服务矩阵 |
| **Phase 9** | 签名、SBOM、漏洞门禁、离线包、多 Harbor 和规模验证 | 企业交付与国产化/离线能力 |

当前页面开发应优先完成：

```text
1. 删除 PluginLoader 中的菜单 fallback 和权限过滤
2. 新增 NavigationManager / navigationStore
3. 实现 Go GET /api/v1/navigation/menus
4. 建立最小 Menu、Route、Plugin、TenantPlugin 数据表
5. 由 API 返回 menus + routes + plugins
6. RouterManager 支持动态注册/卸载和 /403
7. PluginManager 统一调用 onActivate/onDeactivate
8. 加入 ETag 和进程内缓存；规模化后再启用 Redis
9. 实现租户切换清理和 AbortController
10. 完成端到端权限与插件故障测试
```

---

## 18. 设计决策摘要

1. HNB 菜单只采用一种运行时加载模式：通过后端 Navigation API 从数据库按权限动态生成。
2. 不再使用 `manifest.json` 作为菜单 API 失败时的 fallback，也不在浏览器中组装权威菜单。
3. Local Bundle 与 Remote Bundle 只是插件代码交付方式，不是菜单来源；二者共享同一个 Plugin Registry 和 Navigation Registry。
4. 新功能通过 Extension 安装注册插件、菜单、路由、权限和能力，完成后提升版本并失效缓存，Shell 无需修改。
5. 数据库菜单引用 `routeName`，Route 引用 `pluginId + componentKey`，禁止数据库下发任意可执行脚本或组件表达式。
6. Navigation Service 在服务端联合用户、租户、空间、角色、插件启用状态、License、Feature Flag 和 Capability 生成结果。
7. 前端菜单隐藏和路由守卫只改善体验，后端业务 API 是最终安全边界。
8. 导航缓存必须绑定身份、上下文和版本；权限或插件变化后通过版本提升和事件主动失效。
9. API 故障时只能使用身份和上下文严格匹配的最后成功快照；没有可信快照时进入最小安全页面。
10. Plugin Loader 只负责加载模块，Plugin Manager 负责生命周期，Navigation Manager 负责菜单，Router Manager 负责路由。
11. 插件加载与激活分离；onActivate/onDeactivate 需要幂等、超时、故障隔离和完整资源释放。
12. 租户切换必须原子清理旧菜单、路由、权限、请求和插件状态，防止跨租户数据泄漏。
13. 应用市场、Harbor、Operator 和租户制品治理继续沿用 V3.0 的产品、制品、实例三层分离模型。
14. HNB 通过 Plugin Registry、Navigation Service、Artifact Service 和 Service Adapter 隔离前端扩展、Harbor 与第三方 Operator 的实现细节。
15. V3.5 的核心目标是：保持前端轻量，同时让平台功能、菜单、权限和租户能力真正实现数据库配置驱动和无代码扩展。

---

## 19. 参考资料

- Kubernetes Operator Pattern：<https://kubernetes.io/docs/concepts/extend-kubernetes/operator/>
- Helm OCI Registry：<https://helm.sh/docs/topics/registries/>
- Harbor OCI/Helm Chart：<https://goharbor.io/docs/main/working-with-projects/working-with-oci/working-with-helm-oci-charts/>
- Harbor Project：<https://goharbor.io/docs/main/working-with-projects/create-projects/>
- Harbor Project Quota：<https://goharbor.io/docs/main/administration/configure-project-quotas/>
- Harbor Robot Accounts：<https://goharbor.io/docs/main/administration/robot-accounts/>
- Harbor Vulnerability Scanning：<https://goharbor.io/docs/main/administration/vulnerability-scanning/>
- Harbor User-defined OCI Artifact：<https://goharbor.io/docs/main/administration/user-defined-oci-artifact/>
