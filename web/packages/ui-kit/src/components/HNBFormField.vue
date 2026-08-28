<script setup lang="ts">
import { computed, provide } from 'vue'
import { HNB_FORM_FIELD_INJECTION_KEY } from '../types'

const props = defineProps<{
  label: string
  inputId?: string
  help?: string
  error?: string
  required?: boolean
}>()

const ariaDescribedBy = computed(() => {
  if (!props.inputId) return undefined
  if (props.error) return `${props.inputId}-error`
  if (props.help) return `${props.inputId}-help`
  return undefined
})

provide(HNB_FORM_FIELD_INJECTION_KEY, { ariaDescribedBy })
</script>

<template>
  <label class="hnb-form-field" :for="inputId">
    <span class="hnb-form-field__label">{{ label }}<span v-if="required" aria-hidden="true"> *</span></span>
    <slot />
    <span v-if="error" :id="inputId ? `${inputId}-error` : undefined" class="hnb-form-field__error" role="alert">{{ error }}</span>
    <span v-else-if="help" :id="inputId ? `${inputId}-help` : undefined" class="hnb-form-field__help">{{ help }}</span>
  </label>
</template>

<style scoped>
.hnb-form-field { display: flex; flex-direction: column; gap: var(--hnb-space-xs); color: var(--hnb-color-text-primary); }
.hnb-form-field__label { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-secondary); font-weight: var(--hnb-font-weight-semibold); }
.hnb-form-field__help { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-tertiary); }
.hnb-form-field__error { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-status-danger); }
</style>
