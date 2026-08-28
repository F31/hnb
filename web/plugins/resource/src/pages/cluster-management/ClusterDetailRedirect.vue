<script setup lang="ts">
/**
 * ClusterDetailRedirect — 集群详情壳层入口路由：`/resource/clusters/:clusterId`
 * 直接跳转到默认子页 `/overview`（深链接刷新保持上下文）。
 */
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

onMounted(() => {
  const clusterId = String(route.params.clusterId ?? '')
  router.replace(`/resource/clusters/${encodeURIComponent(clusterId)}/overview`)
})
</script>

<template>
  <div class="cluster-detail-redirect" role="status">{{ t('resource.clusterMgmt.detail.loading') }}</div>
</template>

<style scoped>
.cluster-detail-redirect {
  padding: 48px 16px;
  text-align: center;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
}
</style>
