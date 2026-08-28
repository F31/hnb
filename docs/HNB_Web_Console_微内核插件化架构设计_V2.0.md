# HNB Web Console 微内核插件化架构设计 V2.0

> 基于 V1.0 架构评审反馈优化，2026-07-25

---

## 1. 设计原则

1. **微内核** — Shell 只做核心事，全部业务能力由插件提供
2. **双模式插件** — 默认 Local Bundle，可选 Remote Module Federation
3. **按需部署** — 每个插件可独立启用/禁用，最小部署仅 Shell + Dashboard
4. **权限驱动** — 菜单和路由由用户权限 + 插件声明动态生成
5. **前后端统一** — Backend Extension 安装自动注册 Frontend Plugin
6. **零耦合** — 插件之间禁止互相引用，通过 Shell EventBus 通信

---

## 2. 总体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         HNB Web Console                             │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Shell Kernel (微内核)                      │   │
│  │  ┌──────┐ ┌──────┐ ┌──────────┐ ┌──────────┐ ┌─────────┐   │   │
│  │  │Auth  │ │Layout│ │Context   │ │Permission│ │Plugin   │   │   │
│  │  │Mgr   │ │Mgr   │ │Mgr       │ │Mgr      │ │Loader   │   │   │
│  │  └──────┘ └──────┘ └──────────┘ └──────────┘ └────┬─────┘   │   │
│  │  ┌────────┐ ┌──────────┐ ┌──────────┐             │         │   │
│  │  │Cap     │ │EventBus  │ │Router    │    Plugin Registry     │   │
│  │  │Mgr     │ │          │ │Mgr       │             │         │   │
│  │  └────────┘ └──────────┘ └──────────┘             │         │   │
│  └──────────────────────────────────────────────────┼──────────┘   │
│                                                      │             │
│  ┌──────────────────────────────────────────────────┼──────────┐   │
│  │                   Plugin Layer                    │          │   │
│  │                                                   ▼          │   │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐         │   │
│  │  │Dashboard│ │Application│ │Container│ │ Resource │         │   │
│  │  │(T0)    │ │(T1)     │ │(T1)    │ │(T1)     │         │   │
│  │  └─────────┘ └─────────┘ └─────────┘ └──────────┘         │   │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐                      │   │
│  │  │Service  │ │AI       │ │System   │                      │   │
│  │  │(T1)    │ │(T2)    │ │(T1)    │                      │   │
│  │  └────┬────┘ └─────────┘ └─────────┘                      │   │
│  │       │ Sub-plugins (动态加载)                              │   │
│  │  ┌────┼─────────┬──────────┐                              │   │
│  │  │DB  │  Middleware │Gateway│  ServiceMesh                │   │
│  │  └────┴─────────┴──────────┘                              │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    HNB Backend API                           │   │
│  │  apiserver | IAM | Provider | Extension | Operation Worker   │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. Shell 微内核职责

Shell 是唯一常驻运行的核心，提供以下能力：

| 模块 | 职责 | 说明 |
|---|---|---|
| **Auth Manager** | 登录/登出/Token 刷新/会话管理 | 对接 `POST /api/v1/auth/*` |
| **Layout Manager** | 顶部导航/侧边栏/内容区布局 | 菜单由插件动态注册 |
| **Context Manager** | 全局上下文：tenantId/spaceId/environmentId/clusterId | 取代 V1.0 的 Workspace Store |
| **Permission Manager** | 用户权限校验/菜单过滤/路由守卫 | 基于 RBAC + 插件声明权限 |
| **Plugin Loader** | 插件发现/加载/卸载/生命周期管理 | 支持 Local + Remote 双模式 |
| **Capability Manager** | 后端能力检测/插件可用性判断 | 检查后端服务是否部署 |
| **Event Bus** | 跨插件通信 | 禁止插件直接互调 |
| **Router Manager** | 全局路由注册/导航守卫 | 插件动态注册子路由 |

目录结构：

```
shell/
├── core/
│   ├── auth/
│   ├── context/
│   ├── permission/
│   ├── plugin-loader/
│   ├── capability/
│   ├── event-bus/
│   └── router/
├── layout/
│   ├── AppHeader.vue
│   ├── AppSidebar.vue
│   └── AppContent.vue
├── stores/
│   ├── authStore.ts
│   ├── contextStore.ts
│   └── permissionStore.ts
├── public/
│   └── config/
│       └── plugin-manifest.json
├── App.vue
├── main.ts
└── vite.config.ts
```

---

## 4. 双模式插件加载

### 4.1 模式一：Local Bundle（默认，生产推荐）

所有插件打包在同一个容器镜像中，通过 manifest 控制启停。

```
容器目录结构：
/usr/share/nginx/html/
├── shell/
│   └── index.html
├── config/
│   └── plugin-manifest.json
└── modules/
    ├── dashboard/index.js
    ├── application/index.js
    ├── container/index.js
    ├── resource/index.js
    ├── service/index.js
    ├── ai/index.js
    └── system/index.js
```

**优点**：一个镜像、简单部署、安全合规、内网/离线/国产化环境友好。

### 4.2 模式二：Remote Bundle（高级场景，可选）

通过 Module Federation 运行时加载远程插件。

```javascript
// 适用场景：
// - AI Provider 厂商提供独立 UI 插件
// - 第三方行业插件
// - 跨团队独立发布

const plugin = await pluginLoader.loadRemote({
  name: 'ai-provider',
  entry: 'https://plugin.company.com/remoteEntry.js',
})
```

### 4.3 加载决策

```
Plugin Loader 运行时逻辑：

1. 读取 plugin-manifest.json
2. 对每个 enabled 插件：
   a. 检查 permission → 无权限则隐藏
   b. 检查 capability → 后端无能力则隐藏
   c. 检查 mode:
      - "local"  → 从 /modules/{name}/index.js 加载
      - "remote" → 从 remoteEntry 动态加载
3. 注册路由到 Router Manager
4. 注册菜单到 Layout Manager
5. 触发 plugin:loaded 事件
```

---

## 5. 插件 Manifest 增强

### 5.1 完整 Manifest 结构

```json
{
  "name": "container",
  "version": "1.0.0",
  "displayName": "容器",
  "description": "容器集群实例与安全管理",
  "tier": "T1",
  "enabled": true,
  "mode": "local",

  "icon": "container",

  "permissions": {
    "required": ["container:view"],
    "optional": ["container:manage", "container:security"]
  },

  "capabilities": {
    "required": ["kubernetes"],
    "optional": ["container-security"]
  },

  "dependencies": {
    "backend": ["kubernetes-provider"],
    "plugins": []
  },

  "lifecycle": {
    "onInstall": "",
    "onEnable": "",
    "onDisable": "",
    "onUninstall": ""
  },

  "menu": {
    "group": "infrastructure",
    "items": [
      {
        "title": "集群实例",
        "path": "/container/instances",
        "icon": "cluster",
        "permission": "container:view",
        "children": [
          { "title": "工作负载", "path": "/container/instances/workloads" },
          { "title": "命名空间", "path": "/container/instances/namespaces" },
          { "title": "存储资源", "path": "/container/instances/storage" },
          { "title": "访问管理", "path": "/container/instances/access" },
          { "title": "配置管理", "path": "/container/instances/config" },
          { "title": "日志查询", "path": "/container/instances/logs" },
          { "title": "事件查询", "path": "/container/instances/events" }
        ]
      },
      {
        "title": "容器安全",
        "path": "/container/security",
        "icon": "security",
        "permission": "container:security",
        "children": [
          { "title": "安全总览", "path": "/container/security/overview" },
          { "title": "安全防护", "path": "/container/security/protection" },
          { "title": "安全报告", "path": "/container/security/report" },
          { "title": "安全配置", "path": "/container/security/config" }
        ]
      }
    ]
  },

  "routes": [
    {
      "path": "/container",
      "component": "ContainerLayout",
      "children": [
        { "path": "instances/workloads", "component": "Workloads" },
        { "path": "instances/namespaces", "component": "Namespaces" },
        { "path": "instances/storage", "component": "Storage" },
        { "path": "instances/access", "component": "Access" },
        { "path": "instances/config", "component": "Config" },
        { "path": "instances/logs", "component": "Logs" },
        { "path": "instances/events", "component": "Events" },
        { "path": "security/overview", "component": "SecurityOverview" },
        { "path": "security/protection", "component": "SecurityProtection" },
        { "path": "security/report", "component": "SecurityReport" },
        { "path": "security/config", "component": "SecurityConfig" }
      ]
    }
  ],

  "exposes": {
    "components": {
      "WorkloadMetrics": "./components/WorkloadMetrics.vue"
    }
  }
}
```

### 5.2 后端能力声明

```json
{
  "name": "mysql-service",
  "displayName": "MySQL 数据库服务",
  "dependencies": {
    "backend": ["mysql-provider"]
  },
  "capabilities": {
    "required": ["database-service"]
  }
}
```

当后端未部署 `mysql-provider` 时，前端自动隐藏此插件。

---

## 6. Context 上下文设计

### 6.1 核心接口

```typescript
interface HNBContext {
  tenantId?: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
}

interface ContextStore {
  current: HNBContext

  // 切换空间
  setSpace(spaceId: string): Promise<void>

  // 复杂操作时设置完整上下文
  setFullContext(ctx: HNBContext): void

  // 重置
  reset(): void
}
```

### 6.2 使用原则

- **默认**：用户只选择空间，Shell 只保存 `spaceId`
- **按需加载**：`tenantId`、`environmentId`、`clusterId` 在进入具体页面时按需获取
- **生命周期**：切换空间时，所有插件重新初始化（触发 `context:changed` 事件）

### 6.3 应用场景

| 操作 | 上下文要求 |
|---|---|
| 查看仪表盘 | spaceId |
| 查看容器列表 | spaceId + clusterId |
| 部署应用到环境 | spaceId + projectId + environmentId |
| 系统管理 | tenantId |

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
├── 应用市场
├── 应用模板
└── 可观测性（内置能力）
    ├── 应用分析（APM：调用链 + 日志 + 异常事件）
    ├── 全链路拓扑（服务依赖关系图）
    ├── 智能守护（监控策略 + 告警规则）
    └── 时空回溯（历史状态回放）
```

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
├── 数据服务
│   ├── MySQL（sub-plugin，按需加载）
│   ├── PostgreSQL（sub-plugin，按需加载）
│   └── Redis（sub-plugin，按需加载）
├── 消息服务
│   ├── Kafka（sub-plugin，按需加载）
│   └── RabbitMQ（sub-plugin，按需加载）
├── 微服务治理
└── 网关服务
    ├── API Gateway
    └── AI Gateway
```

每个 sub-plugin 独立的 manifest：
```json
{
  "name": "mysql",
  "parent": "service",
  "displayName": "MySQL",
  "enabled": true,
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
└── 扩展管理
```

---

## 8. 权限模型

### 8.1 权限评估流程

```
用户登录
  → 获取用户角色 + 权限列表
  → Shell 遍历所有插件 Manifest
  → 检查插件 permissions.required 是否匹配
  → 不匹配 → 插件隐藏（菜单不显示，路由不可达）
  → 匹配 → 检查子菜单每个 item 的 permission
  → 无权限的菜单项自动隐藏
  → 最终 = 用户可见的菜单树
```

### 8.2 权限声明示例

```json
{
  "plugin": "container",
  "permissions": {
    "required": ["container:view"],
    "optional": ["container:manage", "container:security"]
  }
}
```

- `required`：不满足则整个插件不可见
- `optional`：不满足则隐藏对应菜单项

---

## 9. 前后端扩展统一

### 9.1 Extension Manifest（统一清单）

```yaml
# extension.yaml
name: mysql-extension
version: 1.0.0
tier: T1

backend:
  services:
    - name: mysql-provider
      image: hnb/mysql-provider:1.0.0
      replicaCount: 1
    - name: mysql-controller
      image: hnb/mysql-controller:1.0.0
      replicaCount: 1

frontend:
  plugin:
    name: mysql-service
    bundle: hnb/plugin-mysql-service:1.0.0
    manifest: /config/plugin-mysql-service.json

capabilities:
  provides:
    - database-service
  requires:
    - kubernetes

enabledByDefault: false
```

### 9.2 安装流程

```
安装 mysql-extension：
  1. 部署后端 mysql-provider + mysql-controller
  2. 注册 API 路由到 apiserver
  3. 注册前端插件 manifest 到 Plugin Registry
  4. 通知 Shell 重新加载插件列表
  5. mysql-service 出现在云原生服务菜单中
```

---

## 10. 模块通信

### 10.1 规则

- 插件禁止 import 其他插件
- 插件禁止访问其他插件的内部状态
- 跨插件通信走 Shell EventBus

### 10.2 EventBus 事件表

```typescript
// 全局事件
'context:changed'       // 空间/环境切换
'auth:logged-in'        // 用户登录
'auth:logged-out'       // 用户登出
'permission:updated'    // 权限变更

// 业务事件
'application:created'   // 应用创建
'application:deleted'   // 应用删除
'operation:submitted'   // 操作提交
'operation:approved'    // 操作审批通过
'operation:completed'   // 操作执行完成
'cluster:added'         // 集群接入
'cluster:removed'       // 集群移除
'alert:fired'           // 告警触发
'plugin:loaded'         // 插件加载完成
'plugin:error'          // 插件加载失败
```

### 10.3 使用示例

```typescript
// 应用工厂插件发布事件
eventBus.emit('application:created', { appId: '...', spaceId: '...' })

// 仪表盘插件监听事件，刷新数据
eventBus.on('application:created', () => {
  dashboardStore.refresh()
})
```

---

## 11. 部署方案

### 11.1 最小部署

```
Shell + Dashboard
适用场景：POC、演示、轻量运维
```

### 11.2 标准部署

```
Shell + Dashboard + Application + Container + Resource + System
适用场景：生产环境，覆盖核心运维场景
```

### 11.3 完整部署

```
Shell + Dashboard + Application + Container + Resource + Service + AI + System
适用场景：全功能企业级部署
```

### 11.4 Helm Chart 集成

```yaml
# values.yaml
frontend:
  enabled: true
  shell:
    replicaCount: 2
    image:
      repository: hnb/frontend-shell
    config:
      # 插件注册表（ConfigMap 挂载）
      pluginManifest:
        dashboard:  { enabled: true,  mode: local }
        application: { enabled: true,  mode: local }
        container:  { enabled: true,  mode: local }
        resource:   { enabled: true,  mode: local }
        service:    { enabled: true,  mode: local }
        ai:         { enabled: false, mode: remote, entry: "https://ai.plugin.hnb.cloud/remoteEntry.js" }
        system:     { enabled: true,  mode: local }

  # 扩展自动注册
  extensions:
    mysql-extension:
      enabled: false
    kafka-extension:
      enabled: false
```

### 11.5 Docker Compose 集成

```yaml
services:
  frontend:
    build:
      context: ./web
      dockerfile: shell/Dockerfile
    ports:
      - "8080:80"
    volumes:
      - ./web/plugins:/usr/share/nginx/html/modules
    configs:
      - source: plugin-manifest
        target: /usr/share/nginx/html/config/plugin-manifest.json
```

---

## 12. TypeScript 类型定义

### 12.1 插件接口

```typescript
// packages/types/src/plugin.ts

interface HNBPlugin {
  name: string
  version: string
  displayName: string
  tier: 'T0' | 'T1' | 'T2'
  enabled: boolean
  mode: 'local' | 'remote'

  create(ctx: PluginContext): Promise<PluginInstance>
}

interface PluginInstance {
  routes?: RouteConfig[]
  menuItems?: MenuItem[]
  onActivate?(): Promise<void>
  onDeactivate?(): Promise<void>
  onContextChange?(ctx: HNBContext): Promise<void>
}

interface PluginContext {
  auth: AuthStore
  context: ContextStore
  permission: PermissionStore
  eventBus: EventBus
  apiClient: ApiClient
  capability: CapabilityManager
}
```

### 12.2 Manifest 类型

```typescript
// packages/types/src/manifest.ts

interface PluginManifest {
  name: string
  version: string
  displayName: string
  description?: string
  tier: 'T0' | 'T1' | 'T2'
  enabled: boolean
  mode: 'local' | 'remote'
  icon?: string

  permissions: {
    required: string[]
    optional?: string[]
  }

  capabilities: {
    required: string[]
    optional?: string[]
  }

  dependencies: {
    backend: string[]
    plugins?: string[]
  }

  lifecycle: {
    onInstall?: string
    onEnable?: string
    onDisable?: string
    onUninstall?: string
  }

  menu: {
    group: string
    items: MenuItem[]
  }

  routes: RouteConfig[]
}

interface MenuItem {
  title: string
  path: string
  icon?: string
  permission?: string
  children?: MenuItem[]
}

interface RouteConfig {
  path: string
  component: string
  children?: RouteConfig[]
  meta?: {
    title?: string
    permission?: string
    keepAlive?: boolean
  }
}
```

### 12.3 上下文类型

```typescript
// packages/types/src/context.ts

interface HNBContext {
  tenantId?: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
}

interface ContextStore {
  readonly current: HNBContext

  setSpace(spaceId: string): Promise<void>
  setFullContext(ctx: HNBContext): void
  reset(): void
}
```

---

## 13. 目录结构

```
web/
├── shell/                              # 微内核（独立构建，Docker 镜像）
│   ├── core/
│   │   ├── auth/
│   │   │   ├── AuthManager.ts
│   │   │   ├── LoginPage.vue
│   │   │   └── index.ts
│   │   ├── context/
│   │   │   ├── ContextManager.ts
│   │   │   └── index.ts
│   │   ├── permission/
│   │   │   ├── PermissionManager.ts
│   │   │   └── index.ts
│   │   ├── plugin-loader/
│   │   │   ├── PluginLoader.ts
│   │   │   ├── LocalPluginResolver.ts
│   │   │   ├── RemotePluginResolver.ts
│   │   │   └── index.ts
│   │   ├── capability/
│   │   │   ├── CapabilityManager.ts
│   │   │   └── index.ts
│   │   ├── event-bus/
│   │   │   ├── EventBus.ts
│   │   │   └── index.ts
│   │   └── router/
│   │       ├── RouterManager.ts
│   │       └── index.ts
│   ├── layout/
│   │   ├── AppHeader.vue
│   │   ├── AppSidebar.vue
│   │   ├── AppContent.vue
│   │   └── AppLayout.vue
│   ├── stores/
│   │   ├── authStore.ts
│   │   ├── contextStore.ts
│   │   └── permissionStore.ts
│   ├── public/
│   │   └── config/
│   │       └── plugin-manifest.json
│   ├── App.vue
│   ├── main.ts
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── Dockerfile
│   └── nginx.conf
│
├── plugins/                            # 内置插件（Local Bundle）
│   ├── dashboard/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   └── pages/
│   │   │       ├── Dashboard.vue
│   │   │       ├── ApprovalList.vue
│   │   │       └── RecentOps.vue
│   │   └── vite.config.ts
│   │
│   ├── application/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   └── pages/
│   │   │       ├── AppList.vue
│   │   │       ├── EnvManager.vue
│   │   │       ├── AppMarket.vue
│   │   │       ├── AppTemplates.vue
│   │   │       └── observability/
│   │   │           ├── AppAnalysis.vue
│   │   │           ├── Topology.vue
│   │   │           ├── SmartGuard.vue
│   │   │           └── TimeTravel.vue
│   │   └── vite.config.ts
│   │
│   ├── container/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   └── pages/
│   │   │       ├── cluster-instance/
│   │   │       │   ├── Workloads.vue
│   │   │       │   ├── Namespaces.vue
│   │   │       │   ├── Storage.vue
│   │   │       │   ├── Access.vue
│   │   │       │   ├── Config.vue
│   │   │       │   ├── Logs.vue
│   │   │       │   └── Events.vue
│   │   │       └── security/
│   │   │           ├── Overview.vue
│   │   │           ├── Protection.vue
│   │   │           ├── Report.vue
│   │   │           └── Config.vue
│   │   └── vite.config.ts
│   │
│   ├── resource/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   └── pages/
│   │   │       ├── ClusterList.vue
│   │   │       ├── NodeList.vue
│   │   │       ├── GPUResources.vue
│   │   │       ├── Network.vue
│   │   │       ├── Storage.vue
│   │   │       ├── AgentList.vue
│   │   │       └── GSLB.vue
│   │   └── vite.config.ts
│   │
│   ├── service/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   ├── pages/
│   │   │   │   ├── DataService.vue
│   │   │   │   ├── MessageService.vue
│   │   │   │   ├── Governance.vue
│   │   │   │   └── Gateway.vue
│   │   │   └── sub-plugins/           # 子插件，按需动态加载
│   │   │       ├── mysql/
│   │   │       ├── postgresql/
│   │   │       ├── redis/
│   │   │       ├── kafka/
│   │   │       └── rabbitmq/
│   │   └── vite.config.ts
│   │
│   ├── ai/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   ├── plugin.json
│   │   │   └── pages/
│   │   │       ├── ModelRegistry.vue
│   │   │       ├── Inference.vue
│   │   │       ├── Agent.vue
│   │   │       └── VectorDB.vue
│   │   └── vite.config.ts
│   │
│   └── system/
│       ├── src/
│       │   ├── index.ts
│       │   ├── plugin.json
│       │   └── pages/
│       │       ├── Settings.vue
│       │       ├── UserList.vue
│       │       ├── RoleList.vue
│       │       ├── TenantList.vue
│       │       ├── OrgList.vue
│       │       ├── OperationApproval.vue
│       │       ├── AuditLog.vue
│       │       └── ExtensionList.vue
│       └── vite.config.ts
│
├── packages/                           # 共享库
│   ├── types/                          # 类型定义
│   │   ├── src/
│   │   │   ├── plugin.ts
│   │   │   ├── manifest.ts
│   │   │   ├── context.ts
│   │   │   └── index.ts
│   │   ├── package.json
│   │   └── tsconfig.json
│   │
│   ├── api-client/                     # API 客户端（从 OpenAPI 生成）
│   │   ├── src/
│   │   │   ├── client.ts
│   │   │   └── index.ts
│   │   ├── package.json
│   │   └── tsconfig.json
│   │
│   └── ui-kit/                         # 共享 UI 组件
│       ├── src/
│       │   ├── components/
│       │   │   ├── HNBTable.vue
│       │   │   ├── HNBForm.vue
│       │   │   ├── HNBChart.vue
│       │   │   └── index.ts
│       │   └── index.ts
│       ├── package.json
│       └── tsconfig.json
│
├── scripts/
│   ├── build-all.sh
│   ├── generate-manifest.mjs
│   └── dev.sh
│
├── pnpm-workspace.yaml
├── package.json
├── tsconfig.json
└── .gitignore
```

---

## 14. 与 V1.0 的关键差异

| 项目 | V1.0 | V2.0（优化后） |
|---|---|---|
| **插件加载** | 仅 Module Federation | 双模式：Local Bundle（默认）+ Remote MF（可选） |
| **Context** | Workspace Store | Context Store（tenantId/spaceId/environmentId/clusterId） |
| **Shell 职责** | 基础能力 | 新增 Capability Manager + Permission Manager |
| **Manifest** | 简单（name/entry/navIndex） | 完整（permissions/capabilities/lifecycle/menu/backend deps） |
| **可观测性** | 独立模块 | 嵌入为应用工厂/容器/资源的内置能力 |
| **云原生服务** | 单体插件 | 主插件 + 子插件（database/middleware/gateway 动态加载） |
| **权限模型** | 无 | 插件声明 required/optional 权限，Shell 动态过滤 |
| **前后端统一** | 无 | Extension Manifest 统一后端 + 前端，安装自动注册 |
| **菜单生成** | 手写 | Manifest 声明菜单，Shell 自动渲染 |
| **部署** | 统一 | 三级部署（最小/标准/完整），Helm enabled 控制 |

---

## 15. 落地路线图

| 阶段 | 内容 | 产出 |
|---|---|---|
| **Phase 0** | 工程搭建：Shell 微内核 + Plugin Loader 双模式 + 项目骨架 | 可运行 Shell，plugin-manifest 驱动菜单 |
| **Phase 1** | Dashboard + Application + Container 三个核心插件 | 覆盖核心运维场景 |
| **Phase 2** | Resource + Service（含子插件）+ System | 覆盖全功能 |
| **Phase 3** | AI Plugin + Extension 统一机制 + 远程插件模式 | 插件生态闭环 |
| **Phase 4** | Helm Chart 集成 + 最小部署验证 + 文档 | 可交付生产 |