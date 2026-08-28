import type { WorkloadEvent } from '../eventsApi'

export const workloadEventsFixture: Array<{ namespace: string; event: WorkloadEvent }> = [
  { namespace: 'argocd', event: { id: 'event-1', updatedAt: '2026-08-09T10:15:30Z', type: 'Normal', object: 'Pod/argocd-dex-server-6f48c9-x1', reason: 'Scheduled', message: 'Successfully assigned argocd/argocd-dex-server-6f48c9-x1 to worker-01' } },
  { namespace: 'argocd', event: { id: 'event-2', updatedAt: '2026-08-09T10:15:34Z', type: 'Normal', object: 'Pod/argocd-dex-server-6f48c9-x1', reason: 'Pulled', message: 'Container image was already present on machine' } },
  { namespace: 'argocd', event: { id: 'event-3', updatedAt: '2026-08-09T10:15:36Z', type: 'Normal', object: 'Deployment/argocd-dex-server', reason: 'ScalingReplicaSet', message: 'Scaled up replica set argocd-dex-server-6f48c9 to 1' } },
  { namespace: 'argocd', event: { id: 'event-4', updatedAt: '2026-08-09T10:16:12Z', type: 'Warning', object: 'Pod/argocd-dex-server-6f48c9-x1', reason: 'Unhealthy', message: 'Readiness probe failed: connection refused' } },
  { namespace: 'argocd', event: { id: 'event-other', updatedAt: '2026-08-09T10:17:00Z', type: 'Warning', object: 'Pod/argocd-server-other', reason: 'BackOff', message: 'Back-off restarting failed container' } },
]
