<script setup lang="ts">
/**
 * NetworkPage — 容器 > 集群实例 > 网络（使用视角）。
 * 三个页签：服务管理 / 应用路由 / 网段信息。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ServicesTab from './ServicesTab.vue'
import IngressesTab from './IngressesTab.vue'
import SubnetInfoTab from './SubnetInfoTab.vue'
import NetworkPolicyTab from './NetworkPolicyTab.vue'
import QosTab from './QosTab.vue'
import DiagnosisTab from './DiagnosisTab.vue'
import HpcNetworkTab from './HpcNetworkTab.vue'

const { t } = useI18n()
const activeTab = ref('services')

const tabs = [
  { key: 'services', label: 'services' },
  { key: 'ingresses', label: 'ingresses' },
  { key: 'subnets', label: 'subnets' },
  { key: 'policies', label: 'policies' },
  { key: 'qos', label: 'qos' },
  { key: 'hpc', label: 'hpc' },
  { key: 'diagnosis', label: 'diagnosis' },
]
</script>

<template>
  <div class="container-network-page">
    <header class="page-header">
      <h2 class="page-title">{{ t('container.network.title') }}</h2>
    </header>

    <nav class="network-tabs" role="tablist" :aria-label="t('container.network.title')">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="network-tab"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        {{ t(`container.network.tab.${tab.label}`) }}
      </button>
    </nav>

    <ServicesTab v-if="activeTab === 'services'" />
    <IngressesTab v-else-if="activeTab === 'ingresses'" />
    <SubnetInfoTab v-else-if="activeTab === 'subnets'" />
    <NetworkPolicyTab v-else-if="activeTab === 'policies'" />
    <QosTab v-else-if="activeTab === 'qos'" />
    <HpcNetworkTab v-else-if="activeTab === 'hpc'" />
    <DiagnosisTab v-else />
  </div>
</template>

<style scoped>
.container-network-page {
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
