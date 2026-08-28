/**
 * clusterMonitoringApi service adapter 单元测试。
 * 覆盖：生产路径空摘要、空序列（图表空态）、默认时间范围。
 */
import { describe, it, expect, vi, afterEach } from 'vitest'

const OLD_ENV = { ...import.meta.env }

afterEach(() => {
  vi.resetModules()
  Object.assign(import.meta.env, OLD_ENV)
})

/** 在与被测模块相同的模块实例上注入 mock client（afterEach 会 resetModules） */
async function injectMockClient(get: ReturnType<typeof vi.fn>) {
  const { setClusterApiClient } = await import('../api/clusterApi')
  setClusterApiClient({ get, post: vi.fn(), patch: vi.fn(), delete: vi.fn() } as never)
}

describe('clusterMonitoringApi', () => {
  it('生产路径（无 fixture 标志）返回空摘要', async () => {
    const get = vi.fn().mockResolvedValue({
      alerts: { critical: 0, major: 0, minor: 0, warning: 0, event: 0 },
      namespaceCount: 0,
      projectCount: 0,
      schedulableNodeCount: 0,
      cpu: { total: 0, usagePercent: 0, used: 0, allocationPercent: 0, allocated: 0, overcommitPercent: 0, overcommitted: 0 },
      memory: { total: 0, usagePercent: 0, used: 0, allocationPercent: 0, allocated: 0, overcommitPercent: 0, overcommitted: 0 },
    })
    await injectMockClient(get)
    const mod = await import('../api/clusterMonitoringApi')
    const summary = await mod.getClusterMonitoringSummary('c1')
    expect(get).toHaveBeenCalledWith('/api/v1/resources/clusters/c1/monitoring/summary')
    expect(summary.alerts).toEqual({ critical: 0, major: 0, minor: 0, warning: 0, event: 0 })
    expect(summary.cpu.total).toBe(0)
  })

  it('生产路径返回空序列（图表空态）', async () => {
    const emptySeries = {
      cpuUsage: { name: 'cpuUsage', unit: '%', points: [] },
      memoryUsage: { name: 'memoryUsage', unit: '%', points: [] },
      gpuUsage: { name: 'gpuUsage', unit: '%', points: [] },
      vramUsage: { name: 'vramUsage', unit: '%', points: [] },
    }
    const get = vi.fn().mockResolvedValue(emptySeries)
    await injectMockClient(get)
    const mod = await import('../api/clusterMonitoringApi')
    const range = { start: '2026-08-06T00:00:00Z', end: '2026-08-06T01:00:00Z' }
    const series = await mod.getClusterMonitoringMetrics('c1', range)
    expect(get).toHaveBeenCalledWith('/api/v1/resources/clusters/c1/monitoring/metrics', { params: range })
    expect(series.cpuUsage.points).toHaveLength(0)
    expect(series.memoryUsage.points).toHaveLength(0)
    expect(series.vramUsage.points).toHaveLength(0)
  })

  it('默认时间范围为最近 1 小时', async () => {
    const mod = await import('../api/clusterMonitoringApi')
    const range = mod.defaultMonitoringRange()
    const spanMs = new Date(range.end).getTime() - new Date(range.start).getTime()
    expect(spanMs).toBe(60 * 60 * 1000)
  })
})
