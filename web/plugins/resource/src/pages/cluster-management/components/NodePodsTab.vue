<script setup lang="ts">
/**
 * NodePodsTab — 节点详情 > 容器组（OpenSpec node-detail）。
 * 名称查询、刷新、分页；列：名称、状态、命名空间、Pod IP、节点 IP、创建时间、操作。
 * 操作「查看 YAML」以只读代码视图展示资源 YAML（纯文本）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBTable, HNBTableActions, HNBDialog, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTablePagination, HNBTableAction } from '@hnb/ui-kit'
import { getNodePods } from '../api/nodeApi'
import { useClusterDetailId } from '../composables/useClusterDetailContext'
import { useNodeDetailId } from '../composables/useNodeDetailContext'
import SectionHeader from './SectionHeader.vue'
import type { NodePod } from '../types/node'

const { t } = useI18n()
const clusterId = useClusterDetailId()
const nodeId = useNodeDetailId()

const pods = ref<NodePod[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

const selectedYaml = ref('')
const yamlVisible = ref(false)

const pagination = computed<HNBTablePagination>(() => ({
  page: page.value,
  pageSize: pageSize.value,
  total: total.value,
}))

function statusBadge(pod: NodePod): { label: string; semantic: `success` | `error` | `default` } {
  const map: Record<string, `success` | `error` | `default`> = {
    running: 'success',
    pending: 'default',
    failed: 'error',
    succeeded: 'default',
  }
  return { label: t(`resource.clusterMgmt.nodeDetail.pod.status.${pod.status}`), semantic: map[pod.status] ?? 'default' }
}

const columns = computed<HNBTableColumn<NodePod>[]>(() => [
  { key: 'name', title: t('resource.clusterMgmt.nodeDetail.pod.name'), render: (row) => row.name || '--' },
  {
    key: 'status',
    title: t('resource.clusterMgmt.nodeDetail.pod.statusHeader'),
    render: (row) => {
      const b = statusBadge(row)
      return h(StatusBadge, { label: b.label, semantic: b.semantic })
    },
  },
  { key: 'namespace', title: t('resource.clusterMgmt.nodeDetail.pod.namespace'), render: (row) => row.namespace || '--' },
  { key: 'podIp', title: t('resource.clusterMgmt.nodeDetail.pod.podIp'), render: (row) => row.podIp || '--' },
  { key: 'nodeIp', title: t('resource.clusterMgmt.nodeDetail.pod.nodeIp'), render: (row) => row.nodeIp || '--' },
  { key: 'createdAt', title: t('resource.clusterMgmt.nodeDetail.pod.createdAt'), render: (row) => row.createdAt || '--' },
  {
    key: 'actions',
    title: t('resource.clusterMgmt.nodeDetail.pod.actions'),
    render: (row) => {
      const actions: HNBTableAction[] = [
        { label: t('resource.clusterMgmt.nodeDetail.pod.viewYaml'), key: 'viewYaml' },
      ]
      return h(HNBTableActions, {
        actions,
        onAction: (key: string) => {
          if (key === 'viewYaml') openYaml(row)
        },
      })
    },
  },
])

function openYaml(row: NodePod): void {
  selectedYaml.value = row.yaml ?? ''
  yamlVisible.value = true
}

async function load(): Promise<void> {
  if (!clusterId || !nodeId) return
  loading.value = true
  error.value = ''
  try {
    const res = await getNodePods(clusterId, nodeId, {
      page: page.value,
      pageSize: pageSize.value,
      keyword: keyword.value,
    })
    pods.value = res.items
    total.value = res.total
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    pods.value = []
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
  <section class="node-pods" :aria-label="t('resource.clusterMgmt.aria.nodePods')">
    <SectionHeader :title="t('resource.clusterMgmt.nodeDetail.pod.title')" />

    <div class="pod-toolbar">
      <label class="keyword-field">
        <span>{{ t('resource.clusterMgmt.nodeDetail.pod.keyword') }}</span>
        <input
          v-model="keyword"
          type="text"
          :placeholder="t('resource.clusterMgmt.nodeDetail.pod.keywordPlaceholder')"
          @keyup.enter="onSearch"
        />
      </label>
      <button class="secondary-button" type="button" @click="onSearch">
        {{ t('resource.clusterMgmt.action.query') }}
      </button>
      <button class="secondary-button" type="button" @click="load">
        {{ t('resource.clusterMgmt.action.refresh') }}
      </button>
    </div>

    <p v-if="loading" class="panel-status" role="status">{{ t('resource.clusterMgmt.common.loading') }}</p>
    <div v-else-if="error" class="panel-status error" role="alert">{{ error }}</div>
    <HNBTable
      v-else
      :columns="columns"
      :data="pods"
      :pagination="pagination"
      :empty-title="t('resource.clusterMgmt.nodeDetail.pod.empty')"
      min-width="960px"
      :aria-label="t('resource.clusterMgmt.nodeDetail.pod.title')"
      @update:page="onPage"
      @update:page-size="onPageSize"
    />

    <HNBDialog v-model="yamlVisible" :title="t('resource.clusterMgmt.nodeDetail.pod.yamlTitle')">
      <pre class="yaml-view">{{ selectedYaml }}</pre>
    </HNBDialog>
  </section>
</template>

<style scoped>
.node-pods { display: flex; flex-direction: column; gap: 10px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.panel-status.error { color: var(--hnb-color-status-danger, #f04438); }
.pod-toolbar { display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap; }
.keyword-field { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--hnb-color-text-secondary, #5b6675); }
.keyword-field input {
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
.yaml-view {
  margin: 0;
  padding: 12px;
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  color: var(--hnb-color-text-primary, #12172a);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 420px;
  overflow: auto;
}
</style>
