<script setup lang="ts">
/**
 * NetworkManagementPage — 资源 > 网络（运维视角）。
 * 三个页签：容器子网管理 / IP 资源统计 / 网段申请审批。
 * 纯管控原则：网段创建与分配权收在本页，容器层仅可选用或申请。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import SubnetsTab from './components/SubnetsTab.vue'
import IpStatsTab from './components/IpStatsTab.vue'
import SubnetRequestsTab from './components/SubnetRequestsTab.vue'

const { t } = useI18n()
const activeTab = ref('subnets')

const tabs = [
  { key: 'subnets', label: 'subnets' },
  { key: 'ipStats', label: 'ipStats' },
  { key: 'requests', label: 'requests' },
]
</script>

<template>
  <div class="network-page">
    <header class="page-header">
      <h2 class="page-title">{{ t('resource.clusterMgmt.network.title') }}</h2>
    </header>

    <nav class="network-tabs" role="tablist" :aria-label="t('resource.clusterMgmt.network.title')">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="network-tab"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        {{ t(`resource.clusterMgmt.network.tab.${tab.label}`) }}
      </button>
    </nav>

    <SubnetsTab v-if="activeTab === 'subnets'" />
    <IpStatsTab v-else-if="activeTab === 'ipStats'" />
    <SubnetRequestsTab v-else />
  </div>
</template>

<style scoped>
.network-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 18px 20px;
}
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.network-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
  flex-wrap: wrap;
}
.network-tab {
  position: relative;
  padding: 8px 16px;
  border: 0;
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
  cursor: pointer;
}
.network-tab:hover { color: var(--hnb-color-primary, #2f6fed); }
.network-tab.active { color: var(--hnb-color-primary, #2f6fed); font-weight: 600; }
.network-tab.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
</style>
