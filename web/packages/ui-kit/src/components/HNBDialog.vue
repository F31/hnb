<script setup lang="ts">
import { nextTick, onBeforeUnmount, useId, watch } from 'vue'
import HNBButton from './HNBButton.vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title: string
  description?: string
  closeLabel?: string
  closeOnBackdrop?: boolean
  busy?: boolean
  error?: string
  initialFocus?: string
}>(), {
  closeLabel: 'Close dialog',
  closeOnBackdrop: true,
  busy: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  close: []
}>()

const id = useId()
const titleId = `${id}-title`
const descriptionId = `${id}-description`
const errorId = `${id}-error`
let dialog: HTMLElement | null = null
let restoreTarget: HTMLElement | null = null
let previousOverflow = ''

const focusableSelector = [
  'button:not([disabled])',
  '[href]',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function setDialogRef(element: unknown) {
  dialog = element as HTMLElement | null
}

function requestClose() {
  if (props.busy) return
  emit('update:modelValue', false)
  emit('close')
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    requestClose()
    return
  }
  if (event.key !== 'Tab' || !dialog) return
  const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
  if (focusable.length === 0) {
    event.preventDefault()
    dialog.focus()
    return
  }
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(() => props.modelValue, async (open) => {
  if (open) {
    restoreTarget = document.activeElement instanceof HTMLElement ? document.activeElement : null
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    const initial = props.initialFocus ? dialog?.querySelector<HTMLElement>(props.initialFocus) : null
    const first = dialog?.querySelector<HTMLElement>(focusableSelector)
    ;(initial ?? first ?? dialog)?.focus()
  } else {
    document.body.style.overflow = previousOverflow
    restoreTarget?.focus()
    restoreTarget = null
  }
}, { immediate: true })

onBeforeUnmount(() => {
  document.body.style.overflow = previousOverflow
  restoreTarget?.focus()
})
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="hnb-dialog-layer" @mousedown.self="closeOnBackdrop && requestClose()">
      <section
        :ref="setDialogRef"
        class="hnb-dialog"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        :aria-describedby="[description ? descriptionId : '', error ? errorId : ''].filter(Boolean).join(' ') || undefined"
        :aria-errormessage="error ? errorId : undefined"
        :aria-invalid="error ? 'true' : undefined"
        :aria-busy="busy"
        tabindex="-1"
        @keydown="onKeydown"
      >
        <header class="hnb-dialog__header">
          <div>
            <h2 :id="titleId" class="hnb-dialog__title">{{ title }}</h2>
            <p v-if="description" :id="descriptionId" class="hnb-dialog__description">{{ description }}</p>
          </div>
          <HNBButton variant="ghost" size="small" :disabled="busy" :aria-label="closeLabel" @click="requestClose">×</HNBButton>
        </header>
        <div class="hnb-dialog__body"><slot /></div>
        <div v-if="error" :id="errorId" class="hnb-dialog__error" role="alert">{{ error }}</div>
        <footer v-if="$slots.footer" class="hnb-dialog__footer"><slot name="footer" /></footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.hnb-dialog-layer { position: fixed; inset: 0; z-index: 1000; display: grid; place-items: center; padding: var(--hnb-space-md); background: var(--hnb-color-overlay); }
.hnb-dialog { width: min(560px, 100%); max-height: calc(100dvh - 2 * var(--hnb-space-md)); overflow-y: auto; border-radius: var(--hnb-radius-lg); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); box-shadow: var(--hnb-shadow-4); scrollbar-width: thin; scrollbar-color: var(--hnb-color-border) transparent; }
.hnb-dialog:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }
.hnb-dialog::-webkit-scrollbar { width: 5px; }
.hnb-dialog::-webkit-scrollbar-track { background: transparent; }
.hnb-dialog::-webkit-scrollbar-thumb { background: var(--hnb-color-border); border-radius: 3px; }
.hnb-dialog__header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--hnb-space-md); padding: var(--hnb-space-lg); border-bottom: 1px solid var(--hnb-color-divider); }
.hnb-dialog__title { margin: 0; font-size: var(--hnb-font-size-title); }
.hnb-dialog__description { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-secondary); }
.hnb-dialog__body { padding: var(--hnb-space-lg); }
.hnb-dialog__error { margin: 0 var(--hnb-space-lg); color: var(--hnb-color-status-danger); }
.hnb-dialog__footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: var(--hnb-space-sm); padding: var(--hnb-space-md) var(--hnb-space-lg) var(--hnb-space-lg); }
@media (max-width: 480px) {
  .hnb-dialog-layer { align-items: end; padding: 0; }
  .hnb-dialog { width: 100%; max-height: 92dvh; border-radius: var(--hnb-radius-lg) var(--hnb-radius-lg) 0 0; }
  .hnb-dialog__footer > :deep(*) { flex: 1; }
}
</style>
