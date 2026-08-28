<script setup lang="ts">
/**
 * ClusterScaleCards — 集群规模统计：命名空间总数 / 项目总数 / 可调度节点总数。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MetricCard } from '@hnb/ui-kit'
import type { ClusterMonitoringSummary } from '../types/clusterMonitoring'

const props = defineProps<{ summary: ClusterMonitoringSummary }>()

const { t } = useI18n()

const cards = computed(() => [
  { title: t('resource.clusterMgmt.monitoring.scale.namespaces'), value: props.summary.namespaceCount },
  { title: t('resource.clusterMgmt.monitoring.scale.projects'), value: props.summary.projectCount },
  { title: t('resource.clusterMgmt.monitoring.scale.schedulableNodes'), value: props.summary.schedulableNodeCount },
])
</script>

<template>
  <div class="scale-cards" role="group" :aria-label="t('resource.clusterMgmt.aria.clusterScale')">
    <MetricCard
      v-for="card in cards"
      :key="card.title"
      :title="card.title"
      :value="card.value"
    />
  </div>
</template>

<style scoped>
.scale-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}
</style>
