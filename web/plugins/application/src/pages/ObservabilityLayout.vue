import { HNBErrorBoundary } from '@hnb/ui-kit'
<script setup lang="ts">
</script>

<template>
  <div>
    <h1>{{ $t('application.observability.title') }}</h1>
    <nav class="sub-nav">
      <router-link to="/application/observability/analysis">{{ $t('application.observability.analysis') }}</router-link>
      <router-link to="/application/observability/topology">{{ $t('application.observability.topology') }}</router-link>
      <router-link to="/application/observability/guard">{{ $t('application.observability.guard') }}</router-link>
      <router-link to="/application/observability/timeline">{{ $t('application.observability.timeline') }}</router-link>
    </nav>
    <HNBErrorBoundary>
      <router-view />
    </HNBErrorBoundary>
  </div>
</template>

<style scoped>
.sub-nav { display: flex; gap: 8px; margin: 16px 0; }
.sub-nav a { padding: 8px 16px; background: var(--hnb-color-bg-elevated, #171d31); border-radius: 6px; color: var(--hnb-color-text-secondary, #a9b2c2); text-decoration: none; }
.sub-nav a.router-link-active { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
</style>
