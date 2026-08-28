import { describe, expect, it, vi } from 'vitest'
import { extractPodContainers, fetchContainerLogs, selectorString, setContainerLogsClient } from '../logsApi'

describe('logsApi', () => {
  it('builds Kubernetes label selectors', () => {
    expect(selectorString({ app: 'web', component: 'api' })).toBe('app=web,component=api')
  })

  it('extracts init, regular and ephemeral containers', () => {
    expect(extractPodContainers({ spec: {
      initContainers: [{ name: 'init-db' }], containers: [{ name: 'api' }, { name: 'sidecar' }], ephemeralContainers: [{ name: 'debugger' }],
    } })).toEqual(['init-db', 'api', 'sidecar', 'debugger'])
  })

  it('requests plain-text logs with container and tail query parameters', async () => {
    const requestRaw = vi.fn().mockResolvedValue(new Response('line one\nline two\n', { status: 200, headers: { 'Content-Type': 'text/plain' } }))
    setContainerLogsClient({
      requestRaw,
      get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn(),
    })
    await expect(fetchContainerLogs({ clusterId: 'cluster-a', namespace: 'default', pod: 'pod-a', container: 'api', tailLines: 200 }))
      .resolves.toBe('line one\nline two\n')
    expect(requestRaw).toHaveBeenCalledWith(
      'GET',
      '/api/v1/proxy/cluster-a/api/v1/namespaces/default/pods/pod-a/log',
      undefined,
      expect.objectContaining({ headers: { Accept: '*/*' }, params: expect.objectContaining({ container: 'api', tailLines: 200, follow: false }) }),
    )
  })
})
