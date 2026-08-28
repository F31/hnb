<script setup lang="ts">
/**
 * NodeVirtualMachinesTab — 节点详情 > 虚拟机（OpenSpec node-detail 边界）。
 * 当前工程未提供虚拟机列表能力：保留页签与路由，展示标准空态，不臆造字段或操作。
 */
import { useI18n } from 'vue-i18n'
import { HNBPageState } from '@hnb/ui-kit'

const { t } = useI18n()
</script>

<template>
  <HNBPageState
    state="empty"
    :title="t('resource.clusterMgmt.placeholder.notEnabled')"
    :description="t('resource.clusterMgmt.nodeDetail.virtualMachines.notEnabled')"
  />
</template>
