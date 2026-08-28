<script setup lang="ts">
withDefaults(defineProps<{
  message: string
  politeness?: 'polite' | 'assertive'
}>(), { politeness: 'polite' })
</script>

<template>
  <div class="hnb-live-region" :aria-live="politeness" aria-atomic="true">{{ message }}</div>
</template>

<style scoped>
.hnb-live-region {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>
