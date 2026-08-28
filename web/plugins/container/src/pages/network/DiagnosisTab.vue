<script setup lang="ts">
/**
 * DiagnosisTab — 网络诊断。
 * Pod 间连通性测试 / NetworkPolicy 命中测试 / DNS 解析测试；将底层命令行工具能力页面化。
 */
import { computed, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBTable, StatusBadge } from '@hnb/ui-kit'
import type { HNBTableColumn } from '@hnb/ui-kit'
import {
  containerCniFeatureAvailable,
  runDiagnosis,
  type DiagnosisResult,
  type DiagnosisType,
} from '../../api/containerNetworkApi'

const { t } = useI18n()
const available = containerCniFeatureAvailable('diagnosis')

const running = ref(false)
const results = ref<DiagnosisResult[]>([])
const source = ref('')
const target = ref('')

const presetPairs = [
  { source: 'web-frontend', target: 'api-gateway' },
  { source: 'web-frontend', target: 'redis' },
]

const columns = computed<HNBTableColumn<DiagnosisResult>[]>(() => [
  { key: 'type', title: t('container.network.diagnosis.colType'), render: (r) => t(`container.network.diagnosis.type.${r.type}`) },
  { key: 'source', title: t('container.network.diagnosis.colSource'), render: (r) => r.source || '--' },
  { key: 'target', title: t('container.network.diagnosis.colTarget'), render: (r) => r.target || '--' },
  { key: 'result', title: t('container.network.diagnosis.colResult'), render: (r) => h(StatusBadge, { label: r.result === 'success' ? t('container.network.diagnosis.success') : t('container.network.diagnosis.fail'), semantic: r.result === 'success' ? ('success' as const) : ('error' as const) }) },
  { key: 'detail', title: t('container.network.diagnosis.colDetail'), render: (r) => r.detail || '--' },
])

async function run(type: DiagnosisType): Promise<void> {
  if (!available || running.value || !source.value.trim() || !target.value.trim()) return
  running.value = true
  try {
    const res = await runDiagnosis(type, source.value.trim(), target.value.trim())
    results.value.unshift(res)
  } finally {
    running.value = false
  }
}

function usePair(pair: { source: string; target: string }): void {
  source.value = pair.source
  target.value = pair.target
}
</script>

<template>
  <section class="network-tab-panel" :aria-label="t('container.network.tab.diagnosis')">
    <p v-if="!available" class="panel-status" role="status">{{ t('container.network.diagnosis.notSupported') }}</p>
    <template v-else>
      <div class="diag-toolbar">
        <div class="source-inputs">
          <label class="form-field">
            <span>{{ t('container.network.diagnosis.source') }}</span>
            <input v-model="source" type="text" placeholder="pod/web-frontend" />
          </label>
          <label class="form-field">
            <span>{{ t('container.network.diagnosis.target') }}</span>
            <input v-model="target" type="text" placeholder="pod/api-gateway" />
          </label>
        </div>
        <div class="preset-pairs">
          <button v-for="p in presetPairs" :key="p.source + p.target" class="preset-btn" type="button" @click="usePair(p)">
            {{ p.source }} → {{ p.target }}
          </button>
        </div>
        <div class="diag-actions">
          <HNBButton :loading="running" @click="run('connectivity')">
            {{ t('container.network.diagnosis.connectivity') }}
          </HNBButton>
          <HNBButton :loading="running" variant="secondary" @click="run('networkPolicy')">
            {{ t('container.network.diagnosis.networkPolicy') }}
          </HNBButton>
          <HNBButton :loading="running" variant="secondary" @click="run('dns')">
            {{ t('container.network.diagnosis.dns') }}
          </HNBButton>
        </div>
      </div>

      <HNBTable
        :columns="columns"
        :data="results"
        :empty-title="t('container.network.diagnosis.empty')"
        min-width="900px"
        :aria-label="t('container.network.tab.diagnosis')"
      />
    </template>
  </section>
</template>

<style scoped>
.network-tab-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-status { color: var(--hnb-color-text-secondary, #5b6675); font-size: 13px; }
.diag-toolbar { display: flex; flex-direction: column; gap: 10px; }
.source-inputs { display: flex; gap: 12px; }
.form-field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.form-field input {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: var(--hnb-color-bg-elevated, #f6f8fb);
  color: var(--hnb-color-text-primary, #12172a);
  min-width: 220px;
}
.preset-pairs { display: flex; gap: 8px; flex-wrap: wrap; }
.preset-btn {
  padding: 4px 10px;
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
  background: transparent;
  color: var(--hnb-color-primary, #2f6fed);
  font-size: 12px;
  cursor: pointer;
}
.diag-actions { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
