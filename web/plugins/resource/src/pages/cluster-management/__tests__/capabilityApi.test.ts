/**
 * capabilityApi service adapter 单元测试。
 * 覆盖：能力级别判定、cniHasCapability、矩阵查询。
 */
import { describe, it, expect } from 'vitest'
import { cniCapabilityMatricesFixture } from '../api/fixtures/capability'
import { matrixForCni, cniHasCapability } from '../api/capabilityApi'
import { isCniCapabilityAvailable } from '../types/capability'

describe('capability 能力判定', () => {
  it('可用级别判定（strong/medium/weak 可用，none 不可用）', () => {
    expect(isCniCapabilityAvailable('strong')).toBe(true)
    expect(isCniCapabilityAvailable('medium')).toBe(true)
    expect(isCniCapabilityAvailable('weak')).toBe(true)
    expect(isCniCapabilityAvailable('none')).toBe(false)
    expect(isCniCapabilityAvailable(undefined)).toBe(false)
  })

  it('Kube-OVN 支持 RDMA 与子网隔离，不支持异常检测', () => {
    const kubeOvn = matrixForCni(cniCapabilityMatricesFixture, 'kube-ovn')
    expect(kubeOvn).not.toBeNull()
    expect(cniHasCapability(kubeOvn, 'rdma')).toBe(true)
    expect(cniHasCapability(kubeOvn, 'subnetIsolation')).toBe(true)
    expect(cniHasCapability(kubeOvn, 'networkAnomalyDetection')).toBe(false)
  })

  it('Cilium 支持 QoS 与异常检测；Calico 不支持 RDMA', () => {
    const cilium = matrixForCni(cniCapabilityMatricesFixture, 'cilium')
    expect(cniHasCapability(cilium, 'qosBandwidth')).toBe(true)
    expect(cniHasCapability(cilium, 'networkAnomalyDetection')).toBe(true)
    const calico = matrixForCni(cniCapabilityMatricesFixture, 'calico')
    expect(cniHasCapability(calico, 'rdma')).toBe(false)
  })

  it('未收录 CNI / 空矩阵返回不可用', () => {
    expect(cniHasCapability(null, 'networkPolicy')).toBe(false)
    expect(matrixForCni([], 'kube-ovn')).toBeNull()
  })
})
