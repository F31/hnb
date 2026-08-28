<script setup lang="ts">
/**
 * ClusterInfoTabs — 集群信息 > 集群详情 / 集群监控 页签（OpenSpec cluster-overview）。
 * 页签高亮与 URL 保持一致；集群监控当前为占位（P2 接入）。
 */
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const basePath = computed(() => route.path.replace(/\/overview$|\/monitoring$/, ''))

const tabs = computed(() => [
  { key: 'overview', label: t('resource.clusterMgmt.detail.tabDetail'), href: `${basePath.value}/overview` },
  ...(import.meta.env.VITE_FEATURE_RESOURCE_CLUSTER_MONITORING === 'true'
    ? [{ key: 'monitoring', label: t('resource.clusterMgmt.detail.tabMonitoring'), href: `${basePath.value}/monitoring` }]
    : []),
])

function isActive(key: string): boolean {
  const active = /\/monitoring$/.test(route.path) ? 'monitoring' : 'overview'
  return key === active
}
</script>

<template>
  <nav class="info-tabs" :aria-label="t('resource.clusterMgmt.aria.clusterInfoTabs')">
    <a
      v-for="tab in tabs"
      :key="tab.key"
      class="info-tab"
      :class="{ active: isActive(tab.key) }"
      :href="tab.href"
      @click.prevent="router.push(tab.href)"
    >
      {{ tab.label }}
    </a>
  </nav>
</template>

<style scoped>
.info-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
  margin-bottom: 16px;
}
.info-tab {
  position: relative;
  padding: 8px 16px;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 15px;
  text-decoration: none;
  cursor: pointer;
}
.info-tab:hover { color: var(--hnb-color-primary, #2f6fed); }
.info-tab.active {
  color: var(--hnb-color-primary, #2f6fed);
  font-weight: 600;
}
.info-tab.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
</style>
