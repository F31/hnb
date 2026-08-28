<script setup lang="ts">
import { computed, inject } from 'vue'
import type { HNBSelectOption } from '../types'
import { HNB_FORM_FIELD_INJECTION_KEY } from '../types'

withDefaults(defineProps<{
  modelValue?: string
  options: HNBSelectOption[]
  placeholder?: string
  disabled?: boolean
}>(), {
  modelValue: '',
  placeholder: '请选择',
  disabled: false,
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const formField = inject(HNB_FORM_FIELD_INJECTION_KEY, undefined)
const ariaDescribedBy = computed(() => formField?.ariaDescribedBy.value)
</script>

<template>
  <select class="hnb-select-input" :value="modelValue" :disabled="disabled" :aria-describedby="ariaDescribedBy" @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)">
    <option value="" disabled>{{ placeholder }}</option>
    <option v-for="option in options" :key="option.value" :value="option.value" :disabled="option.disabled">{{ option.label }}</option>
  </select>
</template>

<style scoped>
.hnb-select-input { min-width: 160px; height: 34px; padding: 0 var(--hnb-space-sm); color: var(--hnb-color-text-primary); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.hnb-select-input:focus { outline: 2px solid color-mix(in srgb, var(--hnb-color-primary) 30%, transparent); border-color: var(--hnb-color-primary); }
</style>
