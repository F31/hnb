<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBConfirmation, HNBPageShell, StatusBadge } from '@hnb/ui-kit'
import { listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  getClusterAlertRules,
  getClusterProtectionEnabled,
  getClusterProtectionTopology,
  saveClusterAlertRules,
  setClusterProtectionEnabled,
} from '../../api/securityApi'
import type { ClusterAlertRules, ClusterProtectionTopology } from '../../api/securityTypes'
import NetworkDrawer from '../network/NetworkDrawer.vue'

interface ProtectionRow {
  cluster: ContainerCluster
  topology: ClusterProtectionTopology
  enabled: boolean
}

const { t } = useI18n()
const rows = ref<ProtectionRow[]>([])
const loading = ref(false)
const loadError = ref('')
const searchInput = ref('')
const appliedSearch = ref('')

const filteredRows = computed(() => rows.value.filter((row) => !appliedSearch.value || (row.cluster.display_name || row.cluster.name).toLowerCase().includes(appliedSearch.value.toLowerCase())))

function clusterName(row: ProtectionRow): string {
  return row.cluster.display_name || row.cluster.name
}

function statusLabel(status: string): string {
  if (['online', 'running', 'ready', 'healthy'].includes(status.toLowerCase())) return t('container.security.protection.status.running')
  if (['degraded', 'warning'].includes(status.toLowerCase())) return t('container.security.protection.status.degraded')
  if (['offline', 'stopped', 'failed'].includes(status.toLowerCase())) return t('container.security.protection.status.offline')
  return t('container.security.protection.status.unknown')
}

function statusSemantic(status: string): 'success' | 'warning' | 'error' | 'default' {
  if (['online', 'running', 'ready', 'healthy'].includes(status.toLowerCase())) return 'success'
  if (['degraded', 'warning'].includes(status.toLowerCase())) return 'warning'
  if (['offline', 'stopped', 'failed'].includes(status.toLowerCase())) return 'error'
  return 'default'
}

function formatDate(value?: string): string {
  if (!value) return '--'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const clusters = await listWorkspaceClusters()
    rows.value = await Promise.all(clusters.map(async (cluster) => ({ cluster, topology: await getClusterProtectionTopology(cluster.id), enabled: getClusterProtectionEnabled(cluster.id) })))
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.security.protection.loadError')
  } finally {
    loading.value = false
  }
}

function query(): void {
  appliedSearch.value = searchInput.value.trim()
}

const confirmVisible = ref(false)
const confirmBusy = ref(false)
const confirmError = ref('')
const toggleTarget = ref<ProtectionRow | null>(null)

function requestToggle(row: ProtectionRow): void {
  toggleTarget.value = row
  confirmError.value = ''
  confirmVisible.value = true
}

async function confirmToggle(): Promise<void> {
  if (!toggleTarget.value) return
  confirmBusy.value = true
  confirmError.value = ''
  try {
    const enabled = !toggleTarget.value.enabled
    await setClusterProtectionEnabled(toggleTarget.value.cluster.id, enabled)
    toggleTarget.value.enabled = enabled
    confirmVisible.value = false
  } catch (error) {
    confirmError.value = error instanceof Error ? error.message : String(error)
  } finally {
    confirmBusy.value = false
  }
}

const detailVisible = ref(false)
const detailRow = ref<ProtectionRow | null>(null)
function showDetail(row: ProtectionRow): void { detailRow.value = row; detailVisible.value = true }

const rulesVisible = ref(false)
const rulesBusy = ref(false)
const rulesError = ref('')
const rulesCluster = ref<ProtectionRow | null>(null)
const rules = ref<ClusterAlertRules>({ vulnerabilityLevel: 'severe', runtimeEvents: true, imageVulnerabilities: true, notification: 'console' })

function openRules(row: ProtectionRow): void {
  rulesCluster.value = row
  rules.value = { ...getClusterAlertRules(row.cluster.id) }
  rulesError.value = ''
  rulesVisible.value = true
}

async function saveRules(): Promise<void> {
  if (!rulesCluster.value) return
  rulesBusy.value = true
  rulesError.value = ''
  try {
    await saveClusterAlertRules(rulesCluster.value.cluster.id, rules.value)
    rulesVisible.value = false
  } catch (error) {
    rulesError.value = error instanceof Error ? error.message : String(error)
  } finally {
    rulesBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <HNBPageShell :title="t('container.security.protection.title')">
    <template #actions><a class="help-link" href="https://docs.hnb.example.io/container/security/protection" target="_blank" rel="noopener noreferrer">? {{ t('container.security.protection.help') }}</a></template>
    <div class="toolbar"><input v-model="searchInput" type="search" :placeholder="t('container.security.protection.searchPlaceholder')" @keyup.enter="query"><button type="button" :aria-label="t('container.security.protection.search')" :title="t('container.security.protection.search')" @click="query">⌕</button><button type="button" :aria-label="t('container.security.protection.refresh')" :title="t('container.security.protection.refresh')" @click="load">↻</button></div>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p><p v-if="loading" class="loading" role="status">{{ t('container.security.loading') }}</p>
    <div v-else class="table-wrap"><table class="protection-table"><thead><tr><th>{{ t('container.security.protection.columns.name') }}</th><th>{{ t('container.security.protection.columns.status') }}</th><th>{{ t('container.security.protection.columns.version') }}</th><th>{{ t('container.security.protection.columns.architecture') }}</th><th>{{ t('container.security.protection.columns.os') }}</th><th>{{ t('container.security.protection.columns.nodes') }}</th><th>{{ t('container.security.protection.columns.protection') }}</th><th>{{ t('container.security.protection.columns.createdAt') }}</th><th>{{ t('container.security.protection.columns.actions') }}</th></tr></thead><tbody><tr v-for="row in filteredRows" :key="row.cluster.id"><td><span class="ellipsis" :title="clusterName(row)">{{ clusterName(row) }}</span></td><td><StatusBadge :label="statusLabel(row.cluster.status)" :semantic="statusSemantic(row.cluster.status)" /></td><td>{{ row.topology.version || '--' }}</td><td>{{ row.topology.architecture || '--' }}</td><td><span class="os-cell" :title="row.topology.operatingSystem">{{ row.topology.operatingSystem || '--' }}</span></td><td><span class="stack-line">{{ t('container.security.protection.controlNodes', { count: row.topology.controlPlaneNodes }) }}</span><span class="stack-line">{{ t('container.security.protection.workerNodes', { count: row.topology.workerNodes }) }}</span></td><td><StatusBadge :label="t(`container.security.protection.protectionState.${row.enabled ? 'enabled' : 'disabled'}`)" :semantic="row.enabled ? 'success' : 'default'" /></td><td>{{ formatDate(row.cluster.created_at) }}</td><td><div class="row-actions"><button type="button" @click="requestToggle(row)">{{ t(`container.security.protection.action.${row.enabled ? 'disable' : 'enable'}`) }}</button><button type="button" @click="showDetail(row)">{{ t('container.security.protection.action.detail') }}</button><button type="button" @click="openRules(row)">{{ t('container.security.protection.action.rules') }}</button></div></td></tr><tr v-if="!filteredRows.length"><td colspan="9" class="empty">{{ t('container.security.empty') }}</td></tr></tbody></table></div>

    <NetworkDrawer v-model="detailVisible" :title="t('container.security.protection.detail.title', { name: detailRow ? clusterName(detailRow) : '' })" hide-confirm><dl v-if="detailRow" class="detail-list"><div><dt>{{ t('container.security.protection.columns.protection') }}</dt><dd>{{ t(`container.security.protection.protectionState.${detailRow.enabled ? 'enabled' : 'disabled'}`) }}</dd></div><div><dt>{{ t('container.security.protection.detail.policies') }}</dt><dd>{{ detailRow.enabled ? t('container.security.protection.detail.policyList') : '--' }}</dd></div><div><dt>{{ t('container.security.protection.detail.detections') }}</dt><dd>{{ detailRow.enabled ? t('container.security.protection.detail.detectionList') : '--' }}</dd></div><div><dt>{{ t('container.security.protection.columns.nodes') }}</dt><dd>{{ detailRow.topology.controlPlaneNodes + detailRow.topology.workerNodes }}</dd></div></dl></NetworkDrawer>
    <NetworkDrawer v-model="rulesVisible" :title="t('container.security.protection.rules.title')" :busy="rulesBusy" :error="rulesError" @confirm="saveRules"><form class="rules-form" @submit.prevent="saveRules"><label><span>{{ t('container.security.protection.rules.level') }}</span><select v-model="rules.vulnerabilityLevel"><option value="critical">{{ t('container.security.overview.vulnerability.critical') }}</option><option value="severe">{{ t('container.security.overview.vulnerability.severe') }}</option><option value="medium">{{ t('container.security.overview.vulnerability.medium') }}</option></select></label><label class="check-row"><input v-model="rules.runtimeEvents" type="checkbox"><span>{{ t('container.security.protection.rules.runtime') }}</span></label><label class="check-row"><input v-model="rules.imageVulnerabilities" type="checkbox"><span>{{ t('container.security.protection.rules.image') }}</span></label><label><span>{{ t('container.security.protection.rules.notification') }}</span><select v-model="rules.notification"><option value="console">Console</option><option value="email">Email</option><option value="webhook">Webhook</option></select></label></form></NetworkDrawer>
    <HNBConfirmation v-model="confirmVisible" :title="t(`container.security.protection.confirm.${toggleTarget?.enabled ? 'disableTitle' : 'enableTitle'}`)" :description="t('container.security.protection.confirm.message', { name: toggleTarget ? clusterName(toggleTarget) : '' })" :loading="confirmBusy" :error="confirmError" :confirm-text="t('container.config.action.confirm')" :cancel-text="t('container.config.action.cancel')" :danger="Boolean(toggleTarget?.enabled)" @confirm="confirmToggle" />
  </HNBPageShell>
</template>

<style scoped>
.help-link{color:var(--hnb-color-primary);font-size:13px;text-decoration:none}.toolbar{display:flex;justify-content:flex-end;gap:8px}.toolbar input{width:230px;min-height:34px;padding:6px 10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.toolbar button{width:34px;height:34px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);cursor:pointer}.error,.loading{margin:0;padding:8px 10px;font-size:13px}.error{color:var(--hnb-color-status-danger)}.loading{color:var(--hnb-color-text-secondary)}.table-wrap{overflow-x:auto;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md)}.protection-table{width:100%;min-width:1450px;border-collapse:collapse;table-layout:fixed;font-size:13px}.protection-table th,.protection-table td{padding:11px 12px;border-bottom:1px solid var(--hnb-color-divider);text-align:left;vertical-align:middle}.protection-table th{background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-secondary);white-space:nowrap}.protection-table th:nth-child(1){width:150px}.protection-table th:nth-child(5){width:210px}.protection-table th:nth-child(6){width:130px}.protection-table th:nth-child(8){width:170px}.protection-table th:last-child{width:270px}.ellipsis,.os-cell{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.stack-line{display:block;line-height:1.6}.row-actions{display:flex;gap:10px;white-space:nowrap}.row-actions button{padding:0;border:0;background:transparent;color:var(--hnb-color-primary);cursor:pointer;font-size:13px}.empty{padding:42px!important;text-align:center!important;color:var(--hnb-color-text-tertiary)}.detail-list{display:grid;gap:12px;margin:0}.detail-list div{padding-bottom:10px;border-bottom:1px solid var(--hnb-color-divider)}.detail-list dt{color:var(--hnb-color-text-secondary);font-size:12px}.detail-list dd{margin:5px 0 0;white-space:pre-line}.rules-form{display:flex;flex-direction:column;gap:14px}.rules-form label:not(.check-row){display:grid;grid-template-columns:150px 1fr;align-items:center;gap:12px}.rules-form select{padding:8px 10px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-sm);background:var(--hnb-color-bg-elevated);color:var(--hnb-color-text-primary)}.check-row{display:flex;align-items:center;gap:9px}@media(max-width:560px){.toolbar{flex-wrap:wrap}.toolbar input{width:100%}.rules-form label:not(.check-row){grid-template-columns:1fr}}
</style>
