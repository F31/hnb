<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell } from '@hnb/ui-kit'
import type { EChartsOption } from 'echarts'
import { getSecurityDashboard } from '../../api/securityApi'
import type { SecurityDashboardData } from '../../api/securityTypes'
import SecurityChart from './SecurityChart.vue'

type DonutEntry = { name: string; value: number; color: string }

const { t } = useI18n()
const dashboard = ref<SecurityDashboardData | null>(null)
const loading = ref(true)
const loadError = ref('')

const colors = {
  blue: '#4f7cff', cyan: '#42c7c7', critical: '#8f1d2c', severe: '#d64545', medium: '#e58a2b',
  low: '#8267d5', negligible: '#3fa66b', unknown: '#9aa4b2', pending: '#8267d5', handled: '#d9468f',
  escape: '#e58a2b', process: '#d64545', environment: '#8267d5', enabled: '#3fa66b',
}

function donutOption(entries: DonutEntry[]): EChartsOption {
  const total = entries.reduce((sum, item) => sum + item.value, 0)
  const values = new Map(entries.map((item) => [item.name, item.value]))
  return {
    animation: false,
    title: { text: String(total), subtext: t('container.security.overview.total'), left: '29%', top: '39%', textAlign: 'center', textStyle: { fontSize: 28 }, subtextStyle: { fontSize: 12 } },
    tooltip: { trigger: 'item', formatter: (item: any) => `${item.name}: ${values.get(item.name) ?? 0}` },
    legend: { orient: 'vertical', right: '6%', top: 'middle', formatter: (name: string) => `${name} (${values.get(name) ?? 0})` },
    series: [{ type: 'pie', radius: ['48%', '68%'], center: ['30%', '52%'], avoidLabelOverlap: true, label: { show: false }, emphasis: { scale: true }, data: entries.map((item) => ({ name: item.name, value: total ? item.value : 1, itemStyle: { color: item.color } })) }],
  }
}

function valueAxis(values: number[]): { max: number; interval?: number } {
  const maximum = Math.max(0, ...values)
  if (maximum <= 1) return { max: 1, interval: 0.2 }
  return { max: Math.ceil(maximum * 1.2) }
}

const imageOption = computed<EChartsOption>(() => donutOption([
  { name: t('container.security.overview.image.private'), value: dashboard.value?.images.private ?? 0, color: colors.blue },
  { name: t('container.security.overview.image.public'), value: dashboard.value?.images.public ?? 0, color: colors.cyan },
]))

const vulnerabilityOption = computed<EChartsOption>(() => donutOption([
  { name: t('container.security.overview.vulnerability.critical'), value: dashboard.value?.vulnerabilities.critical ?? 0, color: colors.critical },
  { name: t('container.security.overview.vulnerability.severe'), value: dashboard.value?.vulnerabilities.severe ?? 0, color: colors.severe },
  { name: t('container.security.overview.vulnerability.medium'), value: dashboard.value?.vulnerabilities.medium ?? 0, color: colors.medium },
  { name: t('container.security.overview.vulnerability.low'), value: dashboard.value?.vulnerabilities.low ?? 0, color: colors.low },
  { name: t('container.security.overview.vulnerability.negligible'), value: dashboard.value?.vulnerabilities.negligible ?? 0, color: colors.negligible },
  { name: t('container.security.overview.vulnerability.unknown'), value: dashboard.value?.vulnerabilities.unknown ?? 0, color: colors.unknown },
]))

const runtimeOption = computed<EChartsOption>(() => {
  const runtime = dashboard.value?.runtimeEvents
  const rows = runtime ? [runtime.containerEscape, runtime.processAnomaly, runtime.environmentDetection] : [{ pending: 0, handled: 0, ignored: 0 }, { pending: 0, handled: 0, ignored: 0 }, { pending: 0, handled: 0, ignored: 0 }]
  const values = rows.flatMap((item) => [item.pending, item.handled, item.ignored])
  return {
    animation: false, tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } }, legend: { right: 8, top: 0 }, grid: { left: 48, right: 18, top: 42, bottom: 52 },
    xAxis: { type: 'category', data: [t('container.security.overview.runtime.escape'), t('container.security.overview.runtime.process'), t('container.security.overview.runtime.environment')], axisLabel: { interval: 0 } },
    yAxis: { type: 'value', min: 0, ...valueAxis(values), splitLine: { lineStyle: { type: 'dashed' } } },
    series: [
      { name: t('container.security.overview.runtime.pending'), type: 'bar', data: rows.map((item) => item.pending), itemStyle: { color: colors.pending } },
      { name: t('container.security.overview.runtime.handled'), type: 'bar', data: rows.map((item) => item.handled), itemStyle: { color: colors.handled } },
      { name: t('container.security.overview.runtime.ignored'), type: 'bar', data: rows.map((item) => item.ignored), itemStyle: { color: colors.unknown } },
    ],
  }
})

const protectionOption = computed<EChartsOption>(() => {
  const protection = dashboard.value?.protection
  const enabled = [protection?.totalClusters.enabled ?? 0, protection?.protectedClusterNodes.enabled ?? 0]
  const disabled = [protection?.totalClusters.disabled ?? 0, protection?.protectedClusterNodes.disabled ?? 0]
  return {
    animation: false, tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } }, legend: { right: 8, top: 0 }, grid: { left: 48, right: 18, top: 42, bottom: 52 },
    xAxis: { type: 'category', data: [t('container.security.overview.protection.clusters'), t('container.security.overview.protection.nodes')], axisLabel: { interval: 0 } },
    yAxis: { type: 'value', min: 0, ...valueAxis([...enabled, ...disabled]), splitLine: { lineStyle: { type: 'dashed' } } },
    series: [
      { name: t('container.security.overview.protection.enabled'), type: 'bar', data: enabled, itemStyle: { color: colors.enabled } },
      { name: t('container.security.overview.protection.disabled'), type: 'bar', data: disabled, itemStyle: { color: colors.unknown } },
    ],
  }
})

const trendOption = computed<EChartsOption>(() => {
  const rows = dashboard.value?.trend ?? []
  const values = rows.flatMap((item) => [item.containerEscape, item.processAnomaly, item.environmentDetection])
  return {
    animation: false, tooltip: { trigger: 'axis' }, legend: { right: 8, top: 0 }, grid: { left: 48, right: 18, top: 42, bottom: 36 },
    xAxis: { type: 'category', boundaryGap: false, data: rows.map((item) => item.date) },
    yAxis: { type: 'value', min: 0, ...valueAxis(values), splitLine: { lineStyle: { type: 'dashed' } } },
    series: [
      { name: t('container.security.overview.runtime.escape'), type: 'line', showSymbol: true, data: rows.map((item) => item.containerEscape), itemStyle: { color: colors.escape }, lineStyle: { color: colors.escape, width: 2 } },
      { name: t('container.security.overview.runtime.process'), type: 'line', showSymbol: true, data: rows.map((item) => item.processAnomaly), itemStyle: { color: colors.process }, lineStyle: { color: colors.process, width: 2 } },
      { name: t('container.security.overview.runtime.environment'), type: 'line', showSymbol: true, data: rows.map((item) => item.environmentDetection), itemStyle: { color: colors.environment }, lineStyle: { color: colors.environment, width: 2 } },
    ],
  }
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    dashboard.value = await getSecurityDashboard()
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('container.security.overview.loadError')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <HNBPageShell :title="t('container.security.overview.title')">
    <p v-if="loading" class="panel-status" role="status">{{ t('container.security.loading') }}</p>
    <p v-else-if="loadError" class="panel-error" role="alert">{{ loadError }}</p>
    <div v-else class="dashboard-grid">
      <section class="chart-card"><h3>{{ t('container.security.overview.image.title') }}</h3><SecurityChart :option="imageOption" :ariaLabel="t('container.security.overview.image.title')" /></section>
      <section class="chart-card"><h3>{{ t('container.security.overview.vulnerability.title') }}</h3><SecurityChart :option="vulnerabilityOption" :ariaLabel="t('container.security.overview.vulnerability.title')" /></section>
      <section class="chart-card"><h3>{{ t('container.security.overview.runtime.title') }}</h3><SecurityChart :option="runtimeOption" :ariaLabel="t('container.security.overview.runtime.title')" /></section>
      <section class="chart-card"><h3>{{ t('container.security.overview.protection.title') }}</h3><SecurityChart :option="protectionOption" :ariaLabel="t('container.security.overview.protection.title')" /></section>
      <section class="chart-card chart-card--wide"><h3>{{ t('container.security.overview.trend.title') }}</h3><SecurityChart :option="trendOption" :ariaLabel="t('container.security.overview.trend.title')" height="300px" /></section>
    </div>
  </HNBPageShell>
</template>

<style scoped>
.panel-status{color:var(--hnb-color-text-secondary);font-size:13px}.panel-error{margin:0;padding:10px 12px;border-radius:var(--hnb-radius-sm);color:var(--hnb-color-status-danger);background:var(--hnb-color-status-danger-surface)}.dashboard-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.chart-card{min-width:0;padding:16px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg-surface);box-shadow:var(--hnb-shadow-1)}.chart-card h3{margin:0 0 8px;color:var(--hnb-color-text-primary);font-size:15px;font-weight:600}.chart-card--wide{grid-column:1/-1}@media(max-width:768px){.dashboard-grid{grid-template-columns:1fr}.chart-card--wide{grid-column:auto}}
</style>
