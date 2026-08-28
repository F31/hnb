/**
 * UI Schema 类型定义（UI 规范 V2.5 §4 / §7 / §12 / §13）。
 *
 * 安全边界：Schema 只承载声明式元数据与受信标识符
 * （pageId / componentType / endpointId / actionId），
 * 禁止任意 JavaScript、模板字符串与未过滤 HTML（V2.5 §3.4）。
 */

/** 统一响应信封（V2.5 §4.1） */
export interface SchemaEnvelope<S = unknown> {
  apiVersion: string
  kind: string
  metadata: SchemaMetadata
  spec: S
}

export interface SchemaMetadata {
  id: string
  revision: number
  etag?: string
  generatedAt?: string
  minShellVersion?: string
  pluginId?: string
  /** 服务端语言文案表，PageRenderer 的 texts prop 来源 */
  texts?: Record<string, string>
}

export type PageTemplate =
  | 'list'
  | 'detail'
  | 'form'
  | 'dashboard'
  | 'wizard'
  | 'split'
  | 'settings'
  | 'custom'

export interface PageLayout {
  type: 'grid' | 'stack' | 'tabs' | 'split' | 'drawer'
  columns?: number
  gap?: 'xs' | 'sm' | 'md' | 'lg'
}

export interface PageRegion {
  id: string
  /** 受信组件类型，必须存在于 ComponentRegistry */
  componentType: string
  /** 声明式扩展点命名空间（如 `resource.cluster.detail.tabs`）；配置时经
   *  ExtensionRegistry 解析并渲染已注册且调用方有权限的扩展，忽略 componentType */
  extensionPoint?: string
  /** 12 栅格跨度，默认 12 */
  span?: number
  responsive?: Partial<Record<'xs' | 'sm' | 'md' | 'lg' | 'xl' | 'xxl', number>>
  /** 受控属性，须经组件的 propsSchema 校验 */
  props?: Record<string, unknown>
  condition?: Condition
}

export interface PageSpec {
  template: PageTemplate
  titleKey?: string
  descriptionKey?: string
  contextRequirements?: string[]
  layout?: PageLayout
  endpoints?: EndpointDefinition[]
  dataSources?: DataSourceDefinition[]
  actions?: ActionSchema[]
  regions: PageRegion[]
}

export interface EndpointDefinition {
  id: string
  path: string
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
}

export type PageSchema = SchemaEnvelope<PageSpec>

/** 受控条件 DSL（V2.5 §4.3），禁止任意脚本 */
export interface Condition {
  all?: ConditionTerm[]
  any?: ConditionTerm[]
}

/**
 * V2.6 §4.3：仅允许 permission / feature / license / capability / context。
 * 已移除字段（role / exists / fieldValue / notEmpty / resourceState）
 * 由 ConditionEvaluator 忽略并记录调试日志，不拒绝渲染。
 */
export interface ConditionTerm {
  permission?: string
  feature?: string
  license?: string
  capability?: string
  context?: string
}

/** Action（V2.5 §12） */
export type ActionType =
  | 'navigate'
  | 'api'
  | 'operation'
  | 'workflow'
  | 'download'
  | 'openDrawer'
  | 'openModal'
  | 'emitEvent'

export interface ActionSchema {
  id: string
  type: ActionType
  labelKey?: string
  permission?: string
  enabledWhen?: Condition
  confirm?: {
    titleKey?: string
    messageKey?: string
  }
  request?: {
    method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
    /** 受信端点标识，必须已在 DataSourceManager 注册，禁止任意 URL */
    endpointId: string
    pathParams?: string[]
    queryParams?: string[]
  }
  route?: {
    name: string
    params?: Record<string, string>
  }
  event?: {
    name: string
    payloadKeys?: string[]
  }
  result?: {
    mode?: 'toast' | 'trackOperation' | 'silent'
    successMessageKey?: string
  }
}

/** DataSource（V2.5 §13） */
export type DataSourceType =
  | 'query'
  | 'paginatedQuery'
  | 'aggregate'
  | 'dictionary'
  | 'stream'
  | 'operationStatus'

export interface DataSourceDefinition {
  id: string
  type: DataSourceType
  endpointId: string
  method?: 'GET' | 'POST'
  contextBindings?: string[]
  queryBindings?: string[]
  responseMapping?: {
    items?: string
    total?: string
  }
  /** 缓存配置。未配置时默认不缓存，每次请求都发起 HTTP 调用。
   *  仅对低频变化、用户不实时等待的数据（如字典、选项源）开启缓存。 */
  cache?: {
    /** 缓存模式：auto（TTL 30s）/ realtime（TTL 5s） */
    mode: 'auto' | 'realtime'
    /** 自定义 TTL（秒），覆盖模式默认值 */
    ttl?: number
  }
}

export interface EndpointDefinition {
  id: string
  path: string
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
}

export interface PaginatedResult<T = unknown> {
  items: T[]
  total: number
}
