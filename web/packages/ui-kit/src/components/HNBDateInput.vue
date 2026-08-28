<script setup lang="ts">
import { computed, inject } from 'vue'
import { HNB_FORM_FIELD_INJECTION_KEY } from '../types'

withDefaults(defineProps<{
  modelValue?: string
  type?: 'date' | 'datetime-local'
  disabled?: boolean
}>(), {
  modelValue: '',
  type: 'date',
  disabled: false,
})

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const formField = inject(HNB_FORM_FIELD_INJECTION_KEY, undefined)
const ariaDescribedBy = computed(() => formField?.ariaDescribedBy.value)
</script>

<template>
  <input class="hnb-date-input" :type="type" :value="modelValue" :disabled="disabled" :aria-describedby="ariaDescribedBy" @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)" />
</template>

<style scoped>
.hnb-date-input { height: 34px; padding: 0 var(--hnb-space-sm); color: var(--hnb-color-text-primary); background: var(--hnb-color-bg-surface); border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.hnb-date-input:focus { outline: 2px solid color-mix(in srgb, var(--hnb-color-primary) 30%, transparent); border-color: var(--hnb-color-primary); }
</style>
