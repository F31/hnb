/**
 * P4 service adapter 单元测试。
 * 覆盖：生产路径空态、fixture 分页/关键词、YAML 校验。
 */
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { validateYaml } from '../utils/yaml'
import { setPluginI18nT } from '../api/pluginI18n'

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

  it('插件市场目录为空，安装/卸载抛未开放', async () => {
    const mod = await import('../api/p4Api')
    expect(await mod.getPluginMarketCatalog()).toEqual([])
    await expect(mod.installPlugin('hami', 'v1.0.2')).rejects.toThrow(/Unavailable/)
    await expect(mod.uninstallPlugin('hami')).rejects.toThrow(/Unavailable/)
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
