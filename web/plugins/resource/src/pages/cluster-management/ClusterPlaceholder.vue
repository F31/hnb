<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>

<template>
  <section class="cluster-placeholder">
    <div class="placeholder-card">
      <h1>{{ t('resource.clusterMgmt.title') }}</h1>
      <p class="placeholder-hint">{{ t('resource.clusterMgmt.placeholder.disabled') }}</p>
    </div>
  </section>
</template>

<style scoped>
.cluster-placeholder {
  display: flex;
  justify-content: center;
  padding: var(--hnb-space-xl);
}
.placeholder-card {
  max-width: 480px;
  text-align: center;
  padding: var(--hnb-space-xl);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  background: var(--hnb-color-bg-surface);
}
.placeholder-card h1 {
  margin: 0 0 var(--hnb-space-md);
  font-size: var(--hnb-font-size-title);
}
.placeholder-hint {
  margin: 0;
  color: var(--hnb-color-text-secondary);
  font-size: var(--hnb-font-size-body);
}
</style>