<template>
  <div class="hnb-toolbar">
    <div class="hnb-toolbar__filters"><slot /></div>
    <div v-if="$slots.actions" class="hnb-toolbar__actions"><slot name="actions" /></div>
  </div>
</template>

<style scoped>
.hnb-toolbar { display: flex; justify-content: space-between; gap: var(--hnb-space-md); padding: var(--hnb-space-md); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); box-shadow: var(--hnb-shadow-1); }
.hnb-toolbar__filters, .hnb-toolbar__actions { display: flex; flex-wrap: wrap; gap: var(--hnb-space-sm); align-items: center; }
@media (max-width: 768px) { .hnb-toolbar { flex-direction: column; } }
</style>
