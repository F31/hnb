<script setup lang="ts">
/**
 * ClusterSummaryCards — 集群概览指标卡组（V2.5 §11.1 MetricCardGroup）。
 * 计数优先采用服务端对当前筛选结果的全量聚合；老服务端未提供 summary
 * 时才退化为当前页统计，确保滚动升级兼容。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MetricCard } from '@hnb/ui-kit'
import type { ClusterListAggregate, ClusterSummary } from '../types/cluster'

const props = defineProps<{ data: ClusterSummary[]; summary?: ClusterListAggregate }>()
const { t } = useI18n()

const total = computed(() => props.summary?.total ?? props.data.length)
const running = computed(() => props.summary?.running ?? props.data.filter((c) => c.status === 'RUNNING').length)
const degraded = computed(() => props.summary?.degraded ?? props.data.filter((c) => c.status === 'DEGRADED').length)
const stale = computed(() => props.summary?.stale ?? props.data.filter((c) => c.status === 'STALE').length)
</script>

<template>
  <div class="cluster-summary-cards">
    <MetricCard :title="t('resource.clusterMgmt.summary.total')" :value="total" />
    <MetricCard :title="t('resource.clusterMgmt.summary.running')" :value="running" />
    <MetricCard :title="t('resource.clusterMgmt.summary.degraded')" :value="degraded" />
    <MetricCard :title="t('resource.clusterMgmt.summary.stale')" :value="stale" />
  </div>
</template>

<style scoped>
.cluster-summary-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--hnb-space-md);
}
@media (max-width: 768px) {
  .cluster-summary-cards {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
