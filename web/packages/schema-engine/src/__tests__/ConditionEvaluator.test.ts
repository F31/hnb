import { describe, expect, it, vi } from 'vitest'
import { ConditionEvaluator } from '../ConditionEvaluator'

describe('ConditionEvaluator', () => {
  it('passes when all controlled inputs match', () => {
    const evaluator = new ConditionEvaluator({
      permissions: ['schema:read'],
      capabilities: ['cluster.list'],
      features: ['schema-runtime'],
      licenses: ['enterprise'],
      context: { tenantId: 'tenant-a' },
    })

    expect(evaluator.evaluate({
      all: [
        { permission: 'schema:read' },
        { capability: 'cluster.list' },
        { feature: 'schema-runtime' },
        { license: 'enterprise' },
        { context: 'tenantId' },
      ],
    })).toBe(true)
  })

  it('fails closed when required input is missing', () => {
    const evaluator = new ConditionEvaluator({ permissions: [] })

    expect(evaluator.evaluate({ all: [{ permission: 'schema:read' }] })).toBe(false)
    expect(evaluator.evaluate({ any: [{ capability: 'cluster.list' }] })).toBe(false)
  })

  it('getter 上下文每次求值读取最新值（V2.6 §4.4 实例复用 + 响应式）', () => {
    let permissions: string[] = []
    const evaluator = new ConditionEvaluator(() => ({ permissions }))

    expect(evaluator.evaluate({ all: [{ permission: 'cluster:update' }] })).toBe(false)
    // 权限异步就绪后，同一实例无需重建即可得出新结果
    permissions = ['cluster:update']
    expect(evaluator.evaluate({ all: [{ permission: 'cluster:update' }] })).toBe(true)
  })

  it('已移除条件字段被忽略并记录调试日志（V2.6 §4.3）', () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const evaluator = new ConditionEvaluator({ permissions: ['schema:read'] })

    // resourceState / role 等已移除字段不影响求值结果
    expect(evaluator.evaluate({
      all: [
        { permission: 'schema:read' },
        { resourceState: ['RUNNING'] } as any,
      ],
    })).toBe(true)
    expect(debug).toHaveBeenCalledWith(
      expect.stringContaining('ignoring deprecated condition field "resourceState"'),
    )
    debug.mockRestore()
  })
})
