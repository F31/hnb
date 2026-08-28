<script setup lang="ts">
/**
 * YamlEditor — 只读/可编辑 YAML 编辑区（行号 + 错误摘要）。
 * 校验由调用方注入 validate 函数（可选）；错误摘要显示在编辑器下方。
 */
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    readonly?: boolean
    label?: string
    rows?: number
    placeholder?: string
    error?: string
  }>(),
  { readonly: false, rows: 12, placeholder: '', error: '' },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const lines = computed(() => props.modelValue.split('\n').length)

function onInput(e: Event): void {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}
</script>

<template>
  <div class="yaml-editor">
    <span v-if="label" class="yaml-editor__label">{{ label }}</span>
    <div class="yaml-editor__wrap" :class="{ readonly }">
      <div class="yaml-editor__gutter" aria-hidden="true">
        <span v-for="n in Math.max(lines, rows)" :key="n">{{ n }}</span>
      </div>
      <textarea
        :value="modelValue"
        :readonly="readonly"
        :rows="rows"
        class="yaml-editor__textarea"
        :placeholder="placeholder"
        spellcheck="false"
        @input="onInput"
      ></textarea>
    </div>
    <p v-if="error" class="yaml-editor__error" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.yaml-editor { display: flex; flex-direction: column; gap: 4px; }
.yaml-editor__label { font-size: 12px; color: var(--hnb-color-text-secondary, #5b6675); }
.yaml-editor__wrap {
  display: flex;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  overflow: hidden;
  background: var(--hnb-color-bg-elevated, #f6f8fb);
}
.yaml-editor__gutter {
  display: flex;
  flex-direction: column;
  padding: 8px 6px;
  text-align: right;
  color: var(--hnb-color-text-tertiary, #8a94a3);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  user-select: none;
  background: color-mix(in srgb, var(--hnb-color-border, #e2e7ef) 40%, transparent);
}
.yaml-editor__textarea {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--hnb-color-text-primary, #12172a);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 8px 10px;
  resize: vertical;
  outline: none;
}
.yaml-editor__textarea:focus { box-shadow: inset 0 0 0 2px var(--hnb-color-focus, #2f6fed); }
.yaml-editor__error { color: var(--hnb-color-status-danger, #f04438); font-size: 12px; }
</style>
