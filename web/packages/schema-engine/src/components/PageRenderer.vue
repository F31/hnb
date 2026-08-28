<script setup lang="ts">
/**
 * PageRenderer — 按 PageSchema 渲染页面（V2.5 §7）。
 *
 * 安全约束（V2.5 §3.4 / §8 / §16.1）：
 *  - componentType 必须存在于 ComponentRegistry，未知类型显示安全占位符；
 *  - region props 经组件 propsSchema 校验，校验失败显示安全占位符；
 *  - endpoint/dataSource 必须命中 DataSourceManager allowlist，任意 URL 拒绝；
 *  - region 引用的 actionId 必须存在于 schema spec.actions，未知 actionId 拒绝；
 *  - 区块级错误隔离，单区块失败不影响整页。
 *
 * 状态渲染统一复用 ui-kit 原语（HNBPageState / HNBAlert / HNBButton / HNBSkeleton），
 * 覆盖 loading / empty / error / no-permission / offline / incompatible 六态。
 */
import { computed, reactive, ref, toRaw, watch, type ComputedRef } from 'vue'
import { HNBAlert, HNBButton, HNBPageState } from '@hnb/ui-kit'
import type { PageSchema, PageRegion } from '../types'
import { SchemaEngine, SchemaError } from '../SchemaEngine'
import type { ComponentRegistry } from '../ComponentRegistry'
import type { DataSourceManager } from '../DataSourceManager'
import type { ActionEngine } from '../ActionEngine'
import { ConditionEvaluator, type ConditionContext } from '../ConditionEvaluator'
import type { ExtensionRegistry } from '../ExtensionRegistry'
import RegionWrapper from './RegionWrapper.vue'

const props = defineProps<{
  schema: PageSchema
  registry: ComponentRegistry
  /** 服务端已下发的文案（i18n 就绪前直接使用 title/description 字段） */
  texts?: Record<string, string>
  dataSources?: DataSourceManager
  actionEngine?: ActionEngine
  conditionContext?: ConditionContext
  extensionRegistry?: ExtensionRegistry
}>()

/**
 * 经 props 传入的类实例会被 Vue 转为 reactive 代理（含内部 Map 与组件对象），
 * 导致组件对象身份变化并触发 “Component was made a reactive object” 警告。
 * 这里统一解包为原始实例，保证组件引用稳定。
 */
const registry = toRaw(props.registry)
const extensionRegistry = toRaw(props.extensionRegistry)
const dataSources = toRaw(props.dataSources)
const actionEngine = toRaw(props.actionEngine)

/** V2.6 §7.2：runtimeProps 中未就绪数据的稳定空引用，避免每次重算新建数组 */
const EMPTY_ARRAY: unknown[] = []

const engine = new SchemaEngine()

const validation = computed<{ code: string; message: string } | null>(() => {
  try {
    engine.validatePageSchema(props.schema)
    return null
  } catch (err) {
    if (err instanceof SchemaError) return { code: err.code, message: err.message }
    return { code: 'INVALID', message: err instanceof Error ? err.message : String(err) }
  }
})

/** 页面级六态：incompatible（Schema 不兼容）或 invalid（页面不可用） */
const pageState = computed<'incompatible' | 'error' | null>(() => {
  if (!validation.value) return null
  return validation.value.code === 'INCOMPATIBLE' ? 'incompatible' : 'error'
})

const layoutClass = computed(() => `page-layout-${props.schema.spec?.layout?.type ?? 'grid'}`)
const regionData = reactive<Record<string, unknown[]>>({})
const regionLoading = reactive<Record<string, boolean>>({})
const regionErrors = reactive<Record<string, string>>({})
const regionStates = reactive<Record<string, 'empty' | 'error' | 'loading' | null>>({})

// 构建 dataSource Map 避免 O(n) .find() 查找
const dataSourceMap = computed(() => {
  const map = new Map<string, { type?: string }>()
  for (const ds of props.schema.spec?.dataSources ?? []) {
    map.set(ds.id, ds)
  }
  return map
})

function text(key?: string): string {
  if (!key) return ''
  return props.texts?.[key] ?? key
}

interface ResolvedRegion {
  region: PageRegion
  component: unknown
  props: Record<string, unknown>
  error: string | null
  actionError: string | null
  actions: unknown[]
}

function normalizePermissions(permissions: readonly string[] | Set<string> | undefined): readonly string[] | undefined {
  if (!permissions) return undefined
  return Array.isArray(permissions) ? permissions : Array.from(permissions)
}

function resolveRegionActions(region: PageRegion): { actionError: string | null; actions: unknown[] } {
  const ids = region.props?.actions
  if (!Array.isArray(ids)) return { actionError: null, actions: [] }
  const actions = props.schema.spec.actions ?? []
  const known = new Set(actions.map((a) => a.id))
  for (const id of ids) {
    if (typeof id !== 'string' || !known.has(id)) {
      return { actionError: `未注册的 actionId: ${String(id)}`, actions: [] }
    }
  }
  const permissions = props.conditionContext?.permissions
  const hasPermission = (permission?: string) => {
    if (!permission) return true
    if (!permissions) return false
    return Array.isArray(permissions) ? permissions.includes(permission) || permissions.includes('*') : permissions.has(permission) || permissions.has('*')
  }
  const resolved = actions.filter((action) =>
    ids.includes(action.id) && hasPermission(action.permission) && conditionEvaluator.evaluate(action.enabledWhen),
  )
  return { actionError: null, actions: resolved }
}

// 复用 ConditionEvaluator 实例（V2.6 §4.4），上下文以 getter 形式注入：
// 每次求值读取最新 props.conditionContext（权限/Feature 异步就绪后可重算），
// 且 getter 在 computed 内求值时会建立对 props 的响应式依赖。
const conditionEvaluator = new ConditionEvaluator(() => props.conditionContext ?? {})

// 过滤出符合条件的 region
const activeRegions = computed(() => {
  if (pageState.value) return []
  return (props.schema.spec?.regions ?? []).filter((region) => conditionEvaluator.evaluate(region.condition))
})

// 每个 region 独立的 ResolvedRegion（避免单体 computed 全量重算）

function resolveRegion(region: PageRegion): ResolvedRegion {
  if (region.extensionPoint) {
    if (!extensionRegistry) {
      return { region, component: null, props: {}, error: `扩展点未注册: ${region.extensionPoint}`, actionError: null, actions: [] }
    }
    const extensions = extensionRegistry.list(region.extensionPoint, normalizePermissions(props.conditionContext?.permissions))
    if (extensions.length === 0) {
      return { region, component: null, props: {}, error: `无可用扩展: ${region.extensionPoint}`, actionError: null, actions: [] }
    }
    return { region, component: registry.resolve(extensions[0].componentType), props: { ...(extensions[0].props ?? {}) }, error: null, actionError: null, actions: [] }
  }
  const component = registry.resolve(region.componentType)
  if (!component) {
    return { region, component: null, props: {}, error: `未注册的组件类型: ${region.componentType}`, actionError: null, actions: [] }
  }
  const check = registry.validateProps(region.componentType, region.props ?? {})
  if (!check.valid) {
    return { region, component: null, props: {}, error: `属性校验失败: ${check.errors.join('; ')}`, actionError: null, actions: [] }
  }
  const { actionError, actions } = resolveRegionActions(region)
  if (actionError) {
    return { region, component: null, props: {}, error: actionError, actionError, actions: [] }
  }
  return { region, component, props: region.props ?? {}, error: null, actionError: null, actions }
}

// 每个 region 的 computed，按需求值
interface RegionComputedEntry {
  region: PageRegion
  computed: ComputedRef<ResolvedRegion>
}

/**
 * V2.6 §7.4：region 对象身份变化（schema 更新产生新 region 对象）时重建
 * 对应 computed，使新 schema 的静态 props 生效；同一对象则复用，保留
 * 已加载数据与状态，不触发重新请求。
 */
const regionComputedMap = new Map<string, RegionComputedEntry>()

interface RuntimePropsMemo {
  src: Record<string, unknown>
  data: unknown
  loading: boolean
  props: Record<string, unknown>
}

/**
 * V2.6 §7.2 要求 2：runtimeProps（含 data/loading 动态属性）返回稳定引用。
 * 仅在 src/data/loading 三者之一真正变化时重建对象，避免每次重算创建
 * 新对象引用导致 Vue 子组件不必要的 patch。
 */
const runtimePropsCache = new Map<string, RuntimePropsMemo>()

function buildRuntimeProps(
  regionId: string,
  src: Record<string, unknown>,
  data: unknown,
  loading: boolean,
): Record<string, unknown> {
  const cached = runtimePropsCache.get(regionId)
  if (cached && cached.src === src && cached.data === data && cached.loading === loading) {
    return cached.props
  }
  const props = { ...src, data, loading }
  runtimePropsCache.set(regionId, { src, data, loading, props })
  return props
}

function getRegionComputed(region: PageRegion): ComputedRef<ResolvedRegion> {
  const id = region.id
  const existing = regionComputedMap.get(id)
  if (existing && existing.region === region) return existing.computed

  const computedRef = computed(() => {
    const base = resolveRegion(region)
    if (base.error || !base.component) return base
    const dataSource = region.props?.dataSource
    if (typeof dataSource === 'string') {
      return {
        ...base,
        props: buildRuntimeProps(
          id,
          region.props ?? {},
          regionData[id] ?? EMPTY_ARRAY,
          regionLoading[id] === true,
        ),
      }
    }
    return base
  })
  regionComputedMap.set(id, { region, computed: computedRef })
  return computedRef
}

const resolvedRegions = computed<ResolvedRegion[]>(() => {
  return activeRegions.value.map((region) => getRegionComputed(region).value as ResolvedRegion)
})

watch(
  () => props.schema,
  async (schema, oldSchema) => {
    // 注册端点和数据源（幂等，DataSourcesManager 内部 Map 覆盖旧值）
    for (const endpoint of schema.spec.endpoints ?? []) dataSources?.registerEndpoint(endpoint)
    for (const dataSource of schema.spec.dataSources ?? []) dataSources?.registerDataSource(dataSource)

    const regions = schema.spec.regions ?? []

    // V2.6 §7.4 配套清理：已从 schema 移除的 region 同步清理
    // 数据/加载/错误/状态与 computed/memo，避免陈旧条目残留。
    const newIds = new Set(regions.map((r) => r.id))
    for (const staleId of Array.from(Object.keys(regionData))) {
      if (newIds.has(staleId)) continue
      delete regionData[staleId]
      delete regionLoading[staleId]
      delete regionErrors[staleId]
      delete regionStates[staleId]
      regionComputedMap.delete(staleId)
      runtimePropsCache.delete(staleId)
    }

    // 只加载新增或关键 props 发生变化的 region
    const oldRegions = oldSchema?.spec?.regions ?? []
    const oldMap = new Map(oldRegions.map((r) => [r.id, r]))
    const changed = regions.filter((r) => {
      const old = oldMap.get(r.id)
      if (!old) return true
      // 对比 dataSource、endpoint 等关键 prop 变化
      const oldProps = old.props ?? {}
      const newProps = r.props ?? {}
      if (oldProps.dataSource !== newProps.dataSource) return true
      if (oldProps.endpoint !== newProps.endpoint) return true
      if (JSON.stringify(oldProps.params) !== JSON.stringify(newProps.params)) return true
      return false
    })
    await Promise.all(changed.map(loadRegionData))
  },
  { immediate: true },
)

async function loadRegionData(region: PageRegion): Promise<void> {
  const dataSource = region.props?.dataSource
  if (!dataSources || typeof dataSource !== 'string') return
  regionLoading[region.id] = true
  regionErrors[region.id] = ''
  regionStates[region.id] = 'loading'
  try {
    const def = dataSourceMap.value.get(dataSource)
    if (def?.type === 'paginatedQuery') {
      const result = await dataSources.fetchPaginated(dataSource, { page: 1, pageSize: 20 })
      regionData[region.id] = result.items as unknown[]
    } else {
      const result = await dataSources.fetch(dataSource)
      regionData[region.id] = Array.isArray(result) ? result : []
    }
    regionStates[region.id] = regionData[region.id].length > 0 ? null : 'empty'
  } catch (err) {
    regionErrors[region.id] = err instanceof Error ? err.message : String(err)
    regionData[region.id] = []
    regionStates[region.id] = 'error'
  } finally {
    regionLoading[region.id] = false
  }
}

async function executeAction(actionId: string): Promise<void> {
  const action = props.schema.spec.actions?.find((candidate) => candidate.id === actionId)
  if (!action || !actionEngine) return
  await actionEngine.execute(action)
}

function regionStyle(region: PageRegion): Record<string, string> {
  const span = Math.min(Math.max(region.span ?? 12, 1), 12)
  return { gridColumn: `span ${span}` }
}
</script>

<template>
  <div class="page-renderer">
    <!-- 页面级六态：incompatible / error（fail-closed，不渲染任何区块） -->
    <HNBPageState
      v-if="pageState === 'incompatible'"
      state="incompatible"
      :title="text(schema.spec?.titleKey) || 'Schema 不兼容'"
      :description="validation?.message"
    />
    <HNBPageState
      v-else-if="pageState === 'error'"
      state="error"
      :title="text(schema.spec?.titleKey) || '页面 Schema 不可用'"
      :description="validation?.message"
    />

    <template v-else>
      <header v-if="schema.spec.titleKey || schema.spec.descriptionKey" class="page-header">
        <h1 v-if="schema.spec.titleKey" class="page-title">{{ text(schema.spec.titleKey) }}</h1>
        <p v-if="schema.spec.descriptionKey" class="page-description">
          {{ text(schema.spec.descriptionKey) }}
        </p>
      </header>

      <div class="page-grid" :class="layoutClass">
        <div
          v-for="item in resolvedRegions"
          :key="item.region.id"
          :style="regionStyle(item.region)"
        >
          <RegionWrapper :region-id="item.region.id">
            <!-- 区块级安全占位符：未知组件 / props 校验失败 / 未知 actionId -->
            <HNBAlert
              v-if="item.error"
              semantic="warning"
              live="assertive"
              :title="`区块 ${item.region.id} 不可用`"
            >{{ item.error }}</HNBAlert>

            <!-- 区块 loading / empty / error 六态（复用 ui-kit primitives） -->
            <HNBPageState
              v-else-if="regionStates[item.region.id] === 'loading'"
              state="loading"
              :title="`${item.region.id} loading`"
            />
            <HNBPageState
              v-else-if="regionStates[item.region.id] === 'empty'"
              state="empty"
              :title="text(schema.spec?.titleKey) || '暂无数据'"
            />
            <HNBPageState
              v-else-if="regionStates[item.region.id] === 'error'"
              state="error"
              :title="`${item.region.id} 加载失败`"
              :description="regionErrors[item.region.id]"
              action-text="重试"
            />

            <component :is="item.component" v-else v-bind="item.props" />

            <div v-if="!item.error && item.actions.length" class="region-actions">
              <HNBButton
                v-for="action in item.actions as Array<{ id: string; labelKey?: string }>"
                :key="action.id"
                size="small"
                @click="executeAction(action.id)"
              >
                {{ text(action.labelKey) || action.id }}
              </HNBButton>
            </div>
          </RegionWrapper>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-renderer {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md, 16px);
}
.page-header {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs, 4px);
}
.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--hnb-color-text-primary, #12172a);
}
.page-description {
  margin: 0;
  font-size: 13px;
  color: var(--hnb-color-text-secondary, #7a8a9a);
}
.page-grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: var(--hnb-space-md, 16px);
}
.region-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
@media (max-width: 768px) {
  .page-grid > * {
    grid-column: span 12 !important;
  }
}
</style>
