<script setup lang="ts">
import { HNBErrorBoundary } from '@hnb/ui-kit'
</script>

<template>
  <div class="container-plugin">
    <HNBErrorBoundary>
      <router-view />
    </HNBErrorBoundary>
  </div>
</template>

<style scoped>
.container-plugin {
  height: 100%;
  min-height: 0;
  overflow-y: auto;
  padding: 16px;
  scrollbar-width: thin;
  scrollbar-color: var(--hnb-color-border) transparent;
}
</style>
