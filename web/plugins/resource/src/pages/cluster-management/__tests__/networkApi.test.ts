/**
 * networkApi service adapter 单元测试（生产路径空态 + 写操作抛未开放）。
 */
import { describe, it, expect, vi, afterEach } from 'vitest'

const OLD_ENV = { ...import.meta.env }

afterEach(() => {
  vi.resetModules()
  Object.assign(import.meta.env, OLD_ENV)
})

describe('networkApi 生产路径（无 fixture 标志）', () => {
  it('三类列表均返回空态', async () => {
    const mod = await import('../api/networkApi')
    expect(await mod.getSubnets()).toEqual([])
    expect(await mod.getIpUsageStats()).toEqual([])
    expect(await mod.getSubnetRequests()).toEqual([])
  })

  it('写操作抛未开放', async () => {
    const mod = await import('../api/networkApi')
    await expect(mod.createSubnet({} as never)).rejects.toThrow(/Unavailable/)
    await expect(mod.deleteSubnet('x')).rejects.toThrow(/Unavailable/)
    await expect(mod.approveSubnetRequest({} as never)).rejects.toThrow(/Unavailable/)
    await expect(mod.rejectSubnetRequest({} as never)).rejects.toThrow(/Unavailable/)
  })
})
