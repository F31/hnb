/**
 * 内置组件注册（V2.5 §8.1）：把 ui-kit 的标准组件注册为
 * 可供服务端 Schema 引用的受信 componentType。
 */

import {
  HNBActionBar,
  HNBAlert,
  HNBButton,
  HNBDateInput,
  HNBDetailPanel,
  HNBFormField,
  HNBOperationProgress,
  HNBPageShell,
  HNBPageState,
  HNBPagination,
  HNBSelectInput,
  HNBStatusGroup,
  HNBTabs,
  HNBTable,
  HNBTableActions,
  HNBToolbar,
  StatusBadge,
  MetricCard,
  DescriptionList,
  EmptyState,
  ErrorState,
  HNBSkeleton,
} from '@hnb/ui-kit'
import type { ComponentRegistry } from './ComponentRegistry'

export function registerBuiltinComponents(registry: ComponentRegistry): void {
  registry.register({
    type: 'PageShell',
    component: HNBPageShell,
    propsSchema: {
      type: 'object',
      required: ['title'],
      properties: {
        title: { type: 'string' },
        description: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'Toolbar',
    component: HNBToolbar,
    propsSchema: { type: 'object', properties: {}, additionalProperties: false },
  })
  registry.register({
    type: 'Button',
    component: HNBButton,
    propsSchema: {
      type: 'object',
      properties: {
        variant: { enum: ['primary', 'secondary', 'ghost', 'danger'] },
        size: { enum: ['small', 'medium', 'large'] },
        disabled: { type: 'boolean' },
        type: { enum: ['button', 'submit', 'reset'] },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'TableActions',
    component: HNBTableActions,
    propsSchema: {
      type: 'object',
      required: ['actions'],
      properties: { actions: { type: 'array' } },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'SelectInput',
    component: HNBSelectInput,
    propsSchema: {
      type: 'object',
      required: ['options'],
      properties: {
        modelValue: { type: 'string' },
        options: { type: 'array' },
        placeholder: { type: 'string' },
        disabled: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'DateInput',
    component: HNBDateInput,
    propsSchema: {
      type: 'object',
      properties: {
        modelValue: { type: 'string' },
        type: { enum: ['date', 'datetime-local'] },
        disabled: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'FormField',
    component: HNBFormField,
    propsSchema: {
      type: 'object',
      required: ['label'],
      properties: {
        label: { type: 'string' },
        help: { type: 'string' },
        error: { type: 'string' },
        required: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'DetailPanel',
    component: HNBDetailPanel,
    propsSchema: {
      type: 'object',
      required: ['items'],
      properties: {
        title: { type: 'string' },
        items: { type: 'array' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'ActionBar',
    component: HNBActionBar,
    propsSchema: { type: 'object', properties: {}, additionalProperties: false },
  })
  registry.register({
    type: 'DataTable',
    component: HNBTable,
    propsSchema: {
      type: 'object',
      required: ['columns'],
      properties: {
        columns: { type: 'array' },
        data: { type: 'array' },
        loading: { type: 'boolean' },
        schemaId: { type: 'string' },
        dataSource: { type: 'string' },
      },
    },
  })
  registry.register({
    type: 'StatusBadge',
    component: StatusBadge,
    propsSchema: {
      type: 'object',
      required: ['label'],
      properties: {
        label: { type: 'string' },
        semantic: { enum: ['success', 'warning', 'error', 'info', 'processing', 'default'] },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'MetricCard',
    component: MetricCard,
    propsSchema: {
      type: 'object',
      required: ['title', 'value'],
      properties: {
        title: { type: 'string' },
        value: {},
        unit: { type: 'string' },
        description: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'DescriptionList',
    component: DescriptionList,
    propsSchema: {
      type: 'object',
      required: ['items'],
      properties: {
        items: { type: 'array' },
        column: { type: 'number' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'EmptyState',
    component: EmptyState,
    propsSchema: {
      type: 'object',
      required: ['title'],
      properties: {
        title: { type: 'string' },
        description: { type: 'string' },
        actionText: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'ErrorState',
    component: ErrorState,
    propsSchema: {
      type: 'object',
      properties: {
        title: { type: 'string' },
        description: { type: 'string' },
        retryText: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'Skeleton',
    component: HNBSkeleton,
    propsSchema: {
      type: 'object',
      properties: {
        rows: { type: 'number' },
        title: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'PageState',
    component: HNBPageState,
    propsSchema: {
      type: 'object',
      required: ['state', 'title'],
      properties: {
        state: { enum: ['loading', 'empty', 'error', 'no-permission', 'offline', 'incompatible'] },
        title: { type: 'string' },
        description: { type: 'string' },
        actionText: { type: 'string' },
        actionLoading: { type: 'boolean' },
        skeletonRows: { type: 'number' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'Alert',
    component: HNBAlert,
    propsSchema: {
      type: 'object',
      properties: {
        title: { type: 'string' },
        semantic: { enum: ['info', 'success', 'warning', 'error'] },
        live: { enum: ['off', 'polite', 'assertive'] },
        dismissible: { type: 'boolean' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'Tabs',
    component: HNBTabs,
    propsSchema: {
      type: 'object',
      required: ['tabs'],
      properties: {
        tabs: { type: 'array' },
        modelValue: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'Pagination',
    component: HNBPagination,
    propsSchema: {
      type: 'object',
      required: ['page', 'pageSize', 'total'],
      properties: {
        page: { type: 'number' },
        pageSize: { type: 'number' },
        total: { type: 'number' },
        pageSizes: { type: 'array' },
        statusText: { type: 'string' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'OperationProgress',
    component: HNBOperationProgress,
    propsSchema: {
      type: 'object',
      required: ['status'],
      properties: {
        status: { type: 'string' },
        percent: { type: 'number' },
        steps: { type: 'array' },
      },
      additionalProperties: false,
    },
  })
  registry.register({
    type: 'StatusGroup',
    component: HNBStatusGroup,
    propsSchema: {
      type: 'object',
      required: ['items'],
      properties: {
        items: { type: 'array' },
      },
      additionalProperties: false,
    },
  })
}
