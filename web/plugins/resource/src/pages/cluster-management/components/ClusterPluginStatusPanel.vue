<script setup lang="ts">
/**
 * ClusterPluginStatusPanel — 集群信息 > 插件/平台能力状态列表（OpenSpec cluster-overview）。
 * 运行中=绿色圆点+绿字；未安装=红色圆点+红字；未知=中性文案。
 * 数据经 service adapter（开发 fixture，生产空态）。
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { StatusBadge } from '@hnb/ui-kit'
import { getClusterPluginStatuses } from '../api/clusterDetailApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { ClusterPluginStatus, ClusterPluginStatusKind } from '../types/cluster'

const { t } = useI18n()
const clusterId = useClusterDetailId()

const items = ref<ClusterPluginStatus[]>([])
const loading = ref(true)
const error = ref('')

const semanticFor = computed(() => {
  const map: Record<ClusterPluginStatusKind, 'success' | 'error' | 'default'> = {
    running: 'success',
    installed: 'success',
    'not-installed': 'error',
    abnormal: 'error',
    unknown: 'default',
  }
  return map
})

const labelFor = computed(() => {
  const map: Record<ClusterPluginStatusKind, string> = {
    running: t('resource.clusterMgmt.pluginStatus.running'),
    installed: t('resource.clusterMgmt.pluginStatus.installed'),
    'not-installed': t('resource.clusterMgmt.pluginStatus.notInstalled'),
    abnormal: t('resource.clusterMgmt.pluginStatus.abnormal'),
    unknown: t('resource.clusterMgmt.pluginStatus.unknown'),
  }
  return map
})

async function load(): Promise<void> {
  if (!clusterId) return
  loading.value = true
  error.value = ''
  try {
    items.value = await getClusterPluginStatuses(clusterId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="plugin-status-panel" aria-label="插件状态">
    <SectionHeader :title="t('resource.clusterMgmt.pluginStatus.title')" />

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="load">{{ t('resource.clusterMgmt.action.retry') }}</button>
    </div>
    <p v-else-if="!items.length" class="panel-status empty">
      {{ t('resource.clusterMgmt.pluginStatus.empty') }}
    </p>
    <ul v-else class="plugin-list">
      <li v-for="item in items" :key="item.key" class="plugin-item">
        <span class="plugin-name">{{ item.displayName }}</span>
        <StatusBadge
          :label="labelFor[item.status]"
          :semantic="semanticFor[item.status]"
        />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.plugin-status-panel { display: flex; flex-direction: column; gap: 8px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.panel-status.empty { color: var(--hnb-color-text-tertiary, #8a94a3); }
.retry-button {
  margin-left: 8px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  padding: 2px 10px;
}
.plugin-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px 20px;
}
.plugin-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 2px;
  border-bottom: 1px dashed var(--hnb-color-border, #e2e7ef);
}
.plugin-name {
  font-size: 13px;
  color: var(--hnb-color-text-primary, #12172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
