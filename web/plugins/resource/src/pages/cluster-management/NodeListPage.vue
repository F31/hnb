<script setup lang="ts">
/**
 * NodeListPage — 集群详情 > 节点列表（OpenSpec node-management）。
 * 工具栏（增加云端 worker/搜索字段/关键词/查询/刷新/设置）+ 节点表格（名称可点、
 * 状态圆点）+ 分页；节点名称或「节点详情」进入节点基础配置页。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBTableActions, HNBSelectInput, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTablePagination, HNBTableAction } from '@hnb/ui-kit'
import { getNodeList } from './api/nodeApi'
import { getClusterPermissionStore } from './api/clusterApi'
import { usePluginContext } from './composables/usePluginContext'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import type { NodeSummary } from './types/cluster'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const pluginCtx = usePluginContext()
const permissionStore = getClusterPermissionStore()

const clusterId = String(route.params.clusterId ?? '')

const nodes = ref<NodeSummary[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

const canUpdate = computed(() => permissionStore.hasPermission('cluster:update') || permissionStore.hasPermission('*'))

const searchFieldOptions = computed(() => [
  { label: t('resource.clusterMgmt.nodeDetail.pod.keyword'), value: 'name' },
])

const pagination = computed<HNBTablePagination>(() => ({
  page: page.value,
  pageSize: pageSize.value,
  total: total.value,
}))

function statusBadge(node: NodeSummary): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = {
    running: 'success',
    Ready: 'success',
    NotReady: 'error',
    Unknown: 'default',
  }
  return { label: t(`resource.clusterMgmt.nodeSummary.status.${node.status}`), semantic: map[node.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<NodeSummary>[]>(() => [
  {
    key: 'name',
    title: t('resource.clusterMgmt.col.name'),
    render: (row) => h('button', { type: 'button', class: 'node-name-link', onClick: () => goDetail(row) }, row.name || row.id),
  },
  { key: 'id', title: t('resource.clusterMgmt.nodeSummary.nodeId'), render: (row) => row.id || '--' },
  { key: 'type', title: t('resource.clusterMgmt.nodeList.colType'), render: (row) => row.type || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.nodeList.colStatus'),
    render: (row) => {
      const b = statusBadge(row)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'managementIp', title: t('resource.clusterMgmt.nodeDetail.field.managementIp'), render: (row) => row.managementIp || '--' },
  { key: 'clusterIp', title: t('resource.clusterMgmt.nodeDetail.field.clusterIp'), render: (row) => row.clusterIp || '--' },
  { key: 'nodeGroup', title: t('resource.clusterMgmt.nodeList.colNodeGroup'), render: (row) => row.nodeGroup || '--' },
  { key: 'createdAt', title: t('resource.clusterMgmt.nodeList.colCreatedAt'), render: (row) => row.createdAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.col.actions'),
    render: (row) => {
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.nodeList.action.detail'), key: 'detail' },
        { label: t('resource.clusterMgmt.nodeList.action.alias'), key: 'alias' },
      ]
      return h(HNBTableActions, {
        actions,
        onAction: (key: string) => {
          if (key === 'detail') goDetail(row)
          else onComingSoon()
        },
      })
    },
  },
])

function goDetail(node: NodeSummary): void {
  router.push(
    `/resource/clusters/${encodeURIComponent(clusterId)}/nodes/${encodeURIComponent(node.id)}/basic`,
  )
}

function onComingSoon(): void {
  pluginCtx.notify(t('resource.clusterMgmt.placeholder.notEnabled'))
}

async function load(): Promise<void> {
  if (!clusterId) return
  loading.value = true
  error.value = ''
  try {
    const res = await getNodeList(clusterId, { page: page.value, pageSize: pageSize.value, keyword: keyword.value })
    nodes.value = res.items
    total.value = res.total
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    nodes.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  page.value = 1
  load()
}

function onPage(p: number): void {
  page.value = p
  load()
}

function onPageSize(ps: number): void {
  pageSize.value = ps
  page.value = 1
  load()
}

onMounted(load)
</script>

<template>
  <ClusterDetailLayout>
    <div class="node-list-page">
      <div class="list-toolbar">
        <button
          v-if="canUpdate"
          type="button"
          class="primary-button"
          @click="onComingSoon"
        >
          {{ t('resource.clusterMgmt.nodeList.action.addWorker') }}
        </button>
        <div class="toolbar-right">
          <label class="field-label">
            <span>{{ t('resource.clusterMgmt.nodeList.searchField') }}</span>
            <HNBSelectInput :options="searchFieldOptions" model-value="name" />
          </label>
          <input
            v-model="keyword"
            class="keyword-input"
            type="text"
            :placeholder="t('resource.clusterMgmt.nodeList.keywordPlaceholder')"
            @keyup.enter="onSearch"
          />
          <button class="secondary-button" type="button" @click="onSearch">
            {{ t('resource.clusterMgmt.action.query') }}
          </button>
          <button class="secondary-button" type="button" @click="load">
            {{ t('resource.clusterMgmt.action.refresh') }}
          </button>
          <button class="secondary-button" type="button" @click="onComingSoon">
            {{ t('resource.clusterMgmt.nodeList.action.settings') }}
          </button>
        </div>
      </div>

      <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
      <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
      <HNBTable
        v-else
        :columns="columns"
        :data="nodes"
        :pagination="pagination"
        :empty-title="t('resource.clusterMgmt.nodeList.empty')"
        min-width="1080px"
        :aria-label="t('resource.clusterMgmt.nodeList.title')"
        @update:page="onPage"
        @update:page-size="onPageSize"
      />
    </div>
  </ClusterDetailLayout>
</template>

<style scoped>
.node-list-page { display: flex; flex-direction: column; gap: 10px; }
.list-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.toolbar-right { display: flex; align-items: flex-end; gap: 8px; flex-wrap: wrap; }
.field-label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--hnb-color-text-secondary, #5b6675); }
.keyword-input {
  padding: 6px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 200px;
}
.primary-button {
  padding: 7px 16px;
  border: 0;
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-primary, #2f6fed);
  color: #fff;
  cursor: pointer;
  font-size: 13px;
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
:deep(.node-name-link) {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 14px;
  padding: 0;
}
</style>
