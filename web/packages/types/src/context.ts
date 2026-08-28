export interface HNBContext {
  tenantId?: string
  spaceId?: string
  locale?: string
  environmentId?: string
  clusterId?: string
}

export interface Workspace {
  id: string
  name: string
  displayName: string
  tenantId: string
  isActive: boolean
  createdAt: string
  updatedAt: string
}

export interface Project {
  id: string
  workspaceId: string
  name: string
  displayName: string
  isActive: boolean
  createdAt: string
  updatedAt: string
}

export interface Environment {
  id: string
  projectId: string
  name: string
  type: string
  isActive: boolean
  createdAt: string
  updatedAt: string
}

export type EnvironmentType = 'development' | 'testing' | 'staging' | 'production'
