<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton } from '@hnb/ui-kit'

const props = withDefaults(defineProps<{
  modelValue: boolean
  title: string
  busy?: boolean
  error?: string
  hideConfirm?: boolean
  confirmDisabled?: boolean
  width?: string | number
  closeOnBackdrop?: boolean
  closeLabel?: string
  cancelText?: string
  confirmText?: string
}>(), {
  busy: false,
  error: '',
  hideConfirm: false,
  confirmDisabled: false,
  width: '560px',
  closeOnBackdrop: false,
  closeLabel: '',
  cancelText: '',
  confirmText: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  cancel: []
  confirm: []
}>()

const { t } = useI18n()
const dialogEl = ref<HTMLElement | null>(null)
const titleId = `application-drawer-title-${useId()}`
const drawerWidth = computed(() => typeof props.width === 'number' ? `${props.width}px` : props.width)
let previousFocus: HTMLElement | null = null
let previousBodyOverflow = ''
let scrollLocked = false

const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function close(): void {
  if (props.busy) return
  emit('update:modelValue', false)
  emit('cancel')
}

function focusDrawer(): void {
  const firstFocusable = dialogEl.value?.querySelector<HTMLElement>(focusableSelector)
  ;(firstFocusable || dialogEl.value)?.focus()
}

function restorePage(): void {
  if (scrollLocked) {
    document.body.style.overflow = previousBodyOverflow
    scrollLocked = false
  }
  if (previousFocus?.isConnected) previousFocus.focus()
  previousFocus = null
}

function onKeydown(event: KeyboardEvent): void {
  if (!props.modelValue) return
  const drawers = document.querySelectorAll<HTMLElement>('.application-drawer')
  if (drawers[drawers.length - 1] !== dialogEl.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
    return
  }
  if (event.key !== 'Tab' || !dialogEl.value) return

  const focusable = Array.from(dialogEl.value.querySelectorAll<HTMLElement>(focusableSelector))
  if (focusable.length === 0) {
    event.preventDefault()
    dialogEl.value.focus()
    return
  }

  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && (document.activeElement === first || !dialogEl.value.contains(document.activeElement))) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.modelValue,
  async (visible) => {
    if (!visible) {
      window.removeEventListener('keydown', onKeydown)
      restorePage()
      return
    }

    previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    previousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    scrollLocked = true
    window.addEventListener('keydown', onKeydown)
    await nextTick()
    focusDrawer()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  restorePage()
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="modelValue"
      class="application-drawer-layer"
      @click.self="closeOnBackdrop && close()"
    >
      <section
        ref="dialogEl"
        class="application-drawer"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        :aria-busy="busy"
        :style="{ width: drawerWidth }"
        tabindex="-1"
      >
        <header class="application-drawer__header">
          <h2 :id="titleId" class="application-drawer__title">{{ title }}</h2>
          <button
            class="application-drawer__close"
            type="button"
            :disabled="busy"
            :aria-label="closeLabel || t('application.common.close')"
            @click="close"
          >
            &times;
          </button>
        </header>

        <div class="application-drawer__body">
          <p v-if="error" class="application-drawer__error" role="alert">{{ error }}</p>
          <slot />
        </div>

        <footer class="application-drawer__footer">
          <slot name="footer" :close="close" :busy="busy">
            <HNBButton variant="secondary" :disabled="busy" @click="close">
              {{ cancelText || t('application.common.cancel') }}
            </HNBButton>
            <HNBButton
              v-if="!hideConfirm"
              variant="primary"
              :loading="busy"
              :disabled="confirmDisabled"
              @click="emit('confirm')"
            >
              {{ confirmText || t('application.common.confirm') }}
            </HNBButton>
          </slot>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.application-drawer-layer {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  justify-content: flex-end;
  background: rgba(18, 23, 42, 0.4);
}

.application-drawer {
  display: flex;
  flex-direction: column;
  max-width: 92vw;
  height: 100%;
  box-sizing: border-box;
  color: var(--hnb-color-text-primary, #12172a);
  background: var(--hnb-color-bg-surface, #fff);
  border-left: 1px solid var(--hnb-color-border, #e2e7ef);
  box-shadow: var(--hnb-shadow-4, -24px 0 64px rgba(0, 0, 0, 0.2));
  animation: application-drawer-in 0.22s var(--hnb-ease, ease-out);
}

.application-drawer__header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: var(--hnb-space-md, 16px);
  padding: var(--hnb-space-md, 16px) var(--hnb-space-lg, 20px);
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
}

.application-drawer__title {
  margin: 0;
  font-size: var(--hnb-font-size-lg, 16px);
  font-weight: var(--hnb-font-weight-semibold, 600);
}

.application-drawer__close {
  width: 32px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: var(--hnb-radius-md, 8px);
  color: var(--hnb-color-text-secondary, #5b6675);
  background: transparent;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
}

.application-drawer__close:hover:not(:disabled) {
  color: var(--hnb-color-text-primary, #12172a);
  background: var(--hnb-color-bg-elevated, #f4f6f9);
}

.application-drawer__close:disabled { cursor: not-allowed; opacity: 0.55; }
.application-drawer__close:focus-visible { outline: 2px solid var(--hnb-color-focus, #5b8dff); outline-offset: 2px; }

.application-drawer__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: var(--hnb-space-md, 16px) var(--hnb-space-lg, 20px);
  scrollbar-width: thin;
  scrollbar-color: var(--hnb-color-text-tertiary, #8a94a3) transparent;
  scrollbar-gutter: stable;
}

.application-drawer__body::-webkit-scrollbar { width: 6px; }
.application-drawer__body::-webkit-scrollbar-thumb { background: var(--hnb-color-text-tertiary, #8a94a3); border-radius: 3px; }
.application-drawer__body::-webkit-scrollbar-track { background: transparent; }

.application-drawer__error {
  margin: 0 0 var(--hnb-space-md, 12px);
  color: var(--hnb-color-status-danger, #f04438);
  font-size: var(--hnb-font-size-caption, 13px);
}

.application-drawer__footer {
  display: flex;
  flex: 0 0 auto;
  justify-content: flex-end;
  gap: var(--hnb-space-sm, 10px);
  padding: var(--hnb-space-md, 14px) var(--hnb-space-lg, 20px);
  border-top: 1px solid var(--hnb-color-border, #e2e7ef);
  background: var(--hnb-color-bg-surface, #fff);
}

@keyframes application-drawer-in {
  from { transform: translateX(40px); opacity: 0.6; }
  to { transform: translateX(0); opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .application-drawer { animation: none; }
}
</style>
