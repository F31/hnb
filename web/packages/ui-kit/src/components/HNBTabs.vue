<script setup lang="ts">
import { nextTick, useId } from 'vue'
import type { HNBTab } from '../types'

const props = defineProps<{
  modelValue: string
  tabs: HNBTab[]
  ariaLabel: string
}>()

const emit = defineEmits<{ 'update:modelValue': [id: string] }>()
const id = useId()

function enabledTabs() {
  return props.tabs.filter(tab => !tab.disabled)
}

async function select(tab: HNBTab) {
  if (tab.disabled) return
  emit('update:modelValue', tab.id)
  await nextTick()
  document.getElementById(`${id}-tab-${tab.id}`)?.focus()
}

function onKeydown(event: KeyboardEvent, current: HNBTab) {
  const tabs = enabledTabs()
  const currentIndex = tabs.findIndex(tab => tab.id === current.id)
  let next: HNBTab | undefined
  if (event.key === 'ArrowRight') next = tabs[(currentIndex + 1) % tabs.length]
  if (event.key === 'ArrowLeft') next = tabs[(currentIndex - 1 + tabs.length) % tabs.length]
  if (event.key === 'Home') next = tabs[0]
  if (event.key === 'End') next = tabs[tabs.length - 1]
  if (next) {
    event.preventDefault()
    void select(next)
  }
}
</script>

<template>
  <div class="hnb-tabs">
    <div class="hnb-tabs__list" role="tablist" :aria-label="ariaLabel">
      <template v-for="tab in tabs" :key="tab.id">
        <button
          :id="`${id}-tab-${tab.id}`"
          class="hnb-tabs__tab"
          type="button"
          role="tab"
          :aria-selected="modelValue === tab.id"
          :aria-controls="`${id}-panel-${tab.id}`"
          :aria-describedby="tab.disabledReason ? `${id}-reason-${tab.id}` : undefined"
          :tabindex="modelValue === tab.id ? 0 : -1"
          :disabled="tab.disabled"
          @click="select(tab)"
          @keydown="onKeydown($event, tab)"
        >{{ tab.label }}</button>
        <span v-if="tab.disabledReason" :id="`${id}-reason-${tab.id}`" class="hnb-tabs__sr-only">{{ tab.disabledReason }}</span>
      </template>
    </div>
    <div
      v-for="tab in tabs"
      v-show="modelValue === tab.id"
      :id="`${id}-panel-${tab.id}`"
      :key="`${tab.id}-panel`"
      class="hnb-tabs__panel"
      role="tabpanel"
      :aria-labelledby="`${id}-tab-${tab.id}`"
      tabindex="0"
    ><slot :name="`panel-${tab.id}`" /></div>
  </div>
</template>

<style scoped>
.hnb-tabs__list { display: flex; max-width: 100%; overflow-x: auto; border-bottom: 1px solid var(--hnb-color-divider); }
.hnb-tabs__tab { flex: 0 0 auto; padding: var(--hnb-space-sm) var(--hnb-space-md); border: 0; border-bottom: 2px solid transparent; background: transparent; color: var(--hnb-color-text-secondary); font: inherit; cursor: pointer; }
.hnb-tabs__tab[aria-selected='true'] { border-color: var(--hnb-color-primary); color: var(--hnb-color-primary); font-weight: var(--hnb-font-weight-semibold); }
.hnb-tabs__tab:focus-visible, .hnb-tabs__panel:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: -2px; }
.hnb-tabs__tab:disabled { cursor: not-allowed; opacity: 0.55; }
.hnb-tabs__panel { padding: var(--hnb-space-md) 0; }
.hnb-tabs__sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
</style>
