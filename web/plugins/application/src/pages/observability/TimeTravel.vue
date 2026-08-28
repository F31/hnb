<template>
  <section class="stub-page">
    <div class="stub-content">
      <h2>{{ t('observability.timeTravel.title') }}</h2>
      <p>{{ t('observability.timeTravel.desc') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>

<style scoped>
.stub-page { display: flex; align-items: center; justify-content: center; min-height: 400px; }
.stub-content { text-align: center; }
.stub-content h2 { margin: 0 0 8px; font-size: 18px; color: var(--hnb-color-text-primary, #edeff5); }
.stub-content p { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 13px; }
</style>