/**
 * clusterDetailApi service adapter 单元测试（restore-cluster-detail-console）。
 * 覆盖：后端投影→详情映射、节点映射、插件状态端点、fixture 数据完整性。
 */
import { describe, it, expect, vi } from 'vitest'
import {
  mapSummaryToDetail,
  mapNodeToSummary,
  formatCpuCores,
  formatMemoryGiB,
  getClusterPluginStatuses,
} from '../api/clusterDetailApi'
import { setClusterApiClient } from '../api/clusterApi'
import {
  clusterDetailFixture,
  clusterPluginStatusesFixture,
  clusterNodeSummaryFixture,
} from '../api/fixtures/clusterDetail'

describe('mapSummaryToDetail', () => {
  it('RUNNING 投影映射为 running 并填充可映射字段', () => {
    const detail = mapSummaryToDetail({
      clusterId: 'c1',
      displayName: 'graphify',
      runtimeVersion: 'v1.31.1',
      kind: 'kubernetes',
      createdAt: '2026-08-05T12:45:45Z',
      status: 'RUNNING',
    })
    expect(detail.id).toBe('c1')
    expect(detail.name).toBe('graphify')
    expect(detail.kubernetesVersion).toBe('v1.31.1')
    expect(detail.clusterType).toBe('kubernetes')
    expect(detail.status).toBe('running')
  })

  it('STALE/DEGRADED 映射为 abnormal，未知为 unknown', () => {
    expect(mapSummaryToDetail({ status: 'STALE' }).status).toBe('abnormal')
    expect(mapSummaryToDetail({ status: 'DEGRADED' }).status).toBe('abnormal')
    expect(mapSummaryToDetail({ status: 'PROVISIONING' }).status).toBe('unknown')
  })

  it('缺失字段默认空串（展示层显示 --）', () => {
    const detail = mapSummaryToDetail({ clusterId: 'c1' })
    expect(detail.osVersion).toBe('')
    expect(detail.controlPlaneSchedulingEnabled).toBe(false)
  })
})

describe('mapNodeToSummary', () => {
  it('把节点 Read Model 映射为 NodeSummary（含 GPU 缺省 /）', () => {
    const summary = mapNodeToSummary({
      nodeId: 'n1',
      name: 'node01',
      role: 'worker',
      status: 'Ready',
      ipAddress: '10.0.0.1',
      os: 'ubuntu',
      arch: 'amd64',
      cpuAllocatable: '16',
      memoryAllocatable: '32Gi',
      kubeletVersion: 'v1.30.0',
      lastHeartbeatAt: '2026-08-06T00:00:00Z',
    } as never)
    expect(summary.id).toBe('n1')
    expect(summary.cpuCores).toBe(16)
    expect(summary.memoryGiB).toBe(32)
    expect(summary.gpuResource).toBeUndefined()
  })
})

describe('格式化器', () => {
  it('CPU 与内存格式化', () => {
    expect(formatCpuCores(96)).toBe('96')
    expect(formatMemoryGiB(251.03)).toBe('251.03 GiB')
  })
})

describe('getClusterPluginStatuses', () => {
  it('请求 plugins 端点并映射 items 列表', async () => {
    const get = vi.fn().mockResolvedValue({
      items: [
        { key: 'cni/calico', displayName: 'Calico网络插件', status: 'installed' },
        { key: 'csi/rbd', displayName: 'RBD块存储', status: 'not-installed' },
      ],
      observedAt: '2026-08-21T00:00:00Z',
    })
    setClusterApiClient({ get, post: vi.fn(), patch: vi.fn(), delete: vi.fn() } as never)
    const res = await getClusterPluginStatuses('c1')
    expect(get).toHaveBeenCalledWith('/api/v1/resources/clusters/c1/plugins')
    expect(res).toHaveLength(2)
    expect(res[0]).toEqual({ key: 'cni/calico', displayName: 'Calico网络插件', status: 'installed' })
    expect(res[1].status).toBe('not-installed')
  })

  it('后端无能力快照时返回空列表（前端展示空态，不回退 fixture）', async () => {
    const get = vi.fn().mockResolvedValue({ items: [], observedAt: null })
    setClusterApiClient({ get, post: vi.fn(), patch: vi.fn(), delete: vi.fn() } as never)
    const res = await getClusterPluginStatuses('c2')
    expect(res).toEqual([])
  })
})

describe('fixtures', () => {
  it('插件状态含运行中与未安装', () => {
    const running = clusterPluginStatusesFixture.filter((s) => s.status === 'running')
    const notInstalled = clusterPluginStatusesFixture.filter((s) => s.status === 'not-installed')
    expect(running.length).toBeGreaterThan(0)
    expect(notInstalled.length).toBeGreaterThan(0)
    expect(clusterPluginStatusesFixture.length).toBe(18)
  })

  it('节点摘要包含 controller / worker / GPU worker', () => {
    const roles = clusterNodeSummaryFixture.map((n) => n.role)
    expect(roles).toContain('controller')
    expect(roles).toContain('worker')
    expect(clusterNodeSummaryFixture.some((n) => n.gpuResource)).toBe(true)
  })

  it('集群详情 fixture 完整', () => {
    expect(clusterDetailFixture.kubernetesVersion).toBe('v1.31.1')
    expect(clusterDetailFixture.podCidr).toBeTruthy()
  })
})
