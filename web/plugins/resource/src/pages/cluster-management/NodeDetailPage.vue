<script setup lang="ts">
/**
 * NodeDetailPage — 节点详情路由入口（OpenSpec node-detail）。
 * 提供 nodeId 给 NodeDetailLayout 与页签组件；根据路由后缀渲染对应页签内容。
 * 6 个子路由共享本组件（basic/monitoring/disks/nics/pods/virtual-machines）。
 */
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ClusterDetailLayout from './components/ClusterDetailLayout.vue'
import NodeDetailLayout from './components/NodeDetailLayout.vue'
import NodeBasicInfoTab from './components/NodeBasicInfoTab.vue'
import NodeMonitoringTab from './components/NodeMonitoringTab.vue'
import NodeDisksTab from './components/NodeDisksTab.vue'
import NodeNicsTab from './components/NodeNicsTab.vue'
import NodePodsTab from './components/NodePodsTab.vue'
import NodeVirtualMachinesTab from './components/NodeVirtualMachinesTab.vue'
import { provideNodeDetailId } from './composables/useNodeDetailContext'

const route = useRoute()
const nodeId = String(route.params.nodeId ?? '')
provideNodeDetailId(nodeId)

const activeTab = computed(() => {
  const seg = route.path.split('/').filter(Boolean).pop() ?? 'basic'
  return seg
})
</script>

<template>
  <ClusterDetailLayout>
    <NodeDetailLayout>
      <NodeBasicInfoTab v-if="activeTab === 'basic'" />
      <NodeMonitoringTab v-else-if="activeTab === 'monitoring'" />
      <NodeDisksTab v-else-if="activeTab === 'disks'" />
      <NodeNicsTab v-else-if="activeTab === 'nics'" />
      <NodePodsTab v-else-if="activeTab === 'pods'" />
      <NodeVirtualMachinesTab v-else-if="activeTab === 'virtual-machines'" />
      <NodeBasicInfoTab v-else />
    </NodeDetailLayout>
  </ClusterDetailLayout>
</template>
