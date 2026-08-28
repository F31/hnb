import { describe, expect, it, vi } from 'vitest'
import { filterAndSortEvents, listWorkloadEvents, mapWorkloadEvent, setContainerEventsClient } from '../eventsApi'

function mockClient(get: ReturnType<typeof vi.fn>) {
  return { get, post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() } as any
}

describe('eventsApi', () => {
  it('maps core Kubernetes Events using the latest available timestamp', () => {
    expect(mapWorkloadEvent({
      metadata: { uid: 'event-a', creationTimestamp: '2026-08-09T09:00:00Z' }, involvedObject: { kind: 'Pod', name: 'api-123' },
      type: 'Warning', reason: 'BackOff', message: 'Back-off restarting failed container', series: { lastObservedTime: '2026-08-09T10:00:00Z' },
    })).toEqual({ id: 'event-a', updatedAt: '2026-08-09T10:00:00Z', type: 'Warning', object: 'Pod/api-123', reason: 'BackOff', message: 'Back-off restarting failed container' })
  })

  it('filters workload and Pod objects and sorts newest first', () => {
    const items = [
      { id: 'old', updatedAt: '2026-08-09T09:00:00Z', type: 'Normal', object: 'Deployment/api', reason: 'Scaled', message: '' },
      { id: 'other', updatedAt: '2026-08-09T11:00:00Z', type: 'Warning', object: 'Pod/other', reason: 'Failed', message: '' },
      { id: 'new', updatedAt: '2026-08-09T10:00:00Z', type: 'Normal', object: 'Pod/api-123', reason: 'Pulled', message: '' },
    ]
    expect(filterAndSortEvents(items, ['api', 'api-123']).map((item) => item.id)).toEqual(['new', 'old'])
  })

  it('loads namespace Events through the Kubernetes proxy', async () => {
    const get = vi.fn().mockResolvedValue({ items: [{ metadata: { uid: 'one' }, involvedObject: { kind: 'Deployment', name: 'api' }, lastTimestamp: '2026-08-09T10:00:00Z' }] })
    setContainerEventsClient(mockClient(get))
    await expect(listWorkloadEvents('cluster-a', 'argocd', ['api'])).resolves.toHaveLength(1)
    expect(get).toHaveBeenCalledWith('/api/v1/proxy/cluster-a/api/v1/namespaces/argocd/events')
  })
})
