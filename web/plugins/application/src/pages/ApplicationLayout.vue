import { HNBErrorBoundary } from '@hnb/ui-kit'
<script setup lang="ts"></script>

<template>
  <div class="plugin-layout">
    <HNBErrorBoundary>
      <router-view />
    </HNBErrorBoundary>
  </div>
</template>

<style scoped>
.plugin-layout { padding: 16px; }
</style>
