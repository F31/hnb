<script setup lang="ts">
import { ref, watch } from 'vue'
import HNBButton from './HNBButton.vue'
import HNBDialog from './HNBDialog.vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  loading?: boolean
  error?: string
  requireAcknowledgement?: boolean
  acknowledgementLabel?: string
}>(), {
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  danger: false,
  loading: false,
  requireAcknowledgement: false,
  acknowledgementLabel: 'I understand the impact',
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
  cancel: []
}>()

const acknowledged = ref(false)
watch(() => props.modelValue, (open) => {
  if (open) acknowledged.value = false
})

function cancel() {
  if (props.loading) return
  emit('update:modelValue', false)
  emit('cancel')
}
</script>

<template>
  <HNBDialog
    :model-value="modelValue"
    :title="title"
    :description="description"
    :busy="loading"
    :error="error"
    initial-focus="[data-confirm-cancel]"
    @update:model-value="(value) => emit('update:modelValue', value)"
    @close="emit('cancel')"
  >
    <slot />
    <label v-if="requireAcknowledgement" class="hnb-confirmation__acknowledgement">
      <input v-model="acknowledged" type="checkbox">
      <span>{{ acknowledgementLabel }}</span>
    </label>
    <template #footer>
      <HNBButton data-confirm-cancel :disabled="loading" @click="cancel">{{ cancelText }}</HNBButton>
      <HNBButton
        :variant="danger ? 'danger' : 'primary'"
        :loading="loading"
        :disabled="requireAcknowledgement && !acknowledged"
        :disabled-reason="requireAcknowledgement && !acknowledged ? acknowledgementLabel : undefined"
        @click="emit('confirm')"
      >{{ confirmText }}</HNBButton>
    </template>
  </HNBDialog>
</template>

<style scoped>
.hnb-confirmation__acknowledgement { display: flex; align-items: flex-start; gap: var(--hnb-space-sm); margin-top: var(--hnb-space-md); color: var(--hnb-color-text-primary); }
.hnb-confirmation__acknowledgement input { margin-top: var(--hnb-space-xs); accent-color: var(--hnb-color-primary); }
.hnb-confirmation__acknowledgement input:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }
</style>
