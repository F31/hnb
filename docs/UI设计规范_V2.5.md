# HNB Web Console UI / UX 设计规范 V2.5

> 版本：V2.5　|　发布日期：2026-07-28　|　密级：内部
>
> 本版基于《HNB Web Console 微内核插件化架构设计 V3.6》和《UI / UX 设计规范 V2.0》进一步升级，重点建立一套**基于数据接口动态扩展的 UI 规范**。目标是在不修改 Shell 核心代码的前提下，通过服务端注册、UI Schema、组件注册表和插件 Bundle，实现导航、页面、表单、表格、仪表盘、详情区块和操作能力的动态扩展。

---

## 文档说明

### V2.0 → V2.5 核心增强

| 维度 | V2.0 | V2.5 |
|---|---|---|
| 扩展范围 | 主要覆盖动态菜单、插件装载、主题和多租户 | 扩展到导航、页面布局、表单、表格、详情、仪表盘、操作、字典和状态模型 |
| 页面实现 | 页面主要由插件代码实现 | 标准页面优先由服务端 `PageSchema` 驱动，复杂页面由注册组件或插件实现 |
| 组件使用 | 依赖 UI Kit，但缺少动态组件约束 | 增加 `Component Registry`，后端只能引用受信组件类型和受控属性 |
| 数据访问 | 描述按需请求，但未形成统一契约 | 增加 `DataSource`、Query、Mutation、分页、轮询和实时订阅契约 |
| 动态操作 | 未定义统一操作模型 | 增加导航、API、异步任务、工作流、下载和批量操作的统一 `ActionSchema` |
| 安全边界 | 明确前端不是安全边界 | 进一步禁止服务端下发任意 JavaScript、HTML、模板表达式和远程脚本 |
| 版本治理 | 主要依赖插件版本 | 增加 `apiVersion`、`schemaVersion`、`revision`、`minShellVersion` 和兼容策略 |
| 扩展生命周期 | 插件加载与激活分离 | 增加“注册—校验—发布—通知—刷新—回滚”的 UI 元数据生命周期 |
| 测试治理 | 未形成体系 | 增加 Schema Lint、契约测试、视觉回归、A11y 和动态扩展验收清单 |

### 阅读说明

- **【必须】**：强制约束，设计评审或代码评审不满足时不得发布。
- **【建议】**：推荐实践，允许在形成明确记录后例外处理。
- **【示例】**：用于说明数据结构或交互，不作为唯一实现。

### 规范目标

本规范重点解决以下问题：

1. 新增标准功能时，尽量通过数据库注册和 API 数据配置完成，不修改 Shell。
2. 新插件接入时，不重复建设布局、菜单、权限、表格、表单和异常状态。
3. UI 动态化不能演变为“后端下发任意代码”的安全风险。
4. 数据接口、设计语言、权限、上下文和组件能力保持统一。
5. 动态 UI 可以被验证、缓存、审计、灰度、回滚和兼容升级。

---

## 目录

1. 总体设计原则
2. UI 架构分层
3. 动态扩展模型与边界
4. UI 数据契约总则
5. 布局与页面模板
6. 服务端驱动导航
7. Page Schema 页面编排
8. Component Registry 组件注册
9. 动态表单规范
10. 动态表格与列表规范
11. 仪表盘、详情页与页内扩展
12. 操作与工作流规范
13. 数据源与状态管理规范
14. 插件装载与生命周期
15. 缓存、事件与性能治理
16. 权限、安全与多租户
17. Design Token、主题与国际化
18. 交互状态、反馈与动效
19. 无障碍与响应式设计
20. 版本、发布与兼容治理
21. 测试与验收规范
22. 附录

---

# 1. 总体设计原则

| 原则 | 说明 |
|---|---|
| 微内核 | Shell 只承载认证、布局、导航、路由、上下文、权限、组件注册和插件运行时 |
| 数据驱动 | 标准页面、菜单、表单、表格和操作优先由数据接口描述 |
| 单一权威来源 | 菜单、路由、页面 Schema、权限关联和插件状态由服务端控制面统一管理 |
| 渐进式扩展 | 标准 CRUD 使用 Schema；复杂交互使用注册组件；独立业务域使用插件 Bundle |
| 安全可控 | 服务端只能引用已注册组件和动作，不得下发任意可执行代码 |
| 默认拒绝 | 权限、上下文、插件或 Schema 未就绪时不展示、不执行、不默认放行 |
| 一致性 | Shell、内置插件、第三方插件和数据驱动页面使用同一设计系统与交互语义 |
| 可版本化 | 所有动态 UI 数据必须带版本和兼容信息，可灰度、回滚和审计 |
| 可观察 | 页面装载、组件渲染、接口调用、动作执行和插件失败均可监控 |
| 多租户隔离 | UI 缓存、页面状态、操作上下文和数据请求不得跨租户或空间复用 |
| 性能优先 | 首屏最小加载，Schema、Bundle、语言包和业务数据按需获取 |
| 可访问性 | 动态生成的 UI 与手写页面执行相同的 A11y 标准 |

**【必须】** “无需修改代码扩展”指**不修改 Shell 核心代码**。涉及全新业务逻辑、专用可视化或复杂交互时，可以开发插件或注册新组件，但必须通过标准注册接口接入。

---

# 2. UI 架构分层

```text
┌───────────────────────────────────────────────────────────────────────┐
│                         HNB Web Console Shell                         │
│ Auth │ Layout │ Navigation │ Router │ Context │ Permission │ i18n     │
│ UI Schema Engine │ Page Renderer │ Component Registry │ Action Engine │
│ Plugin Manager │ Event Bus │ API Client │ Error Boundary               │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ 读取受控 UI 契约
┌───────────────────────────────▼───────────────────────────────────────┐
│                         UI Extension Layer                            │
│ Page Schema │ Form Schema │ Table Schema │ Dashboard Widget           │
│ Registered Component │ Local/Remote Plugin Bundle │ Locale Bundle      │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ API / 注册 / 版本发布
┌───────────────────────────────▼───────────────────────────────────────┐
│                         HNB Backend Control Plane                     │
│ UI Registry │ Navigation Service │ Plugin Registry │ IAM/RBAC          │
│ Capability │ License │ Dictionary │ Workflow │ Data Adapter           │
│ PostgreSQL 权威数据 │ NATS/JetStream 事件与共享快照                    │
└───────────────────────────────────────────────────────────────────────┘
```

## 2.1 Shell 核心模块

| 模块 | 职责 |
|---|---|
| UI Schema Engine | 校验、解析和标准化服务端 UI Schema |
| Page Renderer | 按模板和布局树渲染页面 |
| Component Registry | 将受信 `componentType` 映射到实际 Vue 组件 |
| Data Source Manager | 执行统一查询、分页、刷新、轮询和取消请求 |
| Action Engine | 执行导航、API、异步任务、工作流、下载和批量操作 |
| Validation Engine | 执行标准校验规则和受控条件表达式 |
| Plugin Manager | 管理插件加载、激活、停用、隔离和卸载 |
| Navigation Manager | 获取服务端导航、版本比对和动态路由注册 |

## 2.2 服务端 UI Registry

UI Registry 是动态 UI 元数据的管理中心，至少维护：

```text
UIPage
UIPageVersion
UIComponentDefinition
UIFormSchema
UITableSchema
UIActionDefinition
UIDataSourceDefinition
UIDictionary
UIExtensionPoint
UIExtensionContribution
```

**【必须】** UI Registry 保存的是**声明式元数据和受信标识符**，不保存任意 JavaScript、Vue 模板或未经审核的 HTML。

---

# 3. 动态扩展模型与边界

## 3.1 四级扩展模型

| 级别 | 扩展方式 | 适用场景 | 是否需要前端代码 |
|---|---|---|---|
| L1 导航配置 | 数据库 + Navigation API | 菜单、分组、排序、显隐、权限、License | 否 |
| L2 Schema 页面 | Page/Form/Table/Action Schema | 标准 CRUD、配置页、详情页、报表页 | 否 |
| L3 注册组件 | Component Registry | 专用图表、拓扑、资源选择器、代码编辑器 | 开发一次，后续配置复用 |
| L4 插件 Bundle | Local/Remote Plugin | 独立业务域、复杂交互、完整子系统 | 是，但不修改 Shell |

## 3.2 优先级原则

新增功能按以下顺序选择实现方式：

```text
标准页面模板可以满足？
  ├─ 是 → 使用 Page Schema
  └─ 否 → 已有注册组件可以组合？
           ├─ 是 → 使用 Page Schema + 注册组件
           └─ 否 → 开发新注册组件或独立插件 Bundle
```

**【必须】** 不得为了追求“全部配置化”而将复杂业务逻辑编码成难以维护的超大 JSON Schema。

## 3.3 可以动态扩展的内容

- 菜单、路由、面包屑和页签；
- 页面标题、说明、帮助链接和状态提示；
- 栅格布局、区块、卡片和折叠面板；
- 表单字段、校验规则、联动关系和提交动作；
- 表格列、筛选、排序、分页、行操作和批量操作；
- 仪表盘 Widget、数据源、刷新周期和布局；
- 详情页分组、字段展示和关联资源；
- 权限、License、Feature Flag 和 Capability 条件；
- 字典、枚举、状态颜色和操作可用性；
- 插件扩展点和贡献项。

## 3.4 禁止动态下发的内容

- 任意 JavaScript、`eval` 表达式或函数体；
- 任意 Vue/React 模板字符串；
- 未过滤的 HTML；
- 任意远程脚本 URL；
- 认证 Token、Secret 或敏感凭据；
- 绕过 API Client 的直接网络请求配置；
- 可覆盖 Shell 核心组件或核心路由的注册项。

---

# 4. UI 数据契约总则

## 4.1 统一响应信封

所有 UI 元数据接口使用统一响应结构：

```json
{
  "apiVersion": "ui.hnb.io/v1",
  "kind": "PageSchema",
  "metadata": {
    "id": "cluster.list",
    "revision": 18,
    "etag": "page-cluster-list-r18",
    "generatedAt": "2026-07-28T12:00:00Z",
    "minShellVersion": "2.5.0"
  },
  "spec": {}
}
```

## 4.2 必需版本字段

| 字段 | 说明 |
|---|---|
| `apiVersion` | 服务端契约版本 |
| `kind` | Schema 类型，如 `PageSchema`、`FormSchema` |
| `metadata.id` | 全局唯一标识 |
| `metadata.revision` | 单调递增修订号 |
| `metadata.etag` | HTTP 缓存与版本比较 |
| `metadata.minShellVersion` | 最低兼容 Shell 版本 |
| `metadata.pluginId` | 所属插件，可为空表示 Shell 核心 |

## 4.3 通用条件模型

动态显隐和禁用只能使用受控条件 DSL：

```json
{
  "all": [
    { "permission": "cluster:update" },
    { "feature": "cluster-upgrade" },
    { "context": "environmentId", "exists": true }
  ]
}
```

允许的条件类型：

- `permission`
- `role`
- `feature`
- `license`
- `capability`
- `context`
- `fieldValue`
- `resourceState`

**【必须】** 条件表达式不得执行任意脚本。复杂权限和业务条件应由后端计算后返回布尔结果或状态码。

## 4.4 错误兼容

- 未识别的可选字段：忽略并记录调试日志；
- 未识别的必需组件类型：显示安全错误占位符；
- Schema 版本不兼容：拒绝渲染并展示升级提示；
- 单个区块失败：区块级隔离，不影响整页；
- 核心导航或根页面失败：进入最小安全页面。

---

# 5. 布局与页面模板

## 5.1 Shell 布局

Shell 统一管理：

- 顶部栏；
- 一级或侧边导航；
- 面包屑；
- 页面标题区；
- 内容容器；
- 全局通知和操作抽屉；
- 用户、租户、空间和环境切换器。

插件不得重复创建全局 Shell。

## 5.2 响应式断点

| 断点 | 视口范围 | 栅格 | 外边距 | 侧边栏 |
|---|---:|---:|---:|---|
| xs | `<576px` | 4 | 16px | 抽屉 |
| sm | `576–767px` | 4 | 24px | 抽屉 |
| md | `768–1023px` | 8 | 32px | 图标态 |
| lg | `1024–1439px` | 12 | 40px | 展开 |
| xl | `1440–1919px` | 12 | 64px | 展开 |
| xxl | `≥1920px` | 12 | ≥96px | 展开，内容限宽 |

## 5.3 标准页面模板

| 模板 | 用途 |
|---|---|
| `list` | 资源列表、筛选和批量操作 |
| `detail` | 资源详情、状态和关联资源 |
| `form` | 新建、编辑和配置 |
| `dashboard` | 指标卡、图表、告警和快捷入口 |
| `wizard` | 分步骤创建或安装流程 |
| `split` | 左侧资源树 + 右侧内容区 |
| `settings` | 分组配置、保存和恢复默认值 |
| `custom` | 由已注册插件组件完整承载 |

**【建议】** 标准业务页面优先选用模板，避免每个插件重新设计页面骨架。

---

# 6. 服务端驱动导航

## 6.1 唯一接口

```http
GET /api/v1/navigation/menus
```

服务端综合以下信息生成最终结果：

```text
用户身份 + 角色权限 + tenant/space/environment
+ 插件启用状态 + License + Feature Flag + Capability
= NavigationResponse
```

## 6.2 NavigationResponse

```json
{
  "revision": 42,
  "permissionVersion": "p28",
  "pluginCatalogVersion": "pc16",
  "menus": [
    {
      "id": "container",
      "titleKey": "nav.container",
      "icon": "container",
      "order": 100,
      "children": [
        {
          "id": "workloads",
          "titleKey": "nav.container.workloads",
          "routeName": "container.workloads",
          "pageId": "container.workloads.list"
        }
      ]
    }
  ],
  "routes": [
    {
      "name": "container.workloads",
      "path": "/container/workloads",
      "pageId": "container.workloads.list",
      "pluginId": "container"
    }
  ]
}
```

## 6.3 导航约束

- 页面导航最多三级；
- 第三级通常为叶子路由；
- 第四级及以上信息通过 Tab、Drawer、详情区块或局部导航表达；
- `routeName` 全局唯一；
- 路由只能引用 `pageId` 或 `pluginId + componentKey`；
- 前端不再从本地 Manifest 拼装菜单；
- 菜单 API 未完成时不得短暂显示未授权菜单。

## 6.4 禁用与隐藏

| 状态 | 展示方式 |
|---|---|
| 无权限 | 默认隐藏；产品要求展示时置灰并提示联系管理员 |
| License 未开通 | 可置灰并引导到应用市场或订阅页 |
| 插件未安装 | 默认隐藏 |
| 插件故障 | 显示不可用状态和诊断入口，仅管理员可见 |
| Capability 缺失 | 隐藏或展示环境不满足提示 |

---

# 7. Page Schema 页面编排

## 7.1 PageSchema 示例

```json
{
  "apiVersion": "ui.hnb.io/v1",
  "kind": "PageSchema",
  "metadata": {
    "id": "container.workloads.list",
    "revision": 12,
    "pluginId": "container",
    "minShellVersion": "2.5.0"
  },
  "spec": {
    "template": "list",
    "titleKey": "container.workloads.title",
    "descriptionKey": "container.workloads.description",
    "contextRequirements": ["tenantId", "spaceId", "clusterId"],
    "layout": {
      "type": "grid",
      "columns": 12,
      "gap": "md"
    },
    "regions": [
      {
        "id": "summary",
        "componentType": "MetricGroup",
        "span": 12,
        "props": { "dataSource": "workload.summary" }
      },
      {
        "id": "table",
        "componentType": "DataTable",
        "span": 12,
        "props": { "schemaId": "container.workloads.table" }
      }
    ]
  }
}
```

## 7.2 页面区域

标准区域包括：

- `header`：标题、描述、状态、主操作；
- `toolbar`：搜索、筛选、刷新和批量操作；
- `summary`：指标卡和概览；
- `content`：主列表、表单或详情；
- `aside`：辅助信息和帮助；
- `footer`：保存、取消和步骤导航。

## 7.3 布局规则

- 布局仅允许使用已注册的 `grid`、`stack`、`tabs`、`split`、`drawer` 等容器；
- `span` 采用 12 栅格；
- 服务端可声明响应式跨度，但必须有默认值；
- 动态区域不得覆盖 Shell 顶部栏、租户切换器或全局通知层；
- 页面必须声明加载、空、错误和无权限状态。

## 7.4 页面扩展点

标准页面可以声明扩展点：

```json
{
  "extensionPoint": "container.workload.detail.tabs",
  "accepts": ["tab", "panel"],
  "maxContributions": 10
}
```

插件通过注册接口贡献：

```json
{
  "extensionPoint": "container.workload.detail.tabs",
  "contributionId": "security.runtime-risk",
  "componentType": "RuntimeRiskPanel",
  "titleKey": "security.runtimeRisk",
  "order": 300,
  "condition": { "permission": "security:view" }
}
```

**【必须】** 扩展点必须明确允许的贡献类型、最大数量、权限边界和排序规则。

---

# 8. Component Registry 组件注册

## 8.1 组件分类

| 分类 | 示例 |
|---|---|
| 布局组件 | Grid、Stack、Tabs、SplitPane、Collapsible |
| 数据展示 | DescriptionList、StatusBadge、MetricCard、Timeline |
| 数据输入 | TextInput、Select、ResourcePicker、CodeEditor |
| 数据集合 | DataTable、TreeTable、CardList、VirtualList |
| 可视化 | LineChart、BarChart、Topology、ResourceGraph |
| 反馈组件 | Alert、EmptyState、ErrorState、Skeleton |
| 业务组件 | ClusterSelector、NamespaceSelector、ImageSelector |

## 8.2 注册定义

```typescript
interface ComponentDefinition {
  type: string
  version: string
  pluginId?: string
  propsSchema: object
  events: string[]
  slots?: string[]
  minShellVersion: string
  supportsTheme: boolean
  supportsI18n: boolean
  accessibilityLevel: 'AA' | 'partial'
}
```

## 8.3 注册约束

- `componentType` 必须唯一并使用命名空间，如 `container.WorkloadTopology`；
- 属性必须通过 JSON Schema 校验；
- 未声明的属性不得透传到 DOM；
- 组件不得读取全局 Token 或 LocalStorage 中的认证信息；
- 组件访问数据必须通过注入的 API Client；
- 组件事件必须转换为标准 Action，不得直接操作其他插件 Store；
- 插件停用后必须注销其组件定义。

## 8.4 组件版本兼容

- 小版本可增加可选属性；
- 删除属性或修改语义必须升级主版本；
- Page Schema 可声明组件版本范围；
- 不兼容时显示组件级错误，不执行未知行为。

---

# 9. 动态表单规范

## 9.1 FormSchema

```json
{
  "id": "cluster.create.form",
  "mode": "create",
  "layout": "two-column",
  "dataSource": "cluster.detail",
  "submitAction": "cluster.create",
  "fields": [
    {
      "name": "name",
      "componentType": "TextInput",
      "labelKey": "cluster.form.name",
      "required": true,
      "rules": [
        { "type": "pattern", "value": "^[a-z][a-z0-9-]{2,62}$" }
      ]
    },
    {
      "name": "provider",
      "componentType": "Select",
      "labelKey": "cluster.form.provider",
      "optionsSource": "provider.options"
    },
    {
      "name": "region",
      "componentType": "Select",
      "labelKey": "cluster.form.region",
      "optionsSource": "region.options",
      "visibleWhen": { "fieldValue": "provider", "notEmpty": true }
    }
  ]
}
```

## 9.2 字段类型

标准字段至少包括：

- 文本、数字、密码、多行文本；
- 单选、多选、级联选择；
- 日期、时间和时间范围；
- 开关、Slider；
- 标签、Key-Value、JSON/YAML 编辑器；
- 文件和证书上传；
- 集群、命名空间、存储类、镜像等资源选择器。

## 9.3 校验规则

| 类型 | 说明 |
|---|---|
| required | 必填 |
| min/max | 数值或长度范围 |
| pattern | 受控正则表达式 |
| enum | 枚举值 |
| unique | 服务端唯一性校验 |
| dependency | 跨字段依赖 |
| remote | 服务端校验接口 |

**【必须】** 服务端仍须重复执行全部业务校验；前端校验只用于即时反馈。

## 9.4 联动规则

允许：

- 字段显示/隐藏；
- 启用/禁用；
- 默认值更新；
- 选项源更新；
- 帮助文案更新；
- 清空依赖字段。

不允许通过任意脚本表达联动。

## 9.5 敏感数据

- Password、Token、Secret 默认不回显；
- 密钥字段不得进入前端日志和埋点；
- 文件和证书上传使用专用安全接口；
- Secret 仅显示引用信息，不显示明文；
- 离开未保存表单时必须提示。

---

# 10. 动态表格与列表规范

## 10.1 TableSchema

```json
{
  "id": "container.workloads.table",
  "dataSource": "workload.list",
  "rowKey": "uid",
  "pagination": { "mode": "server", "defaultPageSize": 20 },
  "columns": [
    {
      "field": "name",
      "titleKey": "common.name",
      "componentType": "ResourceLink",
      "sortable": true
    },
    {
      "field": "status",
      "titleKey": "common.status",
      "componentType": "StatusBadge",
      "dictionary": "workload.status"
    },
    {
      "field": "namespace",
      "titleKey": "k8s.namespace",
      "filterable": true
    }
  ],
  "rowActions": ["workload.view", "workload.restart", "workload.delete"],
  "batchActions": ["workload.batchDelete"]
}
```

## 10.2 数据量与渲染方式

| 数据量 | 推荐方式 |
|---:|---|
| `<50` | 可一次性渲染 |
| `50–500` | 服务端分页优先 |
| `>500` | 服务端分页 + 虚拟滚动 |
| 实时事件流 | 时间窗口 + 增量更新，不无限堆积 DOM |

## 10.3 列规范

- 默认最多展示 8～10 个主要列；
- 低优先级字段进入列设置或详情抽屉；
- 状态字段必须使用统一字典；
- 时间、容量、CPU、内存和网络流量统一格式化；
- 操作列固定在右侧时应保持最小宽度；
- 用户列偏好可保存，但不得跨租户保存业务筛选值。

## 10.4 筛选和查询

- 筛选条件必须映射到服务端允许的 Query 参数；
- 禁止客户端拼接任意 SQL 或查询表达式；
- 高级筛选应使用受控 Query DSL；
- URL 应保存可分享的非敏感筛选条件；
- 切换上下文时清空不再有效的筛选值。

## 10.5 操作展示

- 高频主操作最多 1 个；
- 行内直接操作建议不超过 3 个，其余进入更多菜单；
- 危险操作使用错误语义色，但不只依赖颜色；
- 不满足权限时隐藏或禁用，由产品语义决定；
- 不满足资源状态时禁用并解释原因。

---

# 11. 仪表盘、详情页与页内扩展

## 11.1 Dashboard Schema

仪表盘由 Widget 组成：

```json
{
  "template": "dashboard",
  "widgets": [
    {
      "id": "cluster-health",
      "componentType": "MetricCardGroup",
      "dataSource": "dashboard.clusterHealth",
      "layout": { "x": 0, "y": 0, "w": 6, "h": 2 },
      "refreshInterval": 30
    }
  ]
}
```

约束：

- 默认自动刷新不得低于 10 秒，特殊实时场景除外；
- 页面不可见时暂停轮询；
- Widget 失败只影响自身；
- 用户布局偏好与服务端默认布局分层保存；
- Widget 必须提供加载、无数据和失败状态。

## 11.2 详情页

详情页建议结构：

```text
标题 + 状态 + 主操作
概览字段
Tabs
├── 运行状态
├── 配置
├── 关联资源
├── 监控
├── 日志与事件
└── 插件扩展 Tab
```

## 11.3 字典和状态模型

服务端统一下发状态字典：

```json
{
  "code": "RUNNING",
  "labelKey": "status.running",
  "semantic": "success",
  "icon": "check-circle",
  "terminal": false
}
```

**【必须】** 插件不得自行定义同一业务状态的颜色和文案，避免跨页面表达不一致。

---

# 12. 操作与工作流规范

## 12.1 Action 类型

| 类型 | 说明 |
|---|---|
| `navigate` | 路由跳转或打开详情 |
| `api` | 同步 REST/HTTP 操作 |
| `operation` | 创建后端异步 Operation 并跟踪状态 |
| `workflow` | 发起审批或工作流 |
| `download` | 下载报告、日志或导出文件 |
| `openDrawer` | 打开抽屉页面或表单 |
| `openModal` | 打开轻量确认或编辑弹窗 |
| `emitEvent` | 向 Shell EventBus 发布受控事件 |

## 12.2 ActionSchema

```json
{
  "id": "workload.restart",
  "type": "operation",
  "labelKey": "workload.action.restart",
  "permission": "workload:restart",
  "enabledWhen": {
    "resourceState": ["RUNNING", "DEGRADED"]
  },
  "confirm": {
    "titleKey": "workload.restart.confirmTitle",
    "messageKey": "workload.restart.confirmMessage"
  },
  "request": {
    "method": "POST",
    "endpointId": "workload.restart",
    "pathParams": ["uid"]
  },
  "result": {
    "mode": "trackOperation",
    "successMessageKey": "workload.restart.submitted"
  }
}
```

## 12.3 安全约束

- API 地址必须引用已注册 `endpointId`，不得下发任意外部 URL；
- 服务端必须重新校验权限、租户和资源状态；
- 写操作必须支持幂等键或 Operation ID；
- 高风险操作必须二次确认；
- 批量操作必须展示影响范围；
- 异步任务进入统一 Operation Center；
- Action 执行结果不得直接注入 HTML。

---

# 13. 数据源与状态管理规范

## 13.1 DataSource 类型

| 类型 | 用途 |
|---|---|
| `query` | 普通查询 |
| `paginatedQuery` | 服务端分页列表 |
| `aggregate` | 指标聚合 |
| `dictionary` | 枚举和选项 |
| `stream` | SSE/WebSocket 增量数据 |
| `operationStatus` | 异步任务状态 |

## 13.2 DataSource 定义

```json
{
  "id": "workload.list",
  "type": "paginatedQuery",
  "endpointId": "k8s.workloads.list",
  "method": "GET",
  "contextBindings": ["tenantId", "spaceId", "clusterId"],
  "queryBindings": ["page", "pageSize", "keyword", "namespace", "status"],
  "responseMapping": {
    "items": "data.items",
    "total": "data.total"
  }
}
```

## 13.3 统一 API Client

插件和 Schema Renderer 只能通过统一 API Client 请求后端，API Client 统一处理：

- Token 和刷新；
- tenant/space/environment 上下文头；
- traceId；
- 超时和取消；
- 错误码标准化；
- 重试策略；
- 敏感字段脱敏；
- 审计上下文。

## 13.4 前端状态分层

| 状态 | 存储位置 |
|---|---|
| 登录和身份 | Auth Store |
| tenant/space/environment | Context Store |
| 权限和版本 | Permission Store |
| 导航和路由 | Navigation Store |
| UI 偏好 | Preference Store |
| 页面短期状态 | 页面或插件 Store |
| 权威业务数据 | 后端，不在前端长期持久化 |

**【必须】** 不得将跨租户业务数据持久化到 LocalStorage。必须持久化的 UI 偏好要使用用户和租户命名空间。

## 13.5 实时更新

- 前端不直接连接后端 NATS；
- 后端通过 SSE/WebSocket 或统一通知 API 转换事件；
- 数据刷新事件只携带资源标识和版本，不携带大量业务数据；
- 页面不可见时降低或暂停实时更新；
- 断线重连后执行版本校验或增量补偿。

---

# 14. 插件装载与生命周期

## 14.1 加载与激活分离

| 模块 | 职责 |
|---|---|
| PluginLoader | 下载/导入 Bundle、校验入口、创建插件实例 |
| PluginManager | 注册、激活、停用、卸载和状态管理 |
| Component Registry | 注册插件组件 |
| UI Registry Client | 获取插件贡献的页面、组件和扩展点数据 |

## 14.2 生命周期

```text
discovered → loading → loaded → activating → active
active → deactivating → inactive → unloading → unloaded
异常：load_failed / activate_failed / incompatible / disabled
```

最低生命周期接口：

```typescript
interface HNBPlugin {
  onLoad?(context: PluginContext): Promise<void> | void
  onActivate?(context: PluginContext): Promise<void> | void
  onDeactivate?(context: PluginContext): Promise<void> | void
  onUnload?(context: PluginContext): Promise<void> | void
}
```

## 14.3 生命周期约束

- 幂等；
- 超时保护；
- 插件级 Error Boundary；
- 释放事件、定时器、WebSocket、Store Watch 和路由；
- 插件禁用后注销组件和扩展贡献；
- 插件不能持有原始 Token；
- Remote Bundle 必须校验来源、版本、Digest 和签名。

## 14.4 扩展安装流程

```text
安装 Extension
→ 校验签名、版本和依赖
→ 部署后端能力
→ 注册 Plugin / Component / Page / Menu / Route / Action
→ 数据库事务提交
→ Transactional Outbox 发布 UI Registry 变更事件
→ NATS/JetStream 通知各后端实例刷新快照
→ Web Console 发现 revision 提升
→ 获取新 Navigation/Page Schema
→ 按需加载 Bundle
→ 动态显示新功能
```

该流程实现“新增扩展不修改 Shell 代码”。

---

# 15. 缓存、事件与性能治理

## 15.1 分层模型

```text
L1：浏览器内存 / 后端进程内缓存
  ↓ 未命中
L2：NATS JetStream KV 共享快照
  ↓ 未命中或版本不一致
L3：PostgreSQL 权威数据源
```

## 15.2 推荐 TTL

| 数据类型 | L1 | JetStream KV | 说明 |
|---|---:|---:|---|
| NavigationResponse | 30 秒 | 默认 10 分钟 | 版本事件即时失效，TTL 仅兜底 |
| Page/Form/Table Schema | 5 分钟 | 30 分钟 | 变更频率低，按 revision 切换 |
| 插件目录 | 1 分钟 | 15 分钟 | 插件启停事件即时刷新 |
| 字典和状态定义 | 5 分钟 | 30～60 分钟 | 可按字典版本缓存 |
| 权限快照 | 30 秒 | 5～10 分钟 | 高风险 API 仍实时鉴权 |
| 业务实时数据 | 由接口定义 | 通常不进入 UI 元数据缓存 | 使用业务缓存和实时系统 |

## 15.3 一致性机制

正常刷新依靠：

```text
数据库 revision 提升
+ Transactional Outbox
+ NATS 变更事件
+ 版本化缓存键
+ 前端 ETag / revision 比对
```

TTL 仅用于清理旧快照和处理漏事件。

## 15.4 缓存键

缓存键至少包含：

- tenantId；
- spaceId；
- permissionVersion 或 permissionSetHash；
- pluginCatalogVersion；
- UI revision；
- locale；
- Shell 主版本。

## 15.5 性能预算

| 指标 | 建议目标 |
|---|---:|
| Shell 初始 JS（gzip） | ≤ 300KB，业务插件不计入 |
| 首屏可交互时间 | 内网标准环境 ≤ 2.5 秒 |
| 导航接口 P95 | 缓存命中 ≤ 100ms，回源 ≤ 500ms |
| Page Schema P95 | 缓存命中 ≤ 100ms |
| 路由切换反馈 | 100ms 内出现反馈，主要内容 1 秒内开始渲染 |
| 单个插件 Bundle | 建议 gzip ≤ 500KB，超出需拆分 |
| 单个 UI Schema | 建议 ≤ 200KB |

## 15.6 Redis 引入边界

Redis 不作为 HNB 微内核默认依赖。仅在以下场景经压测后引入：

- 高频分布式热点缓存；
- 原子限流和配额扣减；
- 需要 Hash/Set/Sorted Set/Bitmap 等复杂结构；
- 第三方组件明确依赖 Redis。

---

# 16. 权限、安全与多租户

## 16.1 安全边界

- 菜单隐藏、按钮禁用和路由守卫不构成安全边界；
- 每个业务 API 独立验证权限和租户；
- Schema 只能引用服务端允许的数据源、Endpoint 和 Action；
- 动态组件只能从 Component Registry 加载；
- 用户输入不得直接成为组件类型、接口地址或 HTML；
- 所有 UI Registry 修改必须审计。

## 16.2 默认拒绝

以下任一状态未就绪时，相关 UI 不渲染：

- 权限；
- tenant/space 上下文；
- 插件状态；
- Schema 版本；
- Capability；
- License。

## 16.3 上下文模型

```typescript
interface AppContext {
  tenantId: string
  spaceId?: string
  environmentId?: string
  clusterId?: string
  generation: number
}
```

## 16.4 原子切换流程

```text
用户切换上下文
→ generation + 1
→ 阻止新的旧上下文操作
→ AbortController 取消旧请求
→ deactivate 旧上下文插件
→ 清理页面状态、导航、权限和数据缓存
→ 请求新上下文 Navigation/UI Schema
→ 激活新插件和路由
→ 恢复界面
→ 丢弃 generation 不匹配的迟到响应
```

## 16.5 UI Schema 隔离

- Schema 快照不得跨租户复用，除非权限集合、插件版本和配置完全一致；
- 用户自定义布局必须绑定用户与租户；
- 后端返回的资源链接必须重新检查租户范围；
- 跨租户操作需明确显示源租户和目标租户，并进行二次确认。

---

# 17. Design Token、主题与国际化

## 17.1 Design Token

所有视觉值必须语义化：

| 分类 | 示例 |
|---|---|
| Color | `color-bg-surface`、`color-text-primary`、`color-status-danger` |
| Typography | `font-size-body`、`font-weight-semibold` |
| Spacing | `space-xs`、`space-sm`、`space-md`、`space-lg` |
| Radius | `radius-sm`、`radius-md`、`radius-lg` |
| Elevation | `shadow-1`～`shadow-4` |
| Motion | `duration-fast`、`duration-normal` |
| Breakpoint | `breakpoint-md`、`breakpoint-xl` |

插件必须引用同一 `packages/ui-kit` 和 Token 契约，不得覆盖全局变量。

## 17.2 基础色彩

| 语义 | Light | Dark |
|---|---|---|
| Primary | `#2F6FED` | `#5B8DFF` |
| Success | `#12B76A` | `#12B76A` |
| Warning | `#F79009` | `#F79009` |
| Error | `#F04438` | `#F04438` |
| Background | `#FFFFFF` | `#101425` |
| Text | `#12172A` | `#EDEFF5` |

实际组件应使用 Token，不直接引用色值。

## 17.3 国际化

- 默认 `zh-CN`，支持 `en-US` 并预留其他语言；
- 所有动态 Schema 使用 `titleKey`、`labelKey`、`descriptionKey`；
- 插件语言包使用命名空间；
- Schema 不直接保存大量展示文案；
- 未找到语言 Key 时记录错误并显示安全占位文案；
- 日期、数字、容量和金额由统一 Formatter 处理；
- 语言切换不丢失当前路由、筛选和滚动位置。

## 17.4 动态 Schema 的 i18n

UI Registry 可注册新语言 Key，但发布时必须完成：

- 默认语言存在；
- 必需语言完整性校验；
- 参数占位符一致；
- 文案长度基本检查；
- 禁止在翻译中注入 HTML。

---

# 18. 交互状态、反馈与动效

## 18.1 标准页面状态

每个动态页面和组件必须具备：

- 初始化；
- 加载中；
- 正常；
- 空数据；
- 部分失败；
- 完全失败；
- 无权限；
- 不兼容；
- 离线或连接中断。

## 18.2 状态表现

| 状态 | 规范 |
|---|---|
| 加载 | 骨架屏尽量还原目标轮廓，避免跳动 |
| 空数据 | 说明原因，并提供合理下一步 |
| 部分失败 | 保留可用内容，错误区块独立重试 |
| 插件失败 | 插件级 Error Boundary，不影响 Shell |
| 无可信导航 | 仅显示最小安全页面 |
| 操作进行中 | 按钮进入 loading，防止重复提交 |
| 异步任务 | 进入 Operation Center，可离开页面继续执行 |

## 18.3 动效

| 类型 | 时长 | 场景 |
|---|---:|---|
| 微反馈 | 100–150ms | Hover、点击和状态变化 |
| 局部过渡 | 200–250ms | Tab、Drawer、Modal |
| 页面过渡 | 250–300ms | 路由和上下文切换 |

尊重 `prefers-reduced-motion`，动态 Schema 不得覆盖用户的减少动效设置。

---

# 19. 无障碍与响应式设计

## 19.1 A11y

- 正文对比度至少 4.5:1；
- 大字号文字至少 3:1；
- 所有操作支持键盘；
- 焦点顺序与视觉顺序一致；
- Modal/Drawer 必须焦点锁定并支持 Esc；
- 状态不能只靠颜色表达；
- 图标按钮必须有可访问名称；
- 动态表单错误需关联字段；
- 动态表格需提供表头语义和排序状态；
- 可点击热区不小于 44×44px。

## 19.2 Schema A11y 字段

组件 Schema 可以声明：

```json
{
  "ariaLabelKey": "workload.action.restart",
  "ariaDescriptionKey": "workload.action.restart.help",
  "liveRegion": "polite"
}
```

**【必须】** 缺少关键 A11y 信息的组件不得注册为公共组件。

## 19.3 响应式 Schema

允许声明：

```json
{
  "span": 6,
  "responsive": {
    "xs": 12,
    "md": 6,
    "xl": 4
  }
}
```

服务端只描述布局意图，最终断点行为由统一 Layout Renderer 决定。

---

# 20. 版本、发布与兼容治理

## 20.1 版本对象

```text
Shell Version
Plugin Version
Component Contract Version
UI Schema apiVersion
Page Revision
Navigation Revision
Permission Version
Dictionary Version
```

## 20.2 兼容策略

- Shell 支持当前和前一个稳定 `apiVersion`；
- Schema 新增可选字段保持向后兼容；
- 删除字段、改变语义或改变默认行为必须升级版本；
- 插件声明 `minShellVersion` 和 `maxShellVersion`；
- UI Registry 发布前执行兼容性检查；
- 不兼容版本不自动降级执行。

## 20.3 发布流程

```text
编辑 Draft
→ Schema Lint
→ 组件和 Endpoint 引用校验
→ 权限与安全扫描
→ 预览环境渲染
→ 契约测试 / 视觉回归 / A11y
→ 审批
→ 发布新 revision
→ Outbox + NATS 事件
→ 灰度租户
→ 全量发布
```

## 20.4 回滚

- 保留最近稳定 revision；
- 回滚只切换活动 revision，不直接覆盖历史记录；
- 回滚事件触发缓存和前端刷新；
- 插件 Bundle 和 Schema revision 必须保持兼容组合；
- 操作记录进入统一审计。

---

# 21. 测试与验收规范

## 21.1 自动化检查

| 检查 | 内容 |
|---|---|
| Schema Lint | 必填字段、类型、命名空间、组件属性 |
| 引用完整性 | page、component、action、endpoint、dictionary 是否存在 |
| 循环检查 | 菜单、页面引用、字段依赖无循环 |
| 安全检查 | 无脚本、无任意 URL、无 Secret、无 HTML 注入 |
| 权限检查 | 菜单、路由、页面、Action 权限关系完整 |
| i18n | 默认语言 Key 完整，参数一致 |
| A11y | Label、焦点和语义字段完整 |
| 性能 | Schema 大小、Bundle 大小、请求数量和渲染耗时 |

## 21.2 端到端场景

至少验证：

1. 插件安装后，无需修改 Shell 即出现新菜单和页面。
2. 插件禁用后，菜单、路由、组件和扩展点全部卸载。
3. 权限收回后，页面和操作立即失效，业务 API 返回拒绝。
4. tenant A 切换 tenant B 后，不残留 A 的导航、筛选、数据和订阅。
5. 未识别组件类型只影响对应区块。
6. Page Schema 回滚后前端正确切换 revision。
7. NATS 事件遗漏时，TTL 和 ETag 能最终恢复正确版本。
8. Remote Bundle 加载失败时 Shell 正常运行。
9. 中英文切换后动态 Schema 页面无 Key 泄露和严重溢出。
10. 键盘可以完成动态表单、表格和弹窗操作。

## 21.3 设计评审清单

- 是否可以复用标准模板？
- 是否需要新注册组件，还是 Schema 组合即可？
- 页面状态是否完整？
- 操作权限和资源状态是否区分？
- 是否存在第四级导航？
- 是否符合 Token 和 i18n？
- 是否明确 tenant/space 上下文？
- 是否可能泄露跨租户缓存？
- 是否定义兼容和回滚方案？
- 是否满足性能预算？

---

# 22. 附录

## 22.1 推荐 API

```http
GET  /api/v1/navigation/menus
GET  /api/v1/ui/pages/{pageId}
GET  /api/v1/ui/forms/{schemaId}
GET  /api/v1/ui/tables/{schemaId}
GET  /api/v1/ui/dictionaries/{dictionaryId}
GET  /api/v1/ui/components
GET  /api/v1/ui/extensions/{extensionPoint}
POST /api/v1/ui/actions/{actionId}:execute
```

接口可以由统一 UI Registry Service 提供，也可由 API Gateway 聚合，但对前端保持统一契约。

## 22.2 命名规范

| 类型 | 规范 | 示例 |
|---|---|---|
| Page ID | `[plugin].[domain].[page]` | `container.workloads.list` |
| Component Type | `[plugin].[ComponentName]` | `security.RuntimeRiskPanel` |
| Action ID | `[domain].[resource].[verb]` | `workload.instance.restart` |
| DataSource ID | `[domain].[resource].[query]` | `workload.list` |
| Extension Point | `[plugin].[page].[region]` | `container.workload.detail.tabs` |
| i18n Key | `[plugin].[module].[field]` | `service.mysql.form.host` |
| Token | `[category]-[semantic]-[level]` | `color-status-danger` |
| NATS Subject | `hnb.ui.[resource].[event]` | `hnb.ui.page.published` |

## 22.3 核心事件建议

```text
hnb.ui.navigation.changed
hnb.ui.page.published
hnb.ui.page.rolled-back
hnb.ui.plugin.changed
hnb.ui.component.changed
hnb.ui.dictionary.changed
hnb.iam.permission.changed
hnb.context.capability.changed
```

前端不直接订阅 NATS；由后端服务转换为 SSE/WebSocket 或 revision 查询结果。

## 22.4 V2.5 关键设计决策

1. 动态 UI 不只包括菜单，还包括页面、表单、表格、操作、仪表盘和扩展点。
2. 服务端数据不能直接等同于可执行 UI，必须经过 Component Registry 和 Schema 校验。
3. 标准 CRUD 页面优先使用 Schema，复杂页面使用注册组件或插件 Bundle。
4. “无需修改代码”限定为无需修改 Shell；新组件可以独立开发并注册复用。
5. UI Registry、Navigation、Plugin Registry、IAM 和 Capability 联合决定最终界面。
6. 动态接口只引用受信 `pageId`、`componentType`、`endpointId` 和 `actionId`。
7. 所有 Schema 都必须版本化、可灰度、可回滚、可审计。
8. PostgreSQL 是 UI 元数据权威来源，NATS/JetStream 用于事件和可重建共享快照。
9. 前端不直接连接 NATS，也不在本地推导权限。
10. 多租户上下文切换必须原子化，所有迟到响应按 generation 丢弃。
11. Design Token、i18n、A11y 和异常状态对动态页面与代码页面同等强制。
12. Redis 不作为 UI 控制面的默认依赖，只有出现明确高频缓存或复杂数据结构需求时引入。

## 22.5 版本记录

| 版本 | 日期 | 说明 |
|---|---|---|
| V1.0 | 2026-07 | 通用 UI 规范，覆盖布局、导航、i18n、按需加载和主题 |
| V2.0 | 2026-07-28 | 适配微内核插件架构，增加服务端导航、权限、多租户和缓存治理 |
| **V2.5** | **2026-07-28** | 建立数据接口驱动的 UI 扩展体系，新增 Page Schema、Component Registry、Form/Table/Action/DataSource 契约、扩展点、版本治理和完整验收标准 |

---

— 本文档由 HNB 设计系统与 Web Console 架构团队共同维护 —
