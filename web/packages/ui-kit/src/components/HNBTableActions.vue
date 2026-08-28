<script setup lang="ts">
import HNBButton from './HNBButton.vue'
import type { HNBTableAction } from '../types'

defineProps<{ actions: HNBTableAction[] }>()
const emit = defineEmits<{ action: [key: string] }>()
</script>

<template>
  <div class="hnb-table-actions">
    <HNBButton v-for="action in actions" :key="action.key" :variant="action.variant ?? 'ghost'" size="small" :disabled="action.disabled" @click="emit('action', action.key)">
      {{ action.label }}
    </HNBButton>
  </div>
</template>

<style scoped>
.hnb-table-actions { display: inline-flex; gap: var(--hnb-space-xs); align-items: center; }
</style>
