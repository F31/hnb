/**
 * @hnb/schema-engine — UI Schema 引擎（UI 规范 V2.5）
 *
 * 组成：
 *  - SchemaEngine：信封校验与标准化（§4）
 *  - ComponentRegistry：受信组件注册与 props 校验（§8）
 *  - DataSourceManager：统一查询/分页/字典，endpointId 白名单（§13）
 *  - ActionEngine：导航/API/事件/弹窗动作执行（§12）
 *  - PageRenderer：PageSchema 渲染，区块级错误隔离（§7）
 */

export * from './types'
export { SchemaEngine, SchemaError, createSchemaEngine, SUPPORTED_API_VERSIONS, SHELL_VERSION, compareVersion } from './SchemaEngine'
export { ComponentRegistry, createComponentRegistry } from './ComponentRegistry'
export type { ComponentDefinition, JsonSchemaSubset, PropsValidationResult } from './ComponentRegistry'
export { DataSourceManager, createDataSourceManager, isTrustedPath } from './DataSourceManager'
export type { DataQuery } from './DataSourceManager'
export { ExtensionRegistry, createExtensionRegistry } from './ExtensionRegistry'
export type { ExtensionPointDefinition, ExtensionValidationResult } from './ExtensionRegistry'
export { ActionEngine, createActionEngine } from './ActionEngine'
export type { ActionContext, ActionEngineOptions } from './ActionEngine'
export { ConditionEvaluator, createConditionEvaluator } from './ConditionEvaluator'
export type { ConditionContext } from './ConditionEvaluator'
export { default as PageRenderer } from './components/PageRenderer.vue'
export { default as RegionWrapper } from './components/RegionWrapper.vue'
export { registerBuiltinComponents } from './builtins'
export {
  DATA_SOURCE_MANAGER_KEY,
  provideDataSourceManager,
  useDataSourceManager,
  useDataSource,
} from './composables'
