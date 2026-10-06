<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { createComponentRegistry } from '@hnb/schema-engine'
import { useI18n } from 'vue-i18n'
import {
  HNBAlert,
  HNBPageShell,
  HNBPageState,
  HNBTable,
  HNBTabs,
  MetricCard,
  StatusBadge,
} from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTab, StatusSemantic } from '@hnb/ui-kit'
import type {
  StorageAlertRule,
  StorageBackend,
  StorageClassBinding,
  StorageOverview,
  WorkloadStorageOffering,
  ProviderBackendSchema,
  StorageBackendInput,
} from '@hnb/contracts/storage'
import {
  getStorageOverview,
  createStorageBackend,
  listStorageAlertRules,
  listStorageBackends,
  listStorageOfferingBindings,
  listStorageOfferings,
  listStorageProviderSchemas,
} from './storage/storageApi'
import { BACKEND_CONFIGURATION_COMPONENT, registerStorageComponents } from './storage/runtime/registerStorageComponents'
import { formatCapacity } from './storage/storagePresentation'
import type { CapacityStatus } from './storage/storagePresentation'

type Freshness = 'Fresh' | 'Stale' | 'Unknown'
type SectionState<T> = { data: T; loading: boolean; error: string }

const { t, locale } = useI18n()
const activeTab = ref('overview')
const overview = ref<SectionState<StorageOverview | null>>({ data: null, loading: true, error: '' })
const backends = ref<SectionState<StorageBackend[]>>({ data: [], loading: true, error: '' })
const offerings = ref<SectionState<WorkloadStorageOffering[]>>({ data: [], loading: true, error: '' })
const offeringBindings = ref<Record<string, StorageClassBinding[]>>({})
const alertRules = ref<SectionState<StorageAlertRule[]>>({ data: [], loading: true, error: '' })
const providerSchemas = ref<ProviderBackendSchema[]>([])
const selectedProvider = ref('')
const creatingBackend = ref(false)
const backendFormError = ref('')
const storageComponents = createComponentRegistry()
registerStorageComponents(storageComponents)

const selectedProviderSchema = computed(() => providerSchemas.value.find((schema) => schema.providerType === selectedProvider.value))
const trustedBackendForm = computed(() => {
  const schema = selectedProviderSchema.value
  if (!schema || schema.componentType !== BACKEND_CONFIGURATION_COMPONENT) return null
  return storageComponents.resolve(schema.componentType)
})

const tabs = computed<HNBTab[]>(() => [
  { id: 'overview', label: t('resource.storage.tab.overview') },
  { id: 'systems', label: t('resource.storage.tab.systems') },
  { id: 'services', label: t('resource.storage.tab.services') },
  // 「驱动与连接器」页签隐藏：driverInstallations 后端为 stub，永远返回空，无真实数据源。
  { id: 'alerts', label: t('resource.storage.tab.alerts') },
])

const countCards = computed(() => {
  const counts = overview.value.data?.counts
  if (!counts) return []
  return [
    [t('resource.storage.metric.systems'), counts.backends],
    [t('resource.storage.metric.services'), counts.offerings],
    [t('resource.storage.metric.drivers'), counts.driverInstallations],
    [t('resource.storage.metric.targets'), counts.targets],
    [t('resource.storage.metric.bindings'), counts.bindings],
  ] as Array<[string, number]>
})

const capacityCards = computed(() => {
  const states = overview.value.data?.capacityStates
  if (!states) return []
  return (['Known', 'Elastic', 'Unknown', 'NotReported'] as CapacityStatus[]).map((status) => ({
    status,
    value: states[status],
  }))
})

function freshnessSemantic(freshness: Freshness): StatusSemantic {
  if (freshness === 'Fresh') return 'success'
  if (freshness === 'Stale') return 'warning'
  return 'default'
}

function capacitySemantic(status: CapacityStatus): StatusSemantic {
  if (status === 'Known') return 'success'
  if (status === 'Elastic') return 'info'
  if (status === 'Unknown') return 'warning'
  return 'default'
}

function formatObservedAt(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatSource(source: string): string {
  const key = {
    runtime_target_storage_inventory: 'clusterAgentObservation',
    'platform.desired-state': 'platformDesiredState',
    'storage-projection': 'storageProjection',
  }[source]
  return key ? t(`resource.storage.sourceLabel.${key}`) : source
}

function formatProviderType(providerType: string): string {
  const key = {
    'generic-csi': 'genericCsi',
    nfs: 'nfs',
    ceph: 'ceph',
    'cloud-disk': 'cloudDisk',
    'local-pv': 'localPv',
  }[providerType]
  return key ? t(`resource.storage.providerType.${key}`) : providerType
}

function metadata(source: string, observedAt: string, freshness: Freshness) {
  return h('div', { class: 'storage-metadata' }, [
    h('span', { title: source }, [h('strong', `${t('resource.storage.source')}: `), formatSource(source)]),
    h('span', [h('strong', `${t('resource.storage.observedAt')}: `), formatObservedAt(observedAt)]),
    h(StatusBadge, {
      label: t(`resource.storage.freshness.${freshness}`),
      semantic: freshnessSemantic(freshness),
    }),
  ])
}

const backendColumns = computed<HNBTableColumn<StorageBackend>[]>(() => [
  { key: 'displayName', title: t('resource.storage.column.name') },
  { key: 'providerType', title: t('resource.storage.column.provider'), render: (row) => formatProviderType(row.providerType) },
  // 健康列移除：后端 healthState 恒为 Unknown，展示它没有信息量。
  {
    key: 'capacity',
    title: t('resource.storage.column.capacity'),
    render: (row) => {
      const capacity = row.capacity
      if (!capacity) return h(StatusBadge, { label: t('resource.storage.capacity.NotReported'), semantic: 'default' })
      return h('div', { class: 'capacity-cell' }, [
        h(StatusBadge, {
          label: t(`resource.storage.capacity.${capacity.status}`),
          semantic: capacitySemantic(capacity.status),
        }),
        h('span', formatCapacity(capacity.status, capacity.value)),
        metadata(capacity.source, capacity.observedAt, capacity.freshness),
      ])
    },
  },
  {
    key: 'observation',
    title: t('resource.storage.column.observation'),
    render: (row) => metadata(row.source, row.observedAt, row.freshness),
  },
])

const offeringColumns = computed<HNBTableColumn<WorkloadStorageOffering>[]>(() => [
  { key: 'name', title: t('resource.storage.column.name') },
  { key: 'serviceMode', title: t('resource.storage.column.mode') },
  { key: 'scope', title: t('resource.storage.column.scope') },
  { key: 'accessModes', title: t('resource.storage.column.accessModes'), render: (row) => row.accessModes.join(', ') },
  { key: 'volumeExpansion', title: t('resource.storage.column.expansion') },
  { key: 'snapshots', title: t('resource.storage.column.snapshots') },
  { key: 'protectionClass', title: t('resource.storage.column.protection') },
  {
    key: 'bindings',
    title: t('resource.storage.column.bindings'),
    render: (row) => {
      const bindings = offeringBindings.value[row.id] ?? []
      if (!bindings.length) return '--'
      return h('div', { class: 'binding-links' }, bindings.map((binding) => {
        const query = new URLSearchParams({
          target: binding.targetId,
          cluster: binding.targetId,
          offering: row.id,
          storageClass: binding.storageClassName,
        })
        return h('a', {
          href: `/container/storage?${query.toString()}`,
          title: `${binding.targetId} / ${binding.storageClassName}`,
        }, `${binding.storageClassName} (${binding.targetId})`)
      }))
    },
  },
])

function alertSeveritySemantic(severity: StorageAlertRule['severity']): StatusSemantic {
  if (severity === 'critical') return 'error'
  if (severity === 'warning') return 'warning'
  return 'info'
}

const alertColumns = computed<HNBTableColumn<StorageAlertRule>[]>(() => [
  { key: 'name', title: t('resource.storage.column.name') },
  {
    key: 'severity',
    title: t('resource.storage.alert.colSeverity'),
    render: (row) => h(StatusBadge, {
      label: t(`resource.storage.alert.severity.${row.severity}`),
      semantic: alertSeveritySemantic(row.severity),
    }),
  },
  {
    key: 'condition',
    title: t('resource.storage.alert.colCondition'),
    render: (row) => `${row.metric.kind} ${row.metric.operator} ${row.metric.threshold} ${row.metric.unit}`,
  },
  { key: 'duration', title: t('resource.storage.alert.colDuration'), render: (row) => row.duration },
  {
    key: 'enabled',
    title: t('resource.storage.alert.colState'),
    render: (row) => h(StatusBadge, {
      label: t(row.enabled ? 'resource.storage.alert.enabled' : 'resource.storage.alert.disabled'),
      semantic: row.enabled ? 'success' : 'default',
    }),
  },
])

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : t('resource.storage.state.loadError')
}

async function loadOverview(): Promise<void> {
  overview.value.loading = true
  overview.value.error = ''
  try {
    overview.value.data = await getStorageOverview()
  } catch (error) {
    overview.value.error = errorMessage(error)
  } finally {
    overview.value.loading = false
  }
}

async function loadBackends(): Promise<void> {
  backends.value.loading = true
  backends.value.error = ''
  try {
    backends.value.data = (await listStorageBackends()).items
  } catch (error) {
    backends.value.error = errorMessage(error)
  } finally {
    backends.value.loading = false
  }
}

async function loadProviderSchemas(): Promise<void> {
  try {
    providerSchemas.value = (await listStorageProviderSchemas()).items
    selectedProvider.value = providerSchemas.value[0]?.providerType ?? ''
  } catch (error) {
    backendFormError.value = errorMessage(error)
  }
}

async function submitBackend(input: StorageBackendInput): Promise<void> {
  creatingBackend.value = true
  backendFormError.value = ''
  try {
    await createStorageBackend(input)
    await loadBackends()
  } catch (error) {
    backendFormError.value = errorMessage(error)
  } finally {
    creatingBackend.value = false
  }
}

async function loadOfferings(): Promise<void> {
  offerings.value.loading = true
  offerings.value.error = ''
  try {
    const items = (await listStorageOfferings()).items
    const entries = await Promise.all(items.map(async (offering) => {
      try {
        return [offering.id, (await listStorageOfferingBindings(offering.id)).items] as const
      } catch {
        return [offering.id, []] as const
      }
    }))
    offeringBindings.value = Object.fromEntries(entries)
    offerings.value.data = items
  } catch (error) {
    offerings.value.error = errorMessage(error)
  } finally {
    offerings.value.loading = false
  }
}

async function loadAlertRules(): Promise<void> {
  alertRules.value.loading = true
  alertRules.value.error = ''
  try {
    alertRules.value.data = (await listStorageAlertRules()).items
  } catch (error) {
    alertRules.value.error = errorMessage(error)
  } finally {
    alertRules.value.loading = false
  }
}

onMounted(() => {
  void Promise.all([loadOverview(), loadBackends(), loadProviderSchemas(), loadOfferings(), loadAlertRules()])
})
</script>

<template>
  <HNBPageShell :title="$t('resource.storage.title')" :description="$t('resource.storage.desc')">
    <HNBAlert semantic="info" :title="$t('resource.storage.readOnlyTitle')">
      {{ $t('resource.storage.readOnlyDesc') }}
    </HNBAlert>

    <HNBTabs v-model="activeTab" :tabs="tabs" :ariaLabel="$t('resource.storage.aria.tabs')">
      <template #panel-overview>
        <HNBPageState
          v-if="overview.loading"
          state="loading"
          :title="$t('resource.storage.state.loadingOverview')"
        />
        <HNBPageState
          v-else-if="overview.error"
          state="error"
          :title="$t('resource.storage.state.loadError')"
          :description="overview.error"
          :action-text="$t('resource.storage.state.retry')"
          @action="loadOverview"
        />
        <HNBPageState
          v-else-if="!overview.data"
          state="empty"
          :title="$t('resource.storage.state.emptyOverview')"
        />
        <div v-else class="overview-content">
          <div class="overview-metadata">
            <span :title="overview.data.source">
              <strong>{{ $t('resource.storage.source') }}:</strong> {{ formatSource(overview.data.source) }}
            </span>
            <span><strong>{{ $t('resource.storage.observedAt') }}:</strong> {{ formatObservedAt(overview.data.observedAt) }}</span>
            <StatusBadge
              :label="$t(`resource.storage.freshness.${overview.data.freshness}`)"
              :semantic="freshnessSemantic(overview.data.freshness)"
            />
          </div>
          <section aria-labelledby="storage-supply-heading">
            <h2 id="storage-supply-heading">{{ $t('resource.storage.section.supply') }}</h2>
            <div class="metric-grid">
              <MetricCard v-for="card in countCards" :key="card[0]" :title="card[0]" :value="card[1]" />
            </div>
          </section>
          <section aria-labelledby="storage-capacity-heading">
            <h2 id="storage-capacity-heading">{{ $t('resource.storage.section.capacity') }}</h2>
            <p class="section-hint">{{ $t('resource.storage.capacityHint') }}</p>
            <div class="capacity-grid">
              <article v-for="card in capacityCards" :key="card.status" class="capacity-card">
                <StatusBadge
                  :label="$t(`resource.storage.capacity.${card.status}`)"
                  :semantic="capacitySemantic(card.status)"
                />
                <strong>{{ card.value }}</strong>
                <span>{{ $t(`resource.storage.capacityHelp.${card.status}`) }}</span>
              </article>
            </div>
          </section>
        </div>
      </template>

      <template #panel-systems>
        <section class="backend-configuration" :aria-label="$t('resource.storage.backendForm.ariaLabel')">
          <HNBAlert v-if="backendFormError" semantic="error" :title="$t('resource.storage.backendForm.unavailable')">{{ backendFormError }}</HNBAlert>
          <label v-if="providerSchemas.length" class="provider-picker">
            <span>{{ $t('resource.storage.backendForm.providerSchema') }}</span>
            <select v-model="selectedProvider">
              <option v-for="schema in providerSchemas" :key="`${schema.providerType}@${schema.providerSchemaVersion}`" :value="schema.providerType">
                {{ formatProviderType(schema.providerType) }} ({{ schema.providerSchemaVersion }})
              </option>
            </select>
          </label>
          <component
            :is="trustedBackendForm"
            v-if="trustedBackendForm && selectedProviderSchema"
            :key="`${selectedProviderSchema.providerType}@${selectedProviderSchema.providerSchemaVersion}`"
            :schema="selectedProviderSchema"
            :submitting="creatingBackend"
            @submit="submitBackend"
          />
        </section>
        <HNBTable
          :columns="backendColumns"
          :data="backends.data"
          :loading="backends.loading"
          :error="backends.error"
          row-key="id"
          min-width="920px"
          :aria-label="$t('resource.storage.tab.systems')"
          :empty-title="$t('resource.storage.state.emptySystems')"
          :error-retry-text="$t('resource.storage.state.retry')"
          @error-retry="loadBackends"
        />
      </template>

      <template #panel-services>
        <HNBTable
          :columns="offeringColumns"
          :data="offerings.data"
          :loading="offerings.loading"
          :error="offerings.error"
          row-key="id"
          min-width="900px"
          :aria-label="$t('resource.storage.tab.services')"
          :empty-title="$t('resource.storage.state.emptyServices')"
          :error-retry-text="$t('resource.storage.state.retry')"
          @error-retry="loadOfferings"
        />
      </template>

      <template #panel-alerts>
        <HNBTable
          :columns="alertColumns"
          :data="alertRules.data"
          :loading="alertRules.loading"
          :error="alertRules.error"
          row-key="id"
          min-width="720px"
          :aria-label="$t('resource.storage.tab.alerts')"
          :empty-title="$t('resource.storage.alert.empty')"
          :error-retry-text="$t('resource.storage.state.retry')"
          @error-retry="loadAlertRules"
        />
      </template>
    </HNBTabs>
  </HNBPageShell>
</template>

<style scoped>
.overview-content,
.overview-content section {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
}

.backend-configuration { display: flex; flex-direction: column; gap: var(--hnb-space-md); margin-bottom: var(--hnb-space-lg); }
.provider-picker { display: flex; align-items: center; gap: var(--hnb-space-sm); color: var(--hnb-color-text-secondary); }
.provider-picker select { min-height: 34px; padding: 0 var(--hnb-space-sm); color: var(--hnb-color-text-primary); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }

.overview-metadata,
:deep(.storage-metadata) {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--hnb-space-sm) var(--hnb-space-lg);
  color: var(--hnb-color-text-secondary);
  font-size: var(--hnb-font-size-caption);
}

.overview-metadata {
  padding: var(--hnb-space-md);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  background: var(--hnb-color-bg-surface);
}

h2 {
  margin: 0;
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-subtitle);
}

.section-hint {
  margin: calc(var(--hnb-space-sm) * -1) 0 0;
  color: var(--hnb-color-text-tertiary);
  font-size: var(--hnb-font-size-caption);
}

.metric-grid,
.capacity-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: var(--hnb-space-md);
}

.capacity-card {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-md);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  background: var(--hnb-color-bg-surface);
}

.capacity-card > strong {
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-subtitle);
}

.capacity-card > span:last-child {
  grid-column: 1 / -1;
  color: var(--hnb-color-text-tertiary);
  font-size: var(--hnb-font-size-caption);
}

:deep(.capacity-cell) {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
}

:deep(.binding-links) {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
}

:deep(.binding-links a) {
  color: var(--hnb-color-primary);
  text-decoration: none;
}

@media (max-width: 768px) {
  .metric-grid,
  .capacity-grid {
    grid-template-columns: 1fr 1fr;
  }

  .overview-metadata {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (max-width: 480px) {
  .metric-grid,
  .capacity-grid {
    grid-template-columns: 1fr;
  }
}
</style>
