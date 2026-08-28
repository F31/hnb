/**
 * Operation Center 列表 PageSchema（L2 Schema，V2.5 §7 / §10）。
 *
 * 列表/详情均从 apiserver Operation BFF 加载（服务端分页/过滤/精确总数），
 * 状态使用服务端 Operation 状态机，不使用本地硬编码表格冒充 Schema 驱动。
 */
import type { PageSchema } from '@hnb/schema-engine'
import type { OperationStatus } from '../types/operation'

export const operationListSchema: PageSchema = {
  apiVersion: 'ui.hnb.io/v1',
  kind: 'PageSchema',
  metadata: {
    id: 'resource.operation.list',
    revision: 1,
    pluginId: 'resource',
    minShellVersion: '2.5.0',
  },
  spec: {
    template: 'list',
    titleKey: 'resource.operationCenter.title',
    descriptionKey: 'resource.operationCenter.desc',
    contextRequirements: ['tenantId'],
    layout: { type: 'grid', columns: 12, gap: 'md' },
    endpoints: [
      { id: 'resource.operations.list', path: '/api/v1/operations', method: 'GET' },
      { id: 'resource.operations.detail', path: '/api/v1/operations/{operationId}', method: 'GET' },
    ],
    dataSources: [
      {
        id: 'resource.operation.list',
        type: 'paginatedQuery',
        endpointId: 'resource.operations.list',
        queryBindings: ['page', 'pageSize', 'status', 'type', 'targetId'],
        responseMapping: { items: 'items', total: 'pagination.total' },
      },
    ],
    regions: [
      { id: 'table', componentType: 'DataTable', span: 12, condition: { all: [{ permission: 'operation:list' }] } },
    ],
  },
}

export interface OperationListColumn {
  key: string
  titleKey: string
  width?: string
}

export const operationListColumns: OperationListColumn[] = [
  { key: 'operationId', titleKey: 'resource.operationCenter.col.operationId' },
  { key: 'type', titleKey: 'resource.operationCenter.col.type' },
  { key: 'targetId', titleKey: 'resource.operationCenter.col.target' },
  { key: 'status', titleKey: 'resource.operationCenter.col.status' },
  { key: 'progress', titleKey: 'resource.operationCenter.col.progress' },
  { key: 'createdAt', titleKey: 'resource.operationCenter.col.createdAt' },
  { key: 'actions', titleKey: 'resource.operationCenter.col.actions', width: '120px' },
]

export const OPERATION_STATUS_OPTIONS: Array<{ value: OperationStatus | ''; labelKey: string }> = [
  { value: '', labelKey: 'resource.operationCenter.filter.allStatus' },
  { value: 'pending_approval', labelKey: 'resource.operationCenter.status.pending_approval' },
  { value: 'queued', labelKey: 'resource.operationCenter.status.queued' },
  { value: 'queued_offline', labelKey: 'resource.operationCenter.status.queued_offline' },
  { value: 'in_progress', labelKey: 'resource.operationCenter.status.in_progress' },
  { value: 'paused', labelKey: 'resource.operationCenter.status.paused' },
  { value: 'succeeded', labelKey: 'resource.operationCenter.status.succeeded' },
  { value: 'failed', labelKey: 'resource.operationCenter.status.failed' },
  { value: 'cancelled', labelKey: 'resource.operationCenter.status.cancelled' },
]
