<script setup lang="ts">
/**
 * HNBSkeleton — 骨架屏（V2.5 §18.2：尽量还原目标轮廓，避免跳动）。
 */
withDefaults(defineProps<{
  rows?: number
  /** 是否渲染标题行 */
  title?: boolean
  label?: string
  variant?: 'text' | 'table' | 'detail'
}>(), { rows: 3, title: false, label: 'Loading', variant: 'text' })
</script>

<template>
  <div class="hnb-skeleton" :class="`hnb-skeleton--${variant}`" aria-busy="true" role="status" :aria-label="label">
    <div v-if="title" class="skeleton-line skeleton-title" />
    <div
      v-for="i in rows"
      :key="i"
      class="skeleton-line"
      :style="{ width: i === rows ? '60%' : '100%' }"
    />
  </div>
</template>

<style scoped>
.hnb-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-md) 0;
}
.skeleton-line {
  height: 14px;
  border-radius: var(--hnb-radius-sm);
  background: linear-gradient(
    90deg,
    var(--hnb-color-bg-elevated) 25%,
    var(--hnb-color-border) 50%,
    var(--hnb-color-bg-elevated) 75%
  );
  background-size: 200% 100%;
  animation: hnb-shimmer 1.4s ease infinite;
}
.skeleton-title {
  height: 20px;
  width: 40%;
  margin-bottom: var(--hnb-space-xs);
}
.hnb-skeleton--table .skeleton-line { height: 32px; }
.hnb-skeleton--detail { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.hnb-skeleton--detail .skeleton-title { grid-column: 1 / -1; }
@keyframes hnb-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
@media (prefers-reduced-motion: reduce) {
  .skeleton-line { animation: none; }
}
@media (max-width: 768px) {
  .hnb-skeleton--detail { grid-template-columns: 1fr; }
}
</style>
