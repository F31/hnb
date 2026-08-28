<script setup lang="ts">
/**
 * ClusterOverviewPanel — 集群信息 > 集群详情 基本信息（OpenSpec cluster-overview）。
 *
 * 通过注入的 DataSourceManager 拉取集群 Read Model，映射为 ClusterDetail 后
 * 以三列栅格展示；描述支持行内编辑（保存/取消，失败保留输入）。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { StatusBadge } from '@hnb/ui-kit'
import { useDataSourceManager } from '@hnb/schema-engine'
import {
  mapSummaryToDetail,
  updateClusterDescription,
  PLACEHOLDER,
} from '../api/clusterDetailApi'
import { usePluginContext } from '../composables/usePluginContext'
import { deriveContextKey } from '../composables/usePluginContext'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { ClusterDetail, ClusterDetailStatus } from '../types/cluster'

const { t } = useI18n()
const pluginCtx = usePluginContext()
const clusterId = useClusterDetailId()
const dataSources = useDataSourceManager()

const cluster = ref<ClusterDetail | null>(null)
const loading = ref(true)
const error = ref('')

// ---- 描述行内编辑 ----
const editing = ref(false)
const draft = ref('')
const saving = ref(false)
const editError = ref('')

const statusSemantic = computed(() => {
  const map: Record<ClusterDetailStatus, 'success' | 'error' | 'default'> = {
    running: 'success',
    abnormal: 'error',
    unknown: 'default',
  }
  return cluster.value ? map[cluster.value.status] : 'default'
})

const statusLabel = computed(() => {
  if (!cluster.value) return ''
  const map: Record<ClusterDetailStatus, string> = {
    running: t('resource.clusterMgmt.detail.status.running'),
    abnormal: t('resource.clusterMgmt.detail.status.abnormal'),
    unknown: t('resource.clusterMgmt.detail.status.unknown'),
  }
  return map[cluster.value.status]
})

/** 三列栅格字段（label/value 渲染；状态与描述单独处理） */
const gridFields = computed(() => {
  const c = cluster.value
  if (!c) return []
  return [
    { label: t('resource.clusterMgmt.detail.id'), value: c.id || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.name'), value: c.name || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.k8sVersion'), value: c.kubernetesVersion || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.createdAt'), value: c.createdAt || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.osVersion'), value: c.osVersion || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.cpuArch'), value: c.cpuArchitecture || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.controlPlaneSchedule'), value: c.controlPlaneSchedulingEnabled ? t('resource.clusterMgmt.detail.yes') : t('resource.clusterMgmt.detail.no') },
    { label: t('resource.clusterMgmt.detail.clusterType'), value: c.clusterType || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.managementVip'), value: c.managementVip || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.clusterVip'), value: c.clusterVip || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.podCidr'), value: c.podCidr || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.serviceCidr'), value: c.serviceCidr || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.clusterDomain'), value: c.clusterDomain || PLACEHOLDER },
    { label: t('resource.clusterMgmt.detail.kubeOvnJoinCidr'), value: c.kubeOvnJoinCidr || PLACEHOLDER },
  ]
})

async function load(): Promise<void> {
  if (!clusterId || !dataSources) return
  loading.value = true
  error.value = ''
  try {
    const summary = await dataSources.fetch<Record<string, unknown>>('resource.cluster.detail', {
      params: { clusterId },
      contextKey: deriveContextKey(pluginCtx.contextStore.current),
    })
    cluster.value = mapSummaryToDetail(summary ?? {})
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    cluster.value = null
  } finally {
    loading.value = false
  }
}

function startEdit(): void {
  draft.value = cluster.value?.description ?? ''
  editError.value = ''
  editing.value = true
}

function cancelEdit(): void {
  editing.value = false
  draft.value = ''
  editError.value = ''
}

async function saveEdit(): Promise<void> {
  if (!clusterId || !cluster.value || saving.value) return
  saving.value = true
  editError.value = ''
  try {
    await updateClusterDescription(clusterId, draft.value.trim())
    cluster.value = { ...cluster.value, description: draft.value.trim() }
    editing.value = false
  } catch (err) {
    editError.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="overview-panel" :aria-label="t('resource.clusterMgmt.aria.clusterBasicInfo')">
    <div class="panel-head">
      <SectionHeader :title="t('resource.clusterMgmt.detail.basicParams')" />
      <StatusBadge
        v-if="cluster && !loading"
        :label="statusLabel"
        :semantic="statusSemantic"
      />
    </div>

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="load">{{ t('resource.clusterMgmt.action.retry') }}</button>
    </div>

    <template v-else-if="cluster">
      <dl class="field-grid">
        <div v-for="field in gridFields" :key="field.label" class="field-item">
          <dt>{{ field.label }}</dt>
          <dd :title="String(field.value)">{{ field.value }}</dd>
        </div>
      </dl>

      <!-- 描述（可编辑） -->
      <div class="desc-row">
        <span class="desc-label">{{ t('resource.clusterMgmt.detail.description') }}</span>
        <template v-if="!editing">
          <span class="desc-value">{{ cluster.description || PLACEHOLDER }}</span>
          <button
            class="desc-edit"
            type="button"
            :aria-label="t('resource.clusterMgmt.detail.editDescription')"
            @click="startEdit"
          >
            ✎
          </button>
        </template>
        <template v-else>
          <textarea
            v-model="draft"
            class="desc-textarea"
            :aria-label="t('resource.clusterMgmt.detail.description')"
            rows="3"
          ></textarea>
          <div class="desc-actions">
            <button class="primary-mini" type="button" :disabled="saving" @click="saveEdit">
              {{ saving ? t('resource.clusterMgmt.common.submitting') : t('resource.clusterMgmt.common.submit') }}
            </button>
            <button class="ghost-mini" type="button" :disabled="saving" @click="cancelEdit">
              {{ t('resource.clusterMgmt.common.cancel') }}
            </button>
          </div>
          <p v-if="editError" class="edit-error" role="alert">{{ editError }}</p>
        </template>
      </div>
    </template>
  </section>
</template>

<style scoped>
.overview-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.retry-button {
  margin-left: 8px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  padding: 2px 10px;
}
.field-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px 20px;
  margin: 0;
}
.field-item {
  min-width: 0;
  border-bottom: 1px dashed var(--hnb-color-border, #e2e7ef);
  padding-bottom: 6px;
}
.field-item dt {
  font-size: 12px;
  color: var(--hnb-color-text-tertiary, #8a94a3);
  margin-bottom: 2px;
}
.field-item dd {
  margin: 0;
  font-size: 14px;
  color: var(--hnb-color-text-primary, #12172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.desc-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-top: 10px;
  font-size: 14px;
}
.desc-label {
  flex-shrink: 0;
  color: var(--hnb-color-text-secondary, #5b6675);
  width: 88px;
}
.desc-value {
  flex: 1;
  color: var(--hnb-color-text-primary, #12172a);
  word-break: break-word;
}
.desc-edit {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 14px;
  padding: 0 4px;
}
.desc-textarea {
  flex: 1;
  min-width: 200px;
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  font-family: inherit;
  font-size: 13px;
  resize: vertical;
}
.desc-actions { display: flex; gap: 8px; }
.primary-mini {
  padding: 5px 14px;
  border: 0;
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-primary, #2f6fed);
  color: #fff;
  cursor: pointer;
  font-size: 13px;
}
.primary-mini:disabled { opacity: 0.6; cursor: not-allowed; }
.ghost-mini {
  padding: 5px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
  font-size: 13px;
}
.edit-error { color: var(--hnb-color-status-danger, #f04438); font-size: 13px; }
</style>
