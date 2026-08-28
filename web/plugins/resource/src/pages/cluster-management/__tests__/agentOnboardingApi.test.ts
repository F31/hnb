import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import { getAgentOnboarding, setAgentOnboardingApiClient } from '../api/agentOnboardingApi'
import { stubApiClient } from './testUtils'

describe('agentOnboardingApi', () => {
  let client: ApiClient & { post: ReturnType<typeof vi.fn> }

  beforeEach(() => {
    const post = vi.fn(async <T,>(_url: string, _data?: unknown) => ({
      clusterId: 'c-1',
      tenantId: 'tenant-a',
      displayName: 'Prod Cluster',
      tunnelUrl: 'wss://hnb.example.com/tunnel',
      token: 'jwt-token',
      tokenExpiry: '2026-08-05T00:00:00Z',
      namespace: 'hnb-agent-tenant-a',
      manifest: 'apiVersion: v1\nkind: Namespace\n',
      installCommand: 'kubectl apply -f - <<EOF\n',
    }) as T)
    client = stubApiClient({ post: post as unknown as ApiClient['post'] }) as unknown as ApiClient & {
      post: ReturnType<typeof vi.fn>
    }
    setAgentOnboardingApiClient(client)
  })

  it('POST 到 BFF 的 agent-onboarding 端点（路径含集群 ID）', async () => {
    const res = await getAgentOnboarding('515eba09-0a41-5b92-b972-69af1f0f655c')
    expect(client.post).toHaveBeenCalledTimes(1)
    const [path] = client.post.mock.calls[0] as [string]
    expect(String(path)).toBe('/api/v1/resources/clusters/515eba09-0a41-5b92-b972-69af1f0f655c/agent-onboarding')
    expect(res.tunnelUrl).toBe('wss://hnb.example.com/tunnel')
    expect(res.manifest).toContain('kind: Namespace')
  })

  it('集群 ID 经 encodeURIComponent 转义', async () => {
    await getAgentOnboarding('a/b')
    const [path] = client.post.mock.calls[0] as [string]
    expect(String(path)).toBe('/api/v1/resources/clusters/a%2Fb/agent-onboarding')
  })
})
