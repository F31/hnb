# HNB Web Console 微内核插件化架构设计 V3.0

> 基于 V2.5 架构设计，完善平台租户与 Harbor Project 联动、制品多租户隔离、共享授权、Robot Account、租户生命周期及 Operator 云原生服务治理机制，2026-07-26

---

## 1. 设计原则

1. **微内核** — Shell 只做核心事，全部业务能力由插件提供
2. **双模式插件** — 默认 Local Bundle，可选 Remote Module Federation
3. **按需部署** — 每个插件可独立启用/禁用，最小部署仅 Shell + Dashboard
4. **权限驱动** — 菜单和路由由用户权限 + 插件声明动态生成
5. **前后端统一** — Backend Extension 安装自动注册 Frontend Plugin
6. **零耦合** — 插件之间禁止互相引用，通过 Shell EventBus 通信
7. **产品与制品分离** — 应用市场管理产品、版本、依赖和生命周期；Harbor 仅保存不可变 OCI 制品
8. **应用与服务分治** — 单体/微服务应用按 Helm Release 管理；数据库/中间件按 Operator + CR 管理
9. **控制器与实例分离** — Operator 是控制器，服务实例是其管理的数据库/中间件工作负载
10. **版本可追溯** — 发布版本绑定 OCI Digest、签名、SBOM 和兼容矩阵，生产环境禁止漂移标签
11. **租户与仓库解耦映射** — 平台租户通过映射关系关联一个或多个 Harbor Project，不将租户表直接绑定单一仓库名称
12. **共享与私有分层** — 平台官方 Operator、云原生服务和公共市场制品集中共享；租户自有应用进入租户私有 Project
13. **代理访问优先** — 普通用户通过 HNB Artifact Service 访问 Harbor，CI/CD 和运行集群使用最小权限 Robot Account
14. **生命周期保护** — 租户停用、删除和 Project 清理必须检查运行实例、制品引用、审计保留及集群拉取依赖

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


### 2.1 应用市场、Harbor 与运行时控制平面

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


### 2.2 租户、应用市场与 Harbor Project 联动

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

## 10. 权限模型

### 10.1 权限评估流程

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

### 10.2 权限声明示例

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


### 10.3 平台角色与 Harbor 权限映射

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

### 10.4 多层授权判定

制品操作需要经过以下授权链：

```text
用户身份与租户角色
→ 当前 tenantId/spaceId/environmentId 上下文
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

删除与跨 Project 晋级属于高风险操作，应支持审批、双人复核和完整审计。

### 10.5 租户隔离边界

- 租户 A 不得通过 Repository 路径猜测、Harbor API 或缓存索引看到租户 B 的私有制品。
- Artifact Service 的所有查询必须带 `tenantId`，并在服务端重新计算可访问 Project 集合。
- Space 和 Environment 只能缩小访问范围，不能扩大租户已有 Project 授权。
- Operator 等平台级制品由租户只读使用；租户不得替换同名官方制品。
- 跨租户共享必须创建显式 `ProductVisibilityGrant` 和 Project `GRANTED` 绑定，并支持随时撤销。


---

## 11. 前后端扩展与制品统一

### 11.1 Extension Manifest（统一清单）

Extension Manifest 描述 HNB 平台扩展能力；Marketplace Product Manifest 描述可供用户安装的产品。二者可关联，但不可混为同一对象。

```yaml
# extension.yaml
name: postgresql-service-extension
version: 3.0.0
tier: T1

extensionType: cloud-service-provider

backend:
  services:
    - name: postgresql-provider-adapter
      image:
        ref: harbor.hnb.local/hnb-plugins/postgresql-provider@sha256:...
      replicaCount: 1
  apiRoutes:
    - /api/v1/cloud-services/postgresql

frontend:
  plugin:
    name: postgresql-service
    mode: local
    bundle:
      ref: harbor.hnb.local/hnb-plugins/postgresql-ui@sha256:...
    manifest: /config/plugin-postgresql-service.json

marketplace:
  products:
    - productId: postgresql-ha
      productType: CLOUD_NATIVE_SERVICE
      version: 17.2.1

operator:
  scope: cluster
  crdChart:
    ref: oci://harbor.hnb.local/hnb-operators/postgresql/crds
    version: 1.26.0
    digest: sha256:...
  controllerChart:
    ref: oci://harbor.hnb.local/hnb-operators/postgresql/operator
    version: 1.26.0
    digest: sha256:...
  instanceChart:
    ref: oci://harbor.hnb.local/hnb-cloud-services/postgresql/instance-chart
    version: 3.0.0
    digest: sha256:...

capabilities:
  provides:
    - database-service.postgresql
    - backup
    - restore
  requires:
    - kubernetes
    - persistent-volume

compatibility:
  kubernetes: ">=1.30"
  architectures: [amd64, arm64]

enabledByDefault: false
```

### 11.2 云原生服务扩展安装流程

```text
安装 postgresql-service-extension：
  1. 校验 Extension 签名、Digest、平台版本和集群能力
  2. 部署 postgresql-provider-adapter
  3. 注册 API 路由到 apiserver
  4. 注册前端 sub-plugin 到 Plugin Registry
  5. 注册 ProductVersion 到 Marketplace Catalog
  6. Operator Manager 安装或关联 CRD + Operator
  7. Capability Manager 注册 database-service.postgresql
  8. Shell 重新加载插件，云原生服务菜单显示 PostgreSQL
```

### 11.3 应用产品安装流程

单体和微服务应用无需注册 Operator：

```text
发布应用产品：
  1. Marketplace 注册 ProductVersion
  2. 绑定 Chart、镜像集合、参数 Schema 和 OCI Digest
  3. 用户选择目标环境并提交参数
  4. Application Installer 通过 Helm/GitOps 部署
  5. 注册 ApplicationInstance、访问入口和可观测性
```

### 11.4 能力发现

Capability Manager 不只检查某个后端 Pod 是否存在，还应综合判断：

- Extension 是否启用且健康。
- 所需 OCI 制品是否可拉取并通过签名验证。
- 目标集群是否安装兼容的 CRD/Operator。
- 目标集群是否具备 StorageClass、Snapshot、GPU、LoadBalancer 等能力。
- 当前租户是否具有产品订阅、配额和操作权限。
- 当前租户是否拥有或获授权访问制品所在 Harbor Project。
- 目标环境是否已同步有效的 Runtime Pull Secret，Robot Account 是否未过期。
- ProductVersion 引用的 Digest 是否仍受引用保护且未被撤销。

---

## 12. 模块通信

### 12.1 规则

- 插件禁止 import 其他插件
- 插件禁止访问其他插件的内部状态
- 跨插件通信走 Shell EventBus

### 12.2 EventBus 事件表

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
'tenant:artifact-ready' // 租户制品 Project 初始化完成
'artifact:project-bound' // 租户与 Harbor Project 绑定完成
'artifact:promoted'      // 制品完成环境晋级
'artifact:access-revoked'// Project 或产品授权被撤销
```

### 12.3 使用示例

```typescript
// 应用工厂插件发布事件
eventBus.emit('application:created', { appId: '...', spaceId: '...' })

// 仪表盘插件监听事件，刷新数据
eventBus.on('application:created', () => {
  dashboardStore.refresh()
})
```

---

## 13. 部署方案

### 13.1 最小部署

```
Shell + Dashboard
适用场景：POC、演示、轻量运维
```

### 13.2 标准部署

```
Shell + Dashboard + Application + Container + Resource + System
适用场景：生产环境，覆盖核心运维场景
```

### 13.3 完整部署

```
Shell + Dashboard + Application + Container + Resource + Service + AI + System
适用场景：全功能企业级部署
```

### 13.4 Helm Chart 集成

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

### 13.5 Harbor、应用市场与租户 Project 配置

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

  sharedProjects:
    platform: hnb-platform
    operators: hnb-operators
    cloudServices: hnb-cloud-services
    marketplace: hnb-marketplace
    plugins: hnb-plugins
    quarantine: hnb-quarantine

artifactGovernance:
  enabled: true
  accessMode: proxy-first
  tenantProjects:
    enabled: true
    defaultPattern: "tenant-{tenantId}"
    defaultScope: TENANT_PRIVATE
    defaultQuota: 100Gi
    splitProductionProject: false
    productionPattern: "tenant-{tenantId}-prod"
  robotAccounts:
    createCI: true
    createRuntime: true
    createPromotion: false
    rotateBeforeExpiryDays: 15
  secretSync:
    enabled: true
    targetNamespaces: tenant-authorized
  deletionProtection:
    checkRuntimeReferences: true
    retentionDays: 30
    archiveBeforeDelete: true

operatorManager:
  enabled: true
  installPolicy: admin-controlled
  defaultScope: cluster
  crdUpgradePolicy: explicit

applicationInstaller:
  helm:
    enabled: true
  gitops:
    enabled: false
```

部署建议：

- 轻量版可复用客户已有 Harbor；完整部署可随平台提供 Harbor，但 Harbor 与 HNB 保持独立生命周期。
- 普通租户默认创建一个私有 Project；只有生产隔离或强合规场景才拆分多个 Project。
- 平台公共应用、Operator 和云原生服务使用共享 Project，租户只读引用相同 Digest。
- 离线环境通过 Harbor 复制包导入，ProductVersion 中的 Digest 不变。
- 集群拉取凭据使用租户 Runtime Robot Account；发布流水线使用独立 CI Robot Account。
- 市场数据库和 Harbor 数据均需备份，恢复时通过 ProductVersion、ProjectBinding 与 Digest 校验索引一致性。
- 租户删除不得直接触发 Project 删除，必须经过引用检查、保留期和归档流程。

### 13.6 Docker Compose 集成

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

## 14. TypeScript 类型定义

### 14.1 插件接口

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

### 14.2 Manifest 类型

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

### 14.3 上下文类型

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


### 14.4 应用市场与制品类型

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


### 14.5 租户制品治理类型

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


### 15.1 后端新增模块建议

```text
backend/
├── marketplace/
│   ├── catalog/                       # Product / ProductVersion
│   ├── subscription/
│   ├── release/
│   └── artifact-index/
├── artifact-registry/
│   ├── harbor-client/
│   ├── project-mapper/
│   ├── access-broker/
│   ├── robot-manager/
│   ├── secret-sync/
│   ├── quota-policy/
│   ├── reference-guard/
│   ├── lifecycle-controller/
│   ├── digest-verifier/
│   ├── signature-verifier/
│   └── replication/
├── application-installer/
│   ├── helm/
│   └── gitops/
├── cloud-service/
│   ├── operator-manager/
│   ├── service-installer/
│   ├── adapters/
│   │   ├── postgresql/
│   │   ├── mysql/
│   │   ├── redis/
│   │   ├── kafka/
│   │   └── rabbitmq/
│   └── lifecycle/
└── operation-worker/
    ├── install/
    ├── upgrade/
    ├── rollback/
    ├── backup/
    └── delete/
```

---

## 16. V3.0 的关键增强

| 项目 | V2.5 | V3.0 |
|---|---|---|
| **租户与 Harbor** | 提出共享产品仓库和租户私有 Project | 建立 TenantArtifactProjectBinding，支持拥有与授权、多 Project 和多 Harbor 实例 |
| **Project 映射** | 仅按平台/应用/服务划分 Project | 使用稳定 tenantId 自动创建私有 Project，并支持 nonprod/prod 分层策略 |
| **共享制品** | 平台共享仓库概念 | 官方 Operator、云原生服务和公共应用集中共享，租户以只读 GRANTED 关系引用 |
| **产品可见性** | ProductVersion 包含租户可见性 | 明确五类 Visibility，并与订阅、Project 授权、发布状态联合判定 |
| **访问模式** | Robot Account 基础建议 | 默认 HNB 代理访问，CI/Runtime/Promotion Robot 按职责和最小权限分离 |
| **凭据治理** | Secret 不落明文 | 增加 Robot 生命周期、自动轮换、Secret 按 Namespace 精确同步 |
| **租户生命周期** | 未建立完整联动 | 创建、停用、保留、归档、删除全状态机，并加入运行引用保护 |
| **制品清理** | 已引用版本不得删除 | 增加 ArtifactReferenceUsage 和 Reference Guard，覆盖 Pod 拉取与跨租户共享引用 |
| **权限体系** | 插件和市场操作权限 | 增加 Artifact Project、Push/Pull、晋级、Robot、配额等细粒度权限 |
| **部署配置** | Harbor 共享 Project 配置 | 增加租户 Project 模板、配额、生产拆分、Robot、Secret 同步和删除保护配置 |
| **后台模块** | Harbor Client、签名和复制 | 增加 Project Mapper、Access Broker、Robot Manager、Secret Sync、Quota、Lifecycle Controller |
| **控制台管理** | 租户与扩展管理 | System Plugin 增加 Harbor 实例、Project 映射、Robot、配额和引用清理页面 |

---

## 17. 落地路线图

| 阶段 | 内容 | 产出 |
|---|---|---|
| **Phase 0** | Shell 微内核、Plugin Loader 双模式、项目骨架 | 可运行 Shell，Manifest 驱动菜单 |
| **Phase 1** | Dashboard、Application、Container 核心插件 | 覆盖应用与容器基本运维 |
| **Phase 2** | Marketplace Catalog + Harbor Client + ProductVersion/Digest 模型 | 单体应用 OCI Chart 上架、安装、升级、回滚闭环 |
| **Phase 3** | Tenant Project Mapper、共享授权、配额、Robot Account、Secret Sync | 租户创建后自动获得私有仓库和平台共享制品访问能力 |
| **Phase 4** | 微服务 Bundle、组件锁定、依赖与 GitOps 可选集成 | 微服务应用组合交付闭环 |
| **Phase 5** | Operator Manager + Service Adapter + PostgreSQL/Redis 首批服务 | 云原生服务安装、状态、备份、删除保护闭环 |
| **Phase 6** | Kafka/RabbitMQ/MySQL 扩展，CRD/Operator 分层升级 | 中间件和数据库服务矩阵 |
| **Phase 7** | 签名、SBOM、漏洞门禁、发布审批、跨 Project 晋级复制 | 企业级供应链安全与环境晋级闭环 |
| **Phase 8** | 租户停用/归档/删除、引用扫描、Robot 轮换、灾备恢复 | 多租户制品全生命周期闭环 |
| **Phase 9** | 离线包、多 Harbor 实例、规模化多集群验证 | 可交付生产与国产化/离线环境 |

推荐优先完成以下最小闭环：

```text
创建租户
→ 自动创建 tenant-{tenantId} Project
→ 创建 CI/Runtime Robot 并同步 Pull Secret
→ 授权访问平台共享 Project
→ ProductVersion(Digest) 上架
→ 租户单体应用安装
→ PostgreSQL Operator 复用/安装
→ PostgreSQL ServiceInstance 创建
→ 状态回传、备份与删除保护
→ 租户停用时引用检查与凭据收敛
```

---

## 18. 设计决策摘要

1. 应用市场统一展示产品，但单体应用、微服务应用和云原生服务使用不同安装器和生命周期模型。
2. Harbor 统一保存 OCI 制品，但不承担产品目录、订阅、审批、租户关系和运行实例管理。
3. 平台租户通过映射实体关联 Harbor Project，不把租户记录直接绑定一个仓库名称。
4. 默认一个租户拥有一个私有 Project；严格合规场景可拆分 nonprod/prod，但不默认按每个 Space 或环境创建 Project。
5. 官方 Operator、CRD、云原生服务和公共应用集中保存在平台共享 Project，租户只读引用相同 Digest。
6. 产品可见性、租户订阅、Project 授权、制品状态和环境策略必须联合判定。
7. 普通用户通过 HNB Artifact Service 代理访问 Harbor；CI/CD 和 Kubernetes 使用用途隔离的 Robot Account。
8. Robot 凭据进入 Secret/Vault，平台数据库仅保存引用和生命周期元数据，并支持自动轮换。
9. 单体与微服务应用属于业务应用平面，主要由 Helm/GitOps 管理；数据库与中间件属于云原生服务平面，由 Operator + CR 管理。
10. Operator 控制器通常每集群或管理域安装一次，一个控制器管理多个服务实例。
11. CRD、Operator、实例模板和引擎镜像独立版本化，由 ProductVersion 兼容矩阵组合并固定 OCI Digest。
12. Harbor 不保存数据库实际数据，数据通过 PVC、分布式存储、快照和对象存储备份管理。
13. 租户停用先关闭 Push 和新部署，再检查运行实例和拉取依赖；租户删除不能直接删除 Harbor Project。
14. 所有被 ProductVersion、运行实例、集群工作负载或跨租户共享关系引用的制品都受删除保护。
15. HNB 通过 Project Mapper、Access Broker、Reference Guard 和 Service Adapter 隔离 Harbor 与第三方 Operator 的实现细节。

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
