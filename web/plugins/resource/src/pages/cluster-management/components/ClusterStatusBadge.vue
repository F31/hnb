<script setup lang="ts">
/**
 * ClusterStatusBadge — 集群状态徽章（V2.5 §11.3 字典驱动，统一语义色）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { StatusBadge } from '@hnb/ui-kit'
import type { ClusterStatus } from '../types/cluster'
import { clusterStatusMeta } from '../schemas/cluster.status'

const props = defineProps<{ status: ClusterStatus }>()
const { t } = useI18n()

const meta = computed(() => clusterStatusMeta(props.status))
const label = computed(() => t(`resource.clusterMgmt.status.${props.status}`))
</script>

<template>
  <StatusBadge :label="label" :semantic="meta.semantic" />
</template>
