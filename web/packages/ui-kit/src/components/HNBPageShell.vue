<script setup lang="ts">
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <section class="hnb-page-shell">
    <header class="hnb-page-shell__header">
      <div>
        <h1>{{ title }}</h1>
        <p v-if="description">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="hnb-page-shell__actions"><slot name="actions" /></div>
    </header>
    <div class="hnb-page-shell__body"><slot /></div>
  </section>
</template>

<style scoped>
.hnb-page-shell { display: flex; flex-direction: column; gap: var(--hnb-space-lg); color: var(--hnb-color-text-primary); }
.hnb-page-shell__header { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--hnb-space-md); }
.hnb-page-shell__header h1 { margin: 0; font-size: var(--hnb-font-size-title); font-weight: var(--hnb-font-weight-semibold); }
.hnb-page-shell__header p { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-body); }
.hnb-page-shell__actions { display: flex; gap: var(--hnb-space-sm); align-items: center; }
.hnb-page-shell__body { display: flex; flex-direction: column; gap: var(--hnb-space-md); }
@media (max-width: 768px) { .hnb-page-shell__header { flex-direction: column; } }
</style>
