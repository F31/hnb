/**
 * ActionEngine — 统一动作执行（V2.5 §12）。
 *
 * 安全约束（V2.5 §12.3）：
 *  - api 动作必须引用已注册 endpointId，禁止任意外部 URL；
 *  - 带 permission 的动作执行前做前端防御性检查（后端仍为权威）；
 *  - confirm 动作必须经确认处理器二次确认；
 *  - 执行结果不得注入 HTML，消息仅作纯文本展示。
 */

import type { ApiClient, EventBus } from '@hnb/types'
import type { ActionSchema } from './types'
import type { DataSourceManager } from './DataSourceManager'

export interface ActionContext {
  /** 行数据或表单值，用于 pathParams/queryParams/event payloadKeys 取值 */
  record?: Record<string, unknown>
  params?: Record<string, unknown>
}

export interface ActionEngineOptions {
  apiClient: ApiClient
  eventBus: EventBus
  dataSources: DataSourceManager
  /** 权限防御性检查 */
  hasPermission?: (permission: string) => boolean
  /** 路由跳转（由 Shell 注入，避免引擎直接依赖 vue-router） */
  navigate?: (name: string, params?: Record<string, string>) => void
  /** 确认对话框（由 Shell 注入 UI 实现；返回是否确认） */
  confirm?: (message: string) => Promise<boolean>
  /** 抽屉/弹窗打开器（由 Shell 注入 UI 实现） */
  openOverlay?: (action: ActionSchema, ctx: ActionContext) => void
  /** 结果提示（纯文本） */
  notify?: (message: string) => void
}

export class ActionEngine {
  constructor(private options: ActionEngineOptions) {}

  async execute(action: ActionSchema, ctx: ActionContext = {}): Promise<void> {
    if (!action?.id || !action.type) throw new Error('Invalid action')

    if (action.permission && this.options.hasPermission && !this.options.hasPermission(action.permission)) {
      throw new Error(`Permission denied for action "${action.id}"`)
    }

    if (action.confirm && this.options.confirm) {
      const message = action.confirm.messageKey ?? action.confirm.titleKey ?? action.id
      const confirmed = await this.options.confirm(message)
      if (!confirmed) return
    }

    switch (action.type) {
      case 'navigate': {
        if (!action.route?.name) throw new Error(`Action "${action.id}" missing route`)
        this.options.navigate?.(action.route.name, action.route.params)
        return
      }
      case 'api':
      case 'operation':
      case 'workflow': {
        await this.executeRequest(action, ctx)
        this.notifyResult(action)
        return
      }
      case 'emitEvent': {
        if (!action.event?.name) throw new Error(`Action "${action.id}" missing event`)
        const payload: Record<string, unknown> = {}
        for (const key of action.event.payloadKeys ?? []) {
          payload[key] = ctx.record?.[key] ?? ctx.params?.[key]
        }
        this.options.eventBus.emit(action.event.name, payload)
        return
      }
      case 'openDrawer':
      case 'openModal': {
        this.options.openOverlay?.(action, ctx)
        return
      }
      case 'download': {
        await this.executeRequest(action, ctx)
        return
      }
      default:
        throw new Error(`Unsupported action type: ${action.type}`)
    }
  }

  private async executeRequest(action: ActionSchema, ctx: ActionContext): Promise<void> {
    const request = action.request
    if (!request?.endpointId) throw new Error(`Action "${action.id}" missing endpointId`)

    const pathParams: Record<string, unknown> = {}
    for (const key of request.pathParams ?? []) {
      pathParams[key] = ctx.record?.[key] ?? ctx.params?.[key]
    }
    const queryParams: Record<string, unknown> = {}
    for (const key of request.queryParams ?? []) {
      queryParams[key] = ctx.record?.[key] ?? ctx.params?.[key]
    }

    // 经 DataSourceManager 的 endpoint 白名单解析路径，杜绝任意 URL
    const endpoint = this.options.dataSources.resolveEndpoint(request.endpointId)
    if (!endpoint) throw new Error(`Unknown endpointId "${request.endpointId}"`)

    let path = endpoint.path
    for (const [key, value] of Object.entries(pathParams)) {
      path = path.replace(`:${key}`, encodeURIComponent(String(value ?? '')))
    }

    const method = request.method ?? 'POST'
    const config = Object.keys(queryParams).length > 0 ? ({ params: queryParams } as any) : undefined
    const body = ctx.record ?? ctx.params
    switch (method) {
      case 'GET':
        await this.options.apiClient.get(path, config)
        break
      case 'POST':
        await this.options.apiClient.post(path, body, config)
        break
      case 'PUT':
        await this.options.apiClient.put(path, body, config)
        break
      case 'PATCH':
        await this.options.apiClient.patch(path, body, config)
        break
      case 'DELETE':
        await this.options.apiClient.delete(path, config)
        break
    }
  }

  private notifyResult(action: ActionSchema): void {
    if (action.result?.mode === 'silent') return
    const message = action.result?.successMessageKey ?? `操作已提交: ${action.id}`
    this.options.notify?.(message)
  }
}

export function createActionEngine(options: ActionEngineOptions): ActionEngine {
  return new ActionEngine(options)
}
