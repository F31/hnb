import { describe, expect, it } from 'vitest'
import { parseKubeSummary, type KubeSummary } from '../utils/kubeconfig'

const VALID_KUBECONFIG = `apiVersion: v1
kind: Config
current-context: prod-ctx
clusters:
  - name: prod-cluster
    cluster:
      server: https://10.0.0.10:6443
  - name: staging-cluster
    cluster:
      server: https://10.0.0.11:6443
contexts:
  - name: prod-ctx
    context:
      cluster: prod-cluster
      user: admin
  - name: staging-ctx
    context:
      cluster: staging-cluster
      user: admin
users:
  - name: admin
    user:
      token: REDACTED-SHOULD-NOT-LEAK
`

describe('parseKubeSummary', () => {
  it('解析 current-context 命中的目标集群与 server，且不泄露敏感字段', () => {
    const summary: KubeSummary = parseKubeSummary(VALID_KUBECONFIG)
    expect(summary.recognizable).toBe(true)
    expect(summary.structurallyValid).toBe(true)
    expect(summary.currentContext).toBe('prod-ctx')
    expect(summary.targetCluster).toEqual({ name: 'prod-cluster', server: 'https://10.0.0.10:6443' })
    expect(summary.clusters).toHaveLength(2)
    // 绝不返回 token / 证书 / key 明文
    expect(JSON.stringify(summary)).not.toContain('REDACTED-SHOULD-NOT-LEAK')
    expect(JSON.stringify(summary)).not.toContain('token')
  })

  it('无 current-context 时取第一个带 server 的 cluster 作为目标', () => {
    const text = `apiVersion: v1
kind: Config
clusters:
  - name: only
    cluster: { server: https://10.0.0.99:6443 }
contexts:
  - name: only-ctx
    context: { cluster: only, user: u }
users:
  - name: u
    user: { token: x }
`
    const summary = parseKubeSummary(text)
    expect(summary.recognizable).toBe(true)
    expect(summary.targetCluster?.name).toBe('only')
    expect(summary.targetCluster?.server).toBe('https://10.0.0.99:6443')
  })

  it('空文本返回不可识别且无异常', () => {
    const summary = parseKubeSummary('')
    expect(summary.recognizable).toBe(false)
    expect(summary.structurallyValid).toBe(false)
  })

  it('非法 YAML 返回不可识别与错误摘要，不抛出', () => {
    const summary = parseKubeSummary('apiVersion: v1\nclusters: [unclosed')
    expect(summary.recognizable).toBe(false)
    expect(summary.errors.length).toBeGreaterThan(0)
    expect(summary.structurallyValid).toBe(false)
  })

  it('结构不完整（缺 users/contexts）标记为 structurallyValid=false', () => {
    const summary = parseKubeSummary(`apiVersion: v1
kind: Config
clusters:
  - name: c
    cluster: { server: https://1.2.3.4:6443 }
`)
    expect(summary.structurallyValid).toBe(false)
    expect(summary.errors).toContain('missing-contexts')
    expect(summary.errors).toContain('missing-users')
  })
})
