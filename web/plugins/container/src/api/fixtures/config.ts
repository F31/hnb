import type { ConfigMapItem, SecretItem } from '../configApi'

export const configMapsFixture: ConfigMapItem[] = [
  { name: 'argocd-cm', namespace: 'argocd', data: { 'application.instanceLabelKey': 'argocd.argoproj.io/instance', 'statusbadge.enabled': 'true' }, createdAt: '2026-08-01T08:00:00Z' },
  { name: 'argocd-rbac-cm', namespace: 'argocd', data: { 'policy.default': 'role:readonly', 'scopes': '[groups]' }, createdAt: '2026-08-01T08:05:00Z' },
  { name: 'application-settings', namespace: 'default', data: { LOG_LEVEL: 'info', REGION: 'cn-north-1' }, createdAt: '2026-08-02T09:00:00Z' },
]

export const secretsFixture: SecretItem[] = [
  { name: 'argocd-secret', namespace: 'argocd', type: 'Opaque', dataKeys: ['admin.password', 'server.secretkey'], createdAt: '2026-08-01T08:00:00Z', protected: true },
  { name: 'argocd-helm', namespace: 'argocd', type: 'Opaque', dataKeys: ['repository'], createdAt: '2026-08-01T08:02:00Z', protected: true },
  { name: 'registry-credentials', namespace: 'argocd', type: 'kubernetes.io/dockerconfigjson', dataKeys: ['.dockerconfigjson'], createdAt: '2026-08-03T10:00:00Z', protected: false },
  { name: 'application-secret', namespace: 'default', type: 'Opaque', dataKeys: ['username', 'password'], createdAt: '2026-08-04T11:00:00Z', protected: false },
]
