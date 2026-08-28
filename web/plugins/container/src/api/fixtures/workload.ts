/**
 * 容器-工作负载 fixture（Deployment / StatefulSet / DaemonSet / Job / CronJob / Pod）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 * 结构与 K8s List 的 items 项一致，供 Workloads.vue 的 normalizeItem 消费。
 */

const now = new Date().toISOString()

export const workloadsFixture: Record<string, Record<string, unknown>[]> = {
  deployment: [
    {
      metadata: { uid: 'dep-1', name: 'web-frontend', namespace: 'default', creationTimestamp: '2026-08-01T02:00:00Z' },
      spec: { replicas: 3, template: { spec: { containers: [{ image: 'nginx:1.25' }] } } },
      status: { availableReplicas: 3, conditions: [{ type: 'Available', status: 'True' }] },
    },
    {
      metadata: { uid: 'dep-2', name: 'api-gateway', namespace: 'default', creationTimestamp: '2026-08-02T03:30:00Z' },
      spec: { replicas: 2, template: { spec: { containers: [{ image: 'nginx:1.25' }, { image: 'envoy:1.30' }] } } },
      status: { availableReplicas: 1, conditions: [{ type: 'Available', status: 'False' }] },
    },
    {
      metadata: { uid: 'dep-3', name: 'redis', namespace: 'rd', creationTimestamp: '2026-07-20T01:00:00Z' },
      spec: { replicas: 1, template: { spec: { containers: [{ image: 'redis:7.2' }] } } },
      status: { availableReplicas: 1, conditions: [{ type: 'Available', status: 'True' }] },
    },
  ],
  statefulset: [
    {
      metadata: { uid: 'sts-1', name: 'etcd', namespace: 'infra', creationTimestamp: '2026-07-19T05:00:00Z' },
      spec: { replicas: 3, template: { spec: { containers: [{ image: 'etcd:3.5' }] } } },
      status: { readyReplicas: 3 },
    },
    {
      metadata: { uid: 'sts-2', name: 'kafka', namespace: 'infra', creationTimestamp: '2026-07-21T06:00:00Z' },
      spec: { replicas: 3, template: { spec: { containers: [{ image: 'kafka:3.6' }] } } },
      status: { readyReplicas: 2 },
    },
  ],
  daemonset: [
    {
      metadata: { uid: 'ds-1', name: 'node-exporter', namespace: 'monitoring', creationTimestamp: '2026-07-18T08:00:00Z' },
      spec: { template: { spec: { containers: [{ image: 'prom/node-exporter:1.7' }] } } },
      status: { desiredNumberScheduled: 5, currentNumberScheduled: 5 },
    },
    {
      metadata: { uid: 'ds-2', name: 'fluentd', namespace: 'logging', creationTimestamp: '2026-07-18T09:00:00Z' },
      spec: { template: { spec: { containers: [{ image: 'fluent/fluentd:v1.16' }] } } },
      status: { desiredNumberScheduled: 5, currentNumberScheduled: 4 },
    },
  ],
  job: [
    {
      metadata: { uid: 'job-1', name: 'db-migration', namespace: 'default', creationTimestamp: '2026-08-03T12:00:00Z' },
      spec: { completions: 1, template: { spec: { containers: [{ image: 'alpine:3.19' }] } } },
      status: { succeeded: 1, startTime: '2026-08-03T12:00:00Z', completionTime: '2026-08-03T12:00:45Z' },
    },
    {
      metadata: { uid: 'job-2', name: 'seed-data', namespace: 'default', creationTimestamp: '2026-08-03T13:00:00Z' },
      spec: { completions: 1, template: { spec: { containers: [{ image: 'alpine:3.19' }] } } },
      status: { succeeded: 0, failed: 1, startTime: '2026-08-03T13:00:00Z' },
    },
  ],
  cronjob: [
    {
      metadata: { uid: 'cj-1', name: 'daily-backup', namespace: 'default', creationTimestamp: '2026-07-25T00:00:00Z' },
      spec: { schedule: '0 2 * * *', template: { spec: { containers: [{ image: 'busybox:1.36' }] } } },
      status: { lastScheduleTime: '2026-08-09T02:00:00Z' },
    },
    {
      metadata: { uid: 'cj-2', name: 'log-cleaner', namespace: 'logging', creationTimestamp: '2026-07-25T00:30:00Z' },
      spec: { schedule: '*/30 * * * *', template: { spec: { containers: [{ image: 'busybox:1.36' }] } } },
      status: { lastScheduleTime: '2026-08-09T01:30:00Z' },
    },
  ],
  pod: [
    {
      metadata: { uid: 'pod-1', name: 'web-frontend-7b8f-abcde', namespace: 'default', creationTimestamp: '2026-08-01T02:00:00Z' },
      spec: { nodeName: 'node-01', containers: [{ image: 'nginx:1.25' }] },
      status: { phase: 'Running', containerStatuses: [{ restartCount: 0 }] },
    },
    {
      metadata: { uid: 'pod-2', name: 'api-gateway-6d9f-xyz', namespace: 'default', creationTimestamp: '2026-08-02T03:30:00Z' },
      spec: { nodeName: 'node-02', containers: [{ image: 'nginx:1.25' }] },
      status: { phase: 'Pending', containerStatuses: [{ restartCount: 2 }] },
    },
    {
      metadata: { uid: 'pod-3', name: 'redis-pod-1', namespace: 'rd', creationTimestamp: '2026-07-20T01:00:00Z' },
      spec: { nodeName: 'node-01', containers: [{ image: 'redis:7.2' }] },
      status: { phase: 'Running', containerStatuses: [{ restartCount: 0 }] },
    },
  ],
}

export const workloadCreatedAt = now