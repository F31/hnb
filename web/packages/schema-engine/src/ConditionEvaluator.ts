/**
 * ConditionEvaluator — 受控条件 DSL 求值（V2.5 §4.3 / V2.6 §4.4）。
 *
 * V2.6 约束：
 *  - 页面渲染周期内复用单例实例，避免每次条件求值创建新对象；
 *  - 上下文可以以 getter 形式注入：每次求值读取最新上下文
 *    （如 props.conditionContext），权限/Feature 异步就绪后条件可重算；
 *  - 已移除的条件字段（role / exists / fieldValue / notEmpty /
 *    resourceState）被忽略并记录一次调试日志，不拒绝渲染。
 */
import type { Condition, ConditionTerm } from './types'

export interface ConditionContext {
  permissions?: Set<string> | string[]
  capabilities?: Set<string> | string[]
  features?: Set<string> | string[]
  licenses?: Set<string> | string[]
  context?: Record<string, unknown>
}

export type ConditionContextSource = ConditionContext | (() => ConditionContext)

function has(values: Set<string> | string[] | undefined, value: string): boolean {
  if (!value) return true
  if (!values) return false
  return Array.isArray(values) ? values.includes(value) : values.has(value)
}

export class ConditionEvaluator {
  /** V2.6 §4.3：允许的条件类型；其余视为已移除字段，忽略并记录调试日志 */
  private static readonly ALLOWED_TERM_KEYS = new Set([
    'permission',
    'feature',
    'license',
    'capability',
    'context',
  ])

  private loggedDeprecated = new Set<string>()

  constructor(private ctx: ConditionContextSource = {}) {}

  /** 每次求值解析当前上下文：getter 形式支持响应式最新值 */
  private current(): ConditionContext {
    return typeof this.ctx === 'function' ? this.ctx() : this.ctx
  }

  evaluate(condition?: Condition): boolean {
    if (!condition) return true
    const all = condition.all ?? []
    const any = condition.any ?? []
    if (all.length > 0 && !all.every((term) => this.evaluateTerm(term))) return false
    if (any.length > 0 && !any.some((term) => this.evaluateTerm(term))) return false
    return true
  }

  private evaluateTerm(term: ConditionTerm): boolean {
    for (const key of Object.keys(term)) {
      if (!ConditionEvaluator.ALLOWED_TERM_KEYS.has(key) && !this.loggedDeprecated.has(key)) {
        this.loggedDeprecated.add(key)
        console.debug(`[ConditionEvaluator] ignoring deprecated condition field "${key}"`)
      }
    }
    const ctx = this.current()
    if (term.permission && !has(ctx.permissions, term.permission)) return false
    if (term.capability && !has(ctx.capabilities, term.capability)) return false
    if (term.feature && !has(ctx.features, term.feature)) return false
    if (term.license && !has(ctx.licenses, term.license)) return false
    if (term.context && !ctx.context?.[term.context]) return false
    return true
  }
}

export function createConditionEvaluator(
  ctx?: ConditionContextSource,
): ConditionEvaluator {
  return new ConditionEvaluator(ctx)
}
