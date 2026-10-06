<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { HNBPageShell, HNBPageState, HNBTable, MetricCard, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn, HNBTablePagination } from '@hnb/ui-kit'
import { listClusterNodes, listClusters } from './cluster-management/api/clusterApi'
import type { ClusterNodeInfo, ClusterSummary, NodeSummary } from './cluster-management/types/cluster'

type ResourceNode = NodeSummary & {
  clusterId: string
  clusterName: string
  clusterStatus: string
}

const router = useRouter()

const nodes = ref<ResourceNode[]>([])
const clusters = ref<ClusterSummary[]>([])
const loading = ref(true)
const error = ref('')
const keyword = ref('')
const statusFilter = ref('')
const page = ref(1)
const pageSize = ref(10)

const filteredNodes = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return nodes.value.filter((node) => {
    const matchesKeyword = !kw
      || node.name.toLowerCase().includes(kw)
      || node.id.toLowerCase().includes(kw)
      || node.clusterName.toLowerCase().includes(kw)
      || node.managementIp?.toLowerCase().includes(kw)
    const matchesStatus = !statusFilter.value || node.status === statusFilter.value
    return matchesKeyword && matchesStatus
  })
})

const pagedNodes = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredNodes.value.slice(start, start + pageSize.value)
})

const pagination = computed<HNBTablePagination>(() => ({
  page: page.value,
  pageSize: pageSize.value,
  total: filteredNodes.value.length,
}))

const readyCount = computed(() => nodes.value.filter((node) => isReady(node.status)).length)
const notReadyCount = computed(() => nodes.value.filter((node) => !isReady(node.status)).length)

const columns = computed<HNBTableColumn<ResourceNode>[]>(() => [
  {
    key: 'name',
    title: '节点名称',
    render: (row) => h('button', { type: 'button', class: 'node-link', onClick: () => goDetail(row) }, row.name || row.id),
  },
  { key: 'clusterName', title: '所属集群', render: (row) => row.clusterName || row.clusterId },
  {
    key: 'status',
    title: '状态',
    render: (row) => h(StatusBadge, { label: statusLabel(row.status), semantic: isReady(row.status) ? 'success' : 'error' }),
  },
  { key: 'role', title: '角色', render: (row) => roleLabel(row.role) },
  { key: 'type', title: '类型', render: (row) => row.type || '--' },
  { key: 'managementIp', title: '管理 IP', render: (row) => row.managementIp || '--' },
  { key: 'cpuCores', title: 'CPU', render: (row) => String(row.cpuCores || 0) },
  { key: 'memoryGiB', title: '内存 GiB', render: (row) => String(row.memoryGiB || 0) },
  // 该列实际渲染 lastHeartbeatAt（后端无节点创建时间字段），列名如实标注。
  { key: 'createdAt', title: '最近心跳', render: (row) => row.createdAt || '--' },
])

function isReady(status: string): boolean {
  return status === 'Ready' || status === 'running' || status === 'RUNNING'
}

function statusLabel(status: string): string {
  const map: Record<string, string> = {
    Ready: '运行中',
    running: '运行中',
    RUNNING: '运行中',
    NotReady: '异常',
    Unknown: '未知',
  }
  return map[status] ?? status
}

function roleLabel(role: string): string {
  const map: Record<string, string> = {
    'control-plane': '控制节点',
    controller: '控制节点',
    worker: '工作节点',
    edge: '边缘节点',
  }
  return map[role] ?? role
}

function parseCpu(value: string): number {
  if (!value) return 0
  if (value.endsWith('m')) return Math.round(Number(value.slice(0, -1)) / 1000)
  return Math.round(Number.parseFloat(value)) || 0
}

function parseMemoryGiB(value: string): number {
  if (!value) return 0
  const match = value.match(/^([\d.]+)\s*([KMGTP]i?)?$/i)
  if (!match) return 0
  const amount = Number.parseFloat(match[1])
  const unit = (match[2] ?? 'Gi').toLowerCase()
  const factor: Record<string, number> = {
    ki: 1 / 1024 / 1024,
    k: 1 / 1024 / 1024,
    mi: 1 / 1024,
    m: 1 / 1024,
    gi: 1,
    g: 1,
    ti: 1024,
    t: 1024,
  }
  return Math.round(amount * (factor[unit] ?? 1) * 100) / 100
}

function mapNode(cluster: ClusterSummary, node: ClusterNodeInfo): ResourceNode {
  return {
    id: node.nodeId,
    name: node.name,
    role: node.role,
    type: cluster.kind,
    status: node.status,
    managementIp: node.ipAddress,
    cpuCores: parseCpu(node.cpuAllocatable),
    memoryGiB: parseMemoryGiB(node.memoryAllocatable),
    createdAt: node.lastHeartbeatAt,
    clusterId: cluster.clusterId,
    clusterName: cluster.displayName,
    clusterStatus: cluster.status,
  }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const clusterRes = await listClusters({ page: 1, pageSize: 100 })
    clusters.value = clusterRes.items ?? []
    const entries = await Promise.all(clusters.value.map(async (cluster) => {
      try {
        const res = await listClusterNodes(cluster.clusterId, { page: 1, pageSize: 200 })
        return (res.items ?? []).map((node) => mapNode(cluster, node))
      } catch {
        return [] as ResourceNode[]
      }
    }))
    nodes.value = entries.flat()
    page.value = 1
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    clusters.value = []
    nodes.value = []
  } finally {
    loading.value = false
  }
}

function search(): void {
  page.value = 1
}

function onPage(next: number): void {
  page.value = next
}

function onPageSize(next: number): void {
  pageSize.value = next
  page.value = 1
}

function goDetail(node: ResourceNode): void {
  router.push(`/resource/clusters/${encodeURIComponent(node.clusterId)}/nodes/${encodeURIComponent(node.id)}/basic`)
}

onMounted(load)
</script>

<template>
  <HNBPageShell title="节点资源" description="跨集群查看控制节点、工作节点与边缘节点状态。">
    <div class="metric-grid">
      <MetricCard title="接入集群" :value="clusters.length" />
      <MetricCard title="节点总数" :value="nodes.length" />
      <MetricCard title="运行中" :value="readyCount" />
      <MetricCard title="异常/未知" :value="notReadyCount" />
    </div>

    <div class="node-toolbar">
      <input
        v-model="keyword"
        class="keyword-input"
        type="search"
        placeholder="搜索节点、集群或 IP"
        @keyup.enter="search"
      />
      <select v-model="statusFilter" class="status-select" @change="search">
        <option value="">全部状态</option>
        <option value="Ready">运行中</option>
        <option value="NotReady">异常</option>
        <option value="Unknown">未知</option>
      </select>
      <button class="secondary-button" type="button" @click="search">查询</button>
      <button class="secondary-button" type="button" @click="load">刷新</button>
    </div>

    <HNBPageState
      v-if="loading"
      state="loading"
      title="正在加载节点资源"
    />
    <HNBPageState
      v-else-if="error"
      state="error"
      title="节点资源加载失败"
      :description="error"
      action-text="重试"
      @action="load"
    />
    <HNBPageState
      v-else-if="clusters.length === 0"
      state="empty"
      title="暂无已接入集群"
      description="请先在集群菜单接入或创建运行时集群，节点资源会在 Agent/CloudCore 上报后显示。"
    />
    <HNBTable
      v-else
      :columns="columns"
      :data="pagedNodes"
      :pagination="pagination"
      row-key="id"
      min-width="1180px"
      aria-label="节点资源列表"
      empty-title="暂无节点资源"
      @update:page="onPage"
      @update:page-size="onPageSize"
    />
  </HNBPageShell>
</template>

<style scoped>
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--hnb-space-md, 16px);
}

.node-toolbar {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm, 8px);
  flex-wrap: wrap;
}

.keyword-input,
.status-select {
  min-height: 34px;
  padding: 0 var(--hnb-space-sm, 8px);
  color: var(--hnb-color-text-primary, #12172a);
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
}

.keyword-input { min-width: 240px; }

.secondary-button {
  min-height: 34px;
  padding: 0 14px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  background: var(--hnb-color-bg-surface, #fff);
  color: var(--hnb-color-text-secondary, #5b6675);
  cursor: pointer;
}

:deep(.node-link) {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
}

@media (max-width: 640px) {
  .metric-grid { grid-template-columns: 1fr 1fr; }
  .keyword-input,
  .status-select,
  .secondary-button { width: 100%; }
}
</style>
