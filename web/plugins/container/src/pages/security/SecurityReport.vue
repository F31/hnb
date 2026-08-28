<script setup lang="ts">
/**
 * SecurityReport — 安全治理 > 安全报告。
 * 漏洞/运行时/网络隔离/网络异常报告；网络隔离审计报告提供一键跳转处理入口。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { HNBTable } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import { getSecurityReports } from '../../api/securityApi'
import type { SecurityReportItem } from '../../api/securityTypes'

const { t } = useI18n()
const router = useRouter()

const items = ref<SecurityReportItem[]>([])
const loading = ref(true)

function reportTypeLabel(tp: SecurityReportItem['type']): string {
  return t(`container.security.report.type.${tp}`)
}

function isIsolationReport(r: SecurityReportItem): boolean {
  return r.type === 'isolation'
}

function goToNetworkPolicy(): void {
  router.push('/container/network')
}

const columns = computed<HNBTableColumn<SecurityReportItem>[]>(() => [
  { key: 'type', title: t('container.security.report.colType'), render: (r) => reportTypeLabel(r.type) },
  { key: 'title', title: t('container.security.report.colTitle'), render: (r) => r.title || '--' },
  { key: 'generatedAt', title: t('container.security.report.colTime'), render: (r) => r.generatedAt || '--' },
  { key: 'summary', title: t('container.security.report.colSummary'), render: (r) => r.summary || '--' },
  {
    key: 'actions',
    title: t('container.security.colActions'),
    render: (r) => {
      if (!isIsolationReport(r)) return h('span', {}, '--')
      return h('button', { type: 'button', class: 'text-action', onClick: goToNetworkPolicy }, t('container.security.report.handle'))
    },
  },
])

async function load(): Promise<void> {
  loading.value = true
  try {
    items.value = await getSecurityReports()
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="security-page">
    <p v-if="loading" class="panel-status" role="status">{{ t('container.security.loading') }}</p>
    <HNBTable v-else :columns="columns" :data="items" :empty-title="t('container.security.empty')" min-width="1000px" :aria-label="t('container.security.report.title')" />
    <p class="report-hint">{{ t('container.security.report.hint') }}</p>
  </div>
</template>

<style scoped>
.security-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 18px 20px;
}
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.report-hint { margin: 0; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
:deep(.text-action) {
  border: 0;
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  cursor: pointer;
  font-size: 13px;
  padding: 2px 6px;
}
</style>
