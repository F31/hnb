<script setup lang="ts">
import type { HNBButtonSize, HNBButtonVariant } from '../types'

const props = withDefaults(defineProps<{
  variant?: HNBButtonVariant
  size?: HNBButtonSize
  disabled?: boolean
  loading?: boolean
  type?: 'button' | 'submit' | 'reset'
  disabledReason?: string
}>(), {
  variant: 'secondary',
  size: 'medium',
  disabled: false,
  loading: false,
  type: 'button',
})
</script>

<template>
  <button
    class="hnb-button"
    :class="[`hnb-button--${variant}`, `hnb-button--${size}`, { 'hnb-button--loading': loading }]"
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading"
    :aria-disabled="disabled || loading"
    :aria-description="disabled && disabledReason ? disabledReason : undefined"
    :title="disabled && disabledReason ? disabledReason : undefined"
  >
    <span v-if="loading" class="hnb-button__spinner" aria-hidden="true" />
    <span :class="{ 'hnb-button__content--hidden': loading }"><slot /></span>
  </button>
</template>

<style scoped>
.hnb-button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--hnb-space-xs);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  font-weight: var(--hnb-font-weight-semibold);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--hnb-duration-fast), border-color var(--hnb-duration-fast), color var(--hnb-duration-fast);
}
.hnb-button:disabled { cursor: not-allowed; opacity: 0.55; }
.hnb-button:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }
.hnb-button--small { height: 28px; padding: 0 var(--hnb-space-sm); font-size: var(--hnb-font-size-caption); }
.hnb-button--medium { height: 34px; padding: 0 var(--hnb-space-md); font-size: var(--hnb-font-size-body); }
.hnb-button--large { height: 40px; padding: 0 var(--hnb-space-lg); font-size: var(--hnb-font-size-body); }
.hnb-button--primary { background: var(--hnb-color-primary); border-color: var(--hnb-color-primary); color: var(--hnb-color-text-on-accent); }
.hnb-button--primary:hover:not(:disabled) { background: var(--hnb-color-primary-hover); border-color: var(--hnb-color-primary-hover); }
.hnb-button--secondary { background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); }
.hnb-button--secondary:hover:not(:disabled) { border-color: var(--hnb-color-primary); color: var(--hnb-color-primary); }
.hnb-button--ghost { background: transparent; border-color: transparent; color: var(--hnb-color-text-secondary); }
.hnb-button--ghost:hover:not(:disabled) { background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.hnb-button--danger { background: var(--hnb-color-status-danger); border-color: var(--hnb-color-status-danger); color: var(--hnb-color-text-on-accent); }
.hnb-button--loading { pointer-events: none; }
.hnb-button__spinner {
  width: 14px;
  height: 14px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: hnb-spin 0.6s linear infinite;
}
.hnb-button__content--hidden { visibility: hidden; width: 0; overflow: hidden; }
@keyframes hnb-spin {
  to { transform: rotate(360deg); }
}
@media (prefers-reduced-motion: reduce) {
  .hnb-button__spinner { animation: none; }
}
</style>
