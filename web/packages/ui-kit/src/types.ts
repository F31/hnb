/**
 * ui-kit 公共类型（从 .vue 组件中抽出，便于 tsc 直接解析与复用）。
 */

import type { ComputedRef, InjectionKey } from 'vue'

export interface HNBTableColumn<T = Record<string, any>> {
  key: string
  title: string
  width?: string
  render?: (row: T, index: number) => any
}

export type HNBTableRowKey = string | ((row: Record<string, any>) => string | number)

export interface HNBTablePagination {
  page: number
  pageSize: number
  total: number
}

export interface HNBTab {
  id: string
  label: string
  disabled?: boolean
  disabledReason?: string
}

export interface HNBStatusItem {
  key: string
  label: string
  valueLabel: string
  semantic?: StatusSemantic
}

export type OperationStepStatus = 'pending' | 'running' | 'success' | 'error' | 'skipped'

export interface OperationProgressStep {
  id: string
  label: string
  status: OperationStepStatus
  description?: string
  timestamp?: string
}

export type PageStateKind = 'loading' | 'empty' | 'error' | 'no-permission' | 'offline' | 'incompatible'

export type StatusSemantic =
  | 'success'
  | 'warning'
  | 'error'
  | 'info'
  | 'processing'
  | 'default'

export type HNBButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type HNBButtonSize = 'small' | 'medium' | 'large'

export interface HNBSelectOption {
  label: string
  value: string
  disabled?: boolean
}

export interface HNBDetailItem {
  label: string
  value?: string | number | boolean | null
}

export interface HNBTableAction {
  label: string
  key: string
  variant?: HNBButtonVariant
  disabled?: boolean
}

export const HNB_FORM_FIELD_INJECTION_KEY = Symbol('hnb-form-field') as InjectionKey<{ ariaDescribedBy: ComputedRef<string | undefined> }>
