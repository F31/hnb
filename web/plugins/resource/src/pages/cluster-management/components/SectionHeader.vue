<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
  <header class="section-header">
    <span class="section-bar" aria-hidden="true"></span>
    <h3 class="section-title">{{ title }}</h3>
  </header>
</template>

<style scoped>
.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.section-bar {
  width: 3px;
  height: 16px;
  border-radius: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
.section-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--hnb-color-text-primary, #12172a);
}
</style>
