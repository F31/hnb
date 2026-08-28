export interface Cluster {
  id: string
  tenantId: string
  name: string
  displayName: string
  targetType: string
  distribution: string
  connectionType: string
  status: string
  labels: Record<string, string>
  isActive: boolean
  createdAt: string
}

export interface Agent {
  clusterId: string
  hostname: string
  status: string
  agentVersion: string
  nodeCount: number
  cpuCores: number
  memoryMb: number
}

export interface Extension {
  id: string
  name: string
  version: string
  providerType: string
  phase: string
  manifest: Record<string, any>
  createdAt: string
}

export interface AuditLog {
  id: string
  timestamp: string
  userId: string
  action: string
  resourceType: string
  method: string
  path: string
  statusCode: number
  durationMs: number
}

export interface Operation {
  id: string
  type: string
  status: 'queued' | 'pending_approval' | 'approved' | 'rejected' | 'running' | 'completed' | 'failed'
  tenantId: string
  workspaceId: string
  targetId: string
  createdAt: string
  updatedAt: string
}

export interface AlertRule {
  id: string
  name: string
  severity: 'critical' | 'warning' | 'info'
  enabled: boolean
  condition: string
  createdAt: string
}

export interface AlertEvent {
  id: string
  ruleId: string
  severity: string
  status: 'firing' | 'resolved' | 'acknowledged'
  message: string
  firedAt: string
}