/**
 * @hnb/ui-kit — HNB 统一组件库（UI 规范 V2.5 §8 / §17）。
 *
 * 组件引用 tokens.css 的语义化 Design Token，不得写死色值；
 * 插件与 Shell 共用同一套组件与 Token 契约。
 */

// 副作用引入 Design Token（CSS 变量，含 Light/Dark 主题）
import './tokens.css'

export { default as HNBTable } from './components/HNBTable.vue'
export { default as HNBPageShell } from './components/HNBPageShell.vue'
export { default as HNBToolbar } from './components/HNBToolbar.vue'
export { default as HNBButton } from './components/HNBButton.vue'
export { default as HNBTableActions } from './components/HNBTableActions.vue'
export { default as HNBSelectInput } from './components/HNBSelectInput.vue'
export { default as HNBDateInput } from './components/HNBDateInput.vue'
export { default as HNBFormField } from './components/HNBFormField.vue'
export { default as HNBDetailPanel } from './components/HNBDetailPanel.vue'
export { default as HNBActionBar } from './components/HNBActionBar.vue'
export { default as StatusBadge } from './components/StatusBadge.vue'
export { default as MetricCard } from './components/MetricCard.vue'
export { default as DescriptionList } from './components/DescriptionList.vue'
export { default as EmptyState } from './components/EmptyState.vue'
export { default as ErrorState } from './components/ErrorState.vue'
export { default as HNBSkeleton } from './components/HNBSkeleton.vue'
export { default as HNBDialog } from './components/HNBDialog.vue'
export { default as HNBConfirmation } from './components/HNBConfirmation.vue'
export { default as HNBAlert } from './components/HNBAlert.vue'
export { default as HNBLiveRegion } from './components/HNBLiveRegion.vue'
export { default as HNBTabs } from './components/HNBTabs.vue'
export { default as HNBPagination } from './components/HNBPagination.vue'
export { default as HNBStatusGroup } from './components/HNBStatusGroup.vue'
export { default as HNBPageState } from './components/HNBPageState.vue'
export { default as HNBOperationProgress } from './components/HNBOperationProgress.vue'
export { default as HNBVirtualList } from './components/HNBVirtualList.vue'
export { default as HNBErrorBoundary } from './components/HNBErrorBoundary.vue'
export type {
  HNBButtonSize,
  HNBButtonVariant,
  HNBDetailItem,
  HNBSelectOption,
  HNBTableAction,
  HNBTableColumn,
  HNBTablePagination,
  HNBTableRowKey,
  HNBTab,
  HNBStatusItem,
  OperationProgressStep,
  OperationStepStatus,
  PageStateKind,
  StatusSemantic,
} from './types'
export { HNB_FORM_FIELD_INJECTION_KEY } from './types'
