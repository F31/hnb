<script setup lang="ts">
/**
 * EdgeNodeGroupsPage — 集群详情 > 边缘节点组（OpenSpec edge-node-group）。
 * 名称搜索/查询/刷新/设置；列：名称、状态、节点数、描述、操作。
 * 空列表时保留工具栏与表头，表体居中显示"暂无数据"，不显示无意义分页。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { HNBTable, StatusBadge, HNBTableActions } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTableAction } from '@hnb/ui-kit'
import { getEdgeNodeGroups } from './api/p4Api'
import { usePluginContext } from './composables/usePluginContext'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import type { EdgeNodeGroup } from './types/p4'

const { t } = useI18n()
const route = useRoute()
const pluginCtx = usePluginContext()
const clusterId = String(route.params.clusterId ?? '')

const groups = ref<EdgeNodeGroup[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')

function statusBadge(g: EdgeNodeGroup): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = { running: 'success', abnormal: 'error', unknown: 'default' }
  return { label: t(`resource.clusterMgmt.edgeGroup.status.${g.status}`), semantic: map[g.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<EdgeNodeGroup>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.edgeGroup.colName'), render: (row) => row.name || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.edgeGroup.colStatus'),
    render: (row) => {
      const b = statusBadge(row)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'nodeCount', title: t('resource.clusterMgmt.edgeGroup.colNodes'), render: (row) => String(row.nodeCount) },
  { key: 'description', title: t('resource.clusterMgmt.edgeGroup.colDesc'), render: (row) => row.description || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: () => {
      const actions: HNBTableAction[] = [{ label: t('resource.clusterMgmt.nodeList.action.detail'), key: 'detail' }]
      return h(HNBTableActions, { actions, onAction: () => pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled')) })
    },
  },
])

async function load(): Promise<void> {
  if (!clusterId) return
  loading.value = true
  error.value = ''
  try {
    groups.value = await getEdgeNodeGroups(clusterId, { keyword: keyword.value })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    groups.value = []
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  load()
}

onMounted(load)
</script>

<template>
  <ClusterDetailLayout>
    <div class="edge-group-page">
      <div class="page-toolbar">
        <div class="toolbar-left">
          <input
            v-model="keyword"
            class="keyword-input"
            type="text"
            :placeholder="t('resource.clusterMgmt.edgeGroup.keywordPlaceholder')"
            @keyup.enter="onSearch"
          />
          <button class="secondary-button" type="button" @click="onSearch">
            {{ t('resource.clusterMgmt.action.query') }}
          </button>
          <button class="secondary-button" type="button" @click="load">
            {{ t('resource.clusterMgmt.action.refresh') }}
          </button>
          <button class="secondary-button" type="button" @click="pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled'))">
            {{ t('resource.clusterMgmt.nodeList.action.settings') }}
          </button>
        </div>
      </div>

      <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
      <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
      <HNBTable
        v-else
        :columns="columns"
        :data="groups"
        :empty-title="t('resource.clusterMgmt.edgeGroup.empty')"
        min-width="760px"
        :aria-label="t('resource.clusterMgmt.edgeGroup.title')"
      />
    </div>
  </ClusterDetailLayout>
</template>

<style scoped>
.edge-group-page { display: flex; flex-direction: column; gap: 10px; }
.page-toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.toolbar-left { display: flex; gap: 8px; flex-wrap: wrap; }
.keyword-input {
  padding: 6px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 220px;
}
.secondary-button {
  padding: 7px 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
}
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
</style>
