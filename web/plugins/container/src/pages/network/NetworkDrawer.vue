<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton } from '@hnb/ui-kit'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
  title: string
  busy?: boolean
  error?: string
  hideConfirm?: boolean
  confirmDisabled?: boolean
  closeOnBackdrop?: boolean
  closeLabel?: string
  cancelText?: string
  confirmText?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  cancel: []
  confirm: []
}>()

const bodyEl = ref<HTMLDivElement | null>(null)

function close(): void {
  if (props.busy) return
  emit('update:modelValue', false)
  emit('cancel')
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape' && props.modelValue) close()
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

watch(
  () => props.modelValue,
  (visible) => {
    if (visible) bodyEl.value?.focus()
  },
)
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="hnb-drawer-layer" @click.self="closeOnBackdrop !== false && close()">
      <div
        ref="bodyEl"
        class="hnb-drawer"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        tabindex="-1"
      >
        <header class="hnb-drawer__header">
          <h3 class="hnb-drawer__title">{{ title }}</h3>
          <button class="hnb-drawer__close" type="button" :disabled="busy" :aria-label="closeLabel || t('container.network.action.close')" @click="close">×</button>
        </header>
        <div class="hnb-drawer__body">
          <p v-if="error" class="hnb-drawer__error" role="alert">{{ error }}</p>
          <slot />
        </div>
        <footer class="hnb-drawer__footer">
          <HNBButton variant="secondary" :disabled="busy" @click="close">
            {{ cancelText || t('container.network.action.cancel') }}
          </HNBButton>
          <HNBButton v-if="!hideConfirm" :loading="busy" :disabled="busy || confirmDisabled" @click="emit('confirm')">
            {{ confirmText || t('container.network.action.confirm') }}
          </HNBButton>
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.hnb-drawer-layer {
  position: fixed;
  inset: 0;
  z-index: 1200;
  background: rgba(18, 23, 42, 0.4);
  display: flex;
  justify-content: flex-end;
}
.hnb-drawer {
  width: 560px;
  max-width: 92vw;
  height: 100%;
  background: var(--hnb-color-bg-surface, #fff);
  box-shadow: var(--hnb-shadow-4, 0 24px 64px rgba(0, 0, 0, 0.2));
  display: flex;
  flex-direction: column;
  animation: hnb-drawer-in 0.22s var(--hnb-ease, ease-out);
}
@keyframes hnb-drawer-in {
  from { transform: translateX(40px); opacity: 0.6; }
  to { transform: translateX(0); opacity: 1; }
}
@media (prefers-reduced-motion: reduce) {
  .hnb-drawer { animation: none; }
}
.hnb-drawer__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
}
.hnb-drawer__title { margin: 0; font-size: 16px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.hnb-drawer__close {
  border: 0;
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 20px;
  cursor: pointer;
  line-height: 1;
}
.hnb-drawer__body {
  flex: 1;
  overflow: auto;
  padding: 16px 20px;
  scrollbar-width: thin;
  scrollbar-color: var(--hnb-color-text-tertiary, #8a94a3) transparent;
  scrollbar-gutter: stable;
}
.hnb-drawer__body::-webkit-scrollbar { width: 6px; }
.hnb-drawer__body::-webkit-scrollbar-thumb { background: var(--hnb-color-text-tertiary, #8a94a3); border-radius: 3px; }
.hnb-drawer__body::-webkit-scrollbar-thumb:hover { background: var(--hnb-color-text-secondary, #5b6675); }
.hnb-drawer__body::-webkit-scrollbar-track { background: transparent; }
.hnb-drawer__error { color: var(--hnb-color-status-danger, #f04438); font-size: 13px; margin: 0 0 12px; }
.hnb-drawer__footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px;
  border-top: 1px solid var(--hnb-color-border, #e2e7ef);
}
</style>
