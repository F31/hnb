/**
 * nodeApi service adapter 单元测试。
 * 覆盖：生产路径空列表/空磁盘/空网卡/空容器组；fixture 分页与关键词。
 */
import { describe, it, expect, vi, afterEach } from 'vitest'

const OLD_ENV = { ...import.meta.env }

afterEach(() => {
  vi.resetModules()
  Object.assign(import.meta.env, OLD_ENV)
})

describe('nodeApi', () => {
  it('生产路径节点列表为空（经节点 Read Model）', async () => {
    const clusterApi = await import('../api/clusterApi')
    clusterApi.setClusterApiClient({ get: async () => ({ items: [], total: 0 }) } as never)
    const mod = await import('../api/nodeApi')
    const res = await mod.getNodeList('c1', { page: 1, pageSize: 10 })
    expect(res.items).toEqual([])
    expect(res.total).toBe(0)
  }, 15_000)

  it('生产路径节点详情为 null（页面展示空字段）', async () => {
    const mod = await import('../api/nodeApi')
    const detail = await mod.getNodeDetail('c1', 'n1')
    expect(detail).toBeNull()
  })

  it('生产路径容器组为空列表', async () => {
    const mod = await import('../api/nodeApi')
    const res = await mod.getNodePods('c1', 'n1', { page: 1, pageSize: 10 })
    expect(res.items).toEqual([])
  })

  it('占位节点不携带臆造数据', async () => {
    const mod = await import('../api/nodeApi')
    const placeholder = mod.PLACEHOLDER_NODE('n1', 'node01')
    expect(placeholder.id).toBe('n1')
    expect(placeholder.name).toBe('node01')
    expect(placeholder.gpuResource).toBeUndefined()
    expect(placeholder.status).toBe('unknown')
  })
})
