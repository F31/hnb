<script setup lang="ts">
/**
 * AlertSeverityCards — 五级告警严重度摘要（紧急/重要/次要/警告/事件）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitoringAlertCounts } from '../types/clusterMonitoring'

const props = defineProps<{ alerts: MonitoringAlertCounts }>()

const { t } = useI18n()

const cards = computed(() => [
  { key: 'critical', label: t('resource.clusterMgmt.monitoring.alerts.critical'), value: props.alerts.critical, cls: 'critical' },
  { key: 'major', label: t('resource.clusterMgmt.monitoring.alerts.major'), value: props.alerts.major, cls: 'major' },
  { key: 'minor', label: t('resource.clusterMgmt.monitoring.alerts.minor'), value: props.alerts.minor, cls: 'minor' },
  { key: 'warning', label: t('resource.clusterMgmt.monitoring.alerts.warning'), value: props.alerts.warning, cls: 'warning' },
  { key: 'event', label: t('resource.clusterMgmt.monitoring.alerts.event'), value: props.alerts.event, cls: 'event' },
])
</script>

<template>
  <div class="alert-cards" role="group" aria-label="告警摘要">
    <div v-for="card in cards" :key="card.key" class="alert-card" :class="card.cls">
      <span class="alert-card__value">{{ card.value }}</span>
      <span class="alert-card__label">{{ card.label }}</span>
      <span class="alert-card__unit">{{ t('resource.clusterMgmt.monitoring.alerts.unit') }}</span>
    </div>
  </div>
</template>

<style scoped>
.alert-cards {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 12px;
}
.alert-card {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px 14px;
  border-radius: var(--hnb-radius-md, 6px);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
}
.alert-card__value { font-size: 24px; font-weight: 700; color: var(--hnb-color-text-primary, #12172a); }
.alert-card__label { font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.alert-card__unit { font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.alert-card.critical { background: color-mix(in srgb, #f04438 10%, transparent); }
.alert-card.major { background: color-mix(in srgb, #f79009 10%, transparent); }
.alert-card.minor { background: color-mix(in srgb, #2f6fed 8%, transparent); }
.alert-card.warning { background: color-mix(in srgb, #f79009 6%, transparent); }
.alert-card.event { background: var(--hnb-color-bg-elevated, #f6f8fb); }
</style>
