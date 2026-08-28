<template>
  <div class="hnb-action-bar"><slot /></div>
</template>

<style scoped>
.hnb-action-bar { display: flex; justify-content: flex-end; gap: var(--hnb-space-sm); padding: var(--hnb-space-md) 0; }
@media (max-width: 768px) { .hnb-action-bar { justify-content: stretch; flex-direction: column; } }
</style>
