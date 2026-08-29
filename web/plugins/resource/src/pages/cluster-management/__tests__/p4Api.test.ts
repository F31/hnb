/**
 * P4 service adapter 单元测试。
 * 覆盖：生产路径空态、fixture 分页/关键词、YAML 校验。
 */
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { validateYaml } from '../utils/yaml'
import { setPluginI18nT } from '../api/pluginI18n'
import { setClusterApiClient } from '../api/clusterApi'
import type { ApiClient } from '@hnb/types'

const OLD_ENV = { ...import.meta.env }

beforeEach(() => {
  setPluginI18nT('resource', (key) => (key.endsWith('yamlInvalid') ? 'YAML 语法错误：' : key))
})

afterEach(() => {
  vi.resetModules()
  Object.assign(import.meta.env, OLD_ENV)
})

describe('p4Api 生产路径（无 fixture 标志）', () => {
  it('边缘节点组 / 租户分配 / 插件实例 返回空态', async () => {
    const mod = await import('../api/p4Api')
    expect(await mod.getEdgeNodeGroups('c1')).toEqual([])
    expect(await mod.getTenantAllocations('c1')).toEqual([])
    const inst = await mod.getPluginInstances('c1', { page: 1, pageSize: 10 })
    expect(inst.items).toEqual([])
  })

  it('漏洞库状态为 null', async () => {
    const mod = await import('../api/p4Api')
    expect(await mod.getVulnerabilityDbStatus()).toBeNull()
  })

  it('插件市场生产路径调用后端目录/安装/卸载端点（无 client 时报错）', async () => {
    const mod = await import('../api/p4Api')
    // 未注入 client → 拉目录应报未初始化，而非静默空态
    await expect(mod.getPluginMarketCatalog('c1')).rejects.toThrow(/client is not initialized/)
    // 未配置 client 时安装/卸载同样报未初始化（而非"未开放"）
    await expect(mod.installPlugin('hami', 'v2.10.0', 'c1')).rejects.toThrow(/client is not initialized/)
    await expect(mod.uninstallPlugin('hami', 'c1')).rejects.toThrow(/client is not initialized/)
  })

  it('插件市场接入后端端点（注入 mock client）', async () => {
    const get = vi.fn<ApiClient['get']>()
    const post = vi.fn<ApiClient['post']>()
    const del = vi.fn<ApiClient['delete']>()
    get.mockResolvedValue([
      { name: 'calico', version: 'v3.32.1', description: 'Calico CNI', category: '网络', installed: true },
    ])
    post.mockResolvedValue({ id: 'ext-1', status: 'pending' })
    del.mockResolvedValue({ status: 'uninstalled' })
    // 与被测模块同一实例注入（afterEach 会 resetModules）
    const { setClusterApiClient } = await import('../api/clusterApi')
    setClusterApiClient({ get, post, delete: del } as unknown as ApiClient)

    const mod = await import('../api/p4Api')
    const catalog = await mod.getPluginMarketCatalog('c1')
    expect(get).toHaveBeenCalledWith('/api/v1/plugin-catalog?clusterId=c1')
    expect(catalog[0]?.installed).toBe(true)

    await mod.installPlugin('hami', 'v2.10.0', 'c1')
    expect(post).toHaveBeenCalledWith('/api/v1/plugin-catalog/installs', {
      name: 'hami',
      version: 'v2.10.0',
      clusterId: 'c1',
    })

    await mod.uninstallPlugin('hami', 'c1')
    expect(del).toHaveBeenCalledWith('/api/v1/plugin-catalog/installs/hami?clusterId=c1')
  })
})

describe('YAML 校验', () => {
  it('非法 YAML 返回错误摘要', () => {
    const err = validateYaml('replicas: [')
    expect(err).toMatch(/YAML 语法错误/)
  })

  it('合法 YAML 返回 null，空文本返回 null', () => {
    expect(validateYaml('replicas: 1\nimagePullPolicy: IfNotPresent')).toBeNull()
    expect(validateYaml('')).toBeNull()
  })
})
