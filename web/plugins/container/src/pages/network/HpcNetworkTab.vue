<script setup lang="ts">
/**
 * HpcNetworkTab — 高性能网络（RDMA/RoCE 资源池，只读）。
 * 展示资源层已开通的高性能网络资源池；工作负载创建时勾选使用（消费池内网卡准入）。
 */
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { StatusBadge } from '@hnb/ui-kit'
import { containerCniFeatureAvailable, getRdmaPoolInfo, type RdmaPoolInfo } from '../../api/containerNetworkApi'

const { t } = useI18n()
const available = containerCniFeatureAvailable('rdma')
const pool = ref<RdmaPoolInfo | null>(null)

onMounted(async () => {
  if (!available) return
  pool.value = await getRdmaPoolInfo()
})
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.hpc')">
    <p v-if="!available" class="panel-status" role="status">{{ t('container.network.hpc.notSupported') }}</p>
    <template v-else>
      <div v-if="pool" class="pool-card">
        <div class="pool-row">
          <span class="pool-label">{{ t('container.network.hpc.poolName') }}</span>
          <span class="pool-value">{{ pool.poolName }}</span>
        </div>
        <div class="pool-row">
          <span class="pool-label">{{ t('container.network.hpc.availableNodes') }}</span>
          <span class="pool-value">{{ pool.availableNodes }}</span>
        </div>
        <div class="pool-row">
          <span class="pool-label">{{ t('container.network.hpc.enabled') }}</span>
          <StatusBadge
            :label="pool.rdmaEnabled ? t('container.network.hpc.enabledOn') : t('container.network.hpc.enabledOff')"
            :semantic="pool.rdmaEnabled ? 'success' : 'default'"
          />
        </div>
      </div>
      <p class="readonly-hint">{{ t('container.network.hpc.hint') }}</p>
    </template>
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.pool-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  max-width: 480px;
}
.pool-row { display: flex; align-items: center; justify-content: space-between; font-size: 14px; }
.pool-label { color: var(--hnb-color-text-secondary, #5b6675); }
.pool-value { color: var(--hnb-color-text-primary, #12172a); font-weight: 600; }
.readonly-hint { margin: 0; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
</style>
