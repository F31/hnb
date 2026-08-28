import type { LogWorkload } from '../logsApi'

export const logWorkloadsFixture: LogWorkload[] = [
  {
    name: 'web-frontend', namespace: 'default', type: 'deployment', selector: { app: 'web-frontend' },
    pods: [
      { name: 'web-frontend-7b8f-abcde', containers: ['web', 'metrics'] },
      { name: 'web-frontend-7b8f-fghij', containers: ['web', 'metrics'] },
    ],
  },
  {
    name: 'api-gateway', namespace: 'default', type: 'deployment', selector: { app: 'api-gateway' },
    pods: [{ name: 'api-gateway-6d9f-xyz12', containers: ['gateway', 'envoy-sidecar'] }],
  },
  {
    name: 'redis', namespace: 'default', type: 'statefulset', selector: { app: 'redis' },
    pods: [{ name: 'redis-0', containers: ['redis', 'redis-exporter'] }],
  },
  {
    name: 'node-exporter', namespace: 'default', type: 'daemonset', selector: { app: 'node-exporter' },
    pods: [{ name: 'node-exporter-node-01', containers: ['node-exporter'] }],
  },
  {
    name: 'daily-backup', namespace: 'default', type: 'cronjob', selector: { job: 'daily-backup' },
    pods: [{ name: 'daily-backup-29100300-abcd', containers: ['backup'] }],
  },
  {
    name: 'argocd-server', namespace: 'argocd', type: 'deployment', selector: { app: 'argocd-server' },
    pods: [{ name: 'argocd-server-7f56cd9f-x1', containers: ['argocd-server'] }],
  },
  {
    name: 'argocd-dex-server', namespace: 'argocd', type: 'deployment', selector: { app: 'argocd-dex-server' },
    pods: [{ name: 'argocd-dex-server-6f48c9-x1', containers: ['dex'] }],
  },
]

export function fixtureLogText(namespace: string, pod: string, container: string, tailLines: number): string {
  const levels = ['INFO', 'INFO', 'DEBUG', 'WARN']
  const count = Math.min(tailLines, 500)
  return Array.from({ length: count }, (_, index) => {
    const timestamp = new Date(Date.UTC(2026, 7, 9, 8, 0, index % 60)).toISOString()
    return `${timestamp} ${levels[index % levels.length]} [${namespace}/${pod}/${container}] request=${String(index + 1).padStart(4, '0')} status=ok duration_ms=${8 + (index % 25)}`
  }).join('\n')
}
