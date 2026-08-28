<script setup lang="ts">
/**
 * NodeDetailRedirect — 节点详情入口：`/nodes/:nodeId` 默认跳转基础配置页。
 */
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

onMounted(() => {
  const clusterId = String(route.params.clusterId ?? '')
  const nodeId = String(route.params.nodeId ?? '')
  router.replace(
    `/resource/clusters/${encodeURIComponent(clusterId)}/nodes/${encodeURIComponent(nodeId)}/basic`,
  )
})
</script>

<template>
  <div class="node-detail-redirect" role="status">{{ t('resource.clusterMgmt.nodeDetail.loading') }}</div>
</template>

<style scoped>
.node-detail-redirect {
  padding: 48px 16px;
  text-align: center;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
}
</style>
