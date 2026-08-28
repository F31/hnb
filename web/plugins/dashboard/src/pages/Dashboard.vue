<script setup lang="ts">
/**
 * 平台运行总览 — Schema 驱动试点页面（V2.5）。
 * 页面结构由 dashboard.schema.ts 描述，经 SchemaEngine 校验、
 * ComponentRegistry 受信组件解析后渲染，区块级错误隔离。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  PageRenderer,
  createComponentRegistry,
  registerBuiltinComponents,
} from '@hnb/schema-engine'
import { dashboardSchema } from './dashboard.schema'

const registry = createComponentRegistry()
registerBuiltinComponents(registry)

const { t } = useI18n({ useScope: 'global' })
// 页面文案随 locale 响应式更新；Schema 内 region props 视为服务端下发内容，
// 由服务端本地化，前端不做二次翻译。
const texts = computed(() => ({
  'dashboard.title': t('dashboard.page.title'),
}))
</script>

<template>
  <PageRenderer :schema="dashboardSchema" :registry="registry" :texts="texts" />
</template>
