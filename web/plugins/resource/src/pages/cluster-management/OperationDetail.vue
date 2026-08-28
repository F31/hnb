<script setup lang="ts">
/**
 * OperationDetail — Operation Center 详情页（L3 组件）。
 *
 * 展示步骤、进度（复用 ui-kit HNBOperationProgress）、安全 failure、
 * allowed actions（approve/reject/cancel）与 Operation/Intent/Target 双向深链。
 * 使用共享 Operation 轮询客户端跟踪非终态，到达终态后停止轮询。
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { createOperationPoller, getOperation, operationAction } from './api/operationApi'
import { getClusterNavigate } from './api/clusterApi'
import { isTerminalStatus } from './types/operation'
import type { OperationAction, OperationDetail } from './types/operation'

const { t } = useI18n()
const route = useRoute()
const operationId = route.params.operationId as string

const detail = ref<OperationDetail | null>(null)
const loading = ref(true)
const error = ref('')
const acting = ref<OperationAction | ''>('')
const actionError = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const res = await getOperation(operationId)
    detail.value = res.data
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    detail.value = null
  } finally {
    loading.value = false
  }
}

const poller = createOperationPoller({
  onUpdate: (next) => {
    detail.value = next
  },
  onTerminal: (next) => {
    detail.value = next
  },
  onError: () => {
    /* transient poll errors are retried by the poller */
  },
})

async function performAction(action: OperationAction): Promise<void> {
  actionError.value = ''
  acting.value = action
  try {
    const res = await operationAction(operationId, action)
    detail.value = res.data
  } catch (err) {
    actionError.value = err instanceof Error ? err.message : String(err)
  } finally {
    acting.value = ''
  }
}

function openLink(href: string): void {
  getClusterNavigate()(href)
}

onMounted(() => {
  load()
  poller.start(operationId)
})

onBeforeUnmount(() => {
  poller.cancel()
})

watch(detail, (value) => {
  if (value && isTerminalStatus(value.status)) {
    poller.stop()
  }
})
</script>

<template>
  <section class="operation-detail">
    <header class="page-header">
      <div>
        <h1>{{ t('resource.operationCenter.detailTitle') }}</h1>
        <p v-if="detail" class="operation-id">{{ detail.operationId }}</p>
      </div>
    </header>

    <div v-if="loading" class="panel-status" role="status">{{ t('resource.operationCenter.common.loading') }}</div>
    <div v-else-if="error" class="panel-status error" role="alert">
      <span>{{ error }}</span>
      <button class="retry-button" type="button" @click="load">{{ t('resource.operationCenter.action.retry') }}</button>
    </div>

    <template v-else-if="detail">
      <div class="summary-card">
        <div class="summary-row">
          <span class="label">{{ t('resource.operationCenter.status') }}</span>
          <span class="status-badge" :data-status="detail.status">{{ t(`resource.operationCenter.status.${detail.status}`) }}</span>
        </div>
        <div class="summary-row">
          <span class="label">{{ t('resource.operationCenter.type') }}</span>
          <span>{{ t(`resource.operationCenter.type.${detail.type}`) }}</span>
        </div>
        <div class="summary-row">
          <span class="label">{{ t('resource.operationCenter.target') }}</span>
          <a
            v-if="detail.links.target"
            class="deep-link"
            href="#"
            @click.prevent="openLink(detail.links.target)"
          >{{ detail.targetId }}</a>
          <span v-else>{{ detail.targetId }}</span>
        </div>
        <div class="summary-row">
          <span class="label">{{ t('resource.operationCenter.intent') }}</span>
          <a
            v-if="detail.links.intent"
            class="deep-link"
            href="#"
            @click.prevent="openLink(detail.links.intent)"
          >{{ detail.intentId }}</a>
          <span v-else>{{ detail.intentId }}</span>
        </div>
        <div v-if="detail.failure" class="failure-banner" role="alert">
          <strong>{{ detail.failure.code }}</strong>
          <span>{{ detail.failure.message }}</span>
        </div>
      </div>

      <div class="progress-card">
        <h2>{{ t('resource.operationCenter.progressTitle') }}</h2>
        <p class="progress-caption">
          {{ detail.progress.completedSteps }} / {{ detail.progress.totalSteps }}
        </p>
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: detail.progress.percent + '%' }"></div>
        </div>
      </div>

      <div v-if="detail.allowedActions.length" class="action-card">
        <h2>{{ t('resource.operationCenter.actionsTitle') }}</h2>
        <p v-if="actionError" class="action-error" role="alert">{{ actionError }}</p>
        <div class="action-row">
          <button
            v-for="action in detail.allowedActions"
            :key="action"
            class="action-button"
            :class="{ danger: action === 'reject' || action === 'cancel' }"
            type="button"
            :disabled="acting !== ''"
            @click="performAction(action)"
          >
            {{ t(`resource.operationCenter.action.${action}`) }}
          </button>
        </div>
      </div>

      <div class="steps-card">
        <h2>{{ t('resource.operationCenter.stepsTitle') }}</h2>
        <table class="steps-table">
          <thead>
            <tr>
              <th>{{ t('resource.operationCenter.step.name') }}</th>
              <th>{{ t('resource.operationCenter.step.statusHeader') }}</th>
              <th>{{ t('resource.operationCenter.step.attempt') }}</th>
              <th>{{ t('resource.operationCenter.step.completedAt') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="step in detail.steps" :key="step.stepId">
              <td>{{ step.name }}</td>
              <td>
                <span class="status-badge" :data-status="step.status">{{ t(`resource.operationCenter.step.status.${step.status}`) }}</span>
              </td>
              <td>{{ step.attempt }}</td>
              <td>{{ step.completedAt || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.operation-detail { display: flex; flex-direction: column; gap: var(--hnb-space-md); color: var(--hnb-color-text-primary); }
.page-header h1 { margin: 0; font-size: var(--hnb-font-size-title); }
.operation-id { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); word-break: break-all; }
.panel-status { padding: var(--hnb-space-xl); text-align: center; color: var(--hnb-color-text-tertiary); }
.panel-status.error { color: var(--hnb-color-status-danger); display: flex; flex-direction: column; gap: var(--hnb-space-sm); align-items: center; }
.retry-button { border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); background: var(--hnb-color-bg-surface); color: var(--hnb-color-text-primary); padding: 4px 12px; cursor: pointer; }
.summary-card, .progress-card, .action-card, .steps-card {
  border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-lg); background: var(--hnb-color-bg-surface); padding: var(--hnb-space-md);
}
.summary-card { display: flex; flex-direction: column; gap: var(--hnb-space-sm); }
.summary-row { display: flex; gap: var(--hnb-space-md); align-items: center; }
.summary-row .label { width: 90px; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.deep-link { color: var(--hnb-color-primary); cursor: pointer; }
.status-badge {
  display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: var(--hnb-font-size-caption);
  background: color-mix(in srgb, var(--hnb-color-text-tertiary) 16%, transparent); color: var(--hnb-color-text-secondary);
}
.status-badge[data-status='succeeded'] { background: color-mix(in srgb, var(--hnb-color-status-success) 14%, transparent); color: var(--hnb-color-status-success); }
.status-badge[data-status='failed'] { background: color-mix(in srgb, var(--hnb-color-status-danger) 14%, transparent); color: var(--hnb-color-status-danger); }
.status-badge[data-status='in_progress'] { background: color-mix(in srgb, var(--hnb-color-status-info) 14%, transparent); color: var(--hnb-color-status-info); }
.failure-banner { display: flex; flex-direction: column; gap: var(--hnb-space-xs); padding: var(--hnb-space-sm) var(--hnb-space-md); border-radius: var(--hnb-radius-md); background: color-mix(in srgb, var(--hnb-color-status-danger) 10%, transparent); color: var(--hnb-color-status-danger); font-size: var(--hnb-font-size-body); }
.progress-caption { margin: 0; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.progress-track { height: 10px; border-radius: 999px; background: var(--hnb-color-divider); overflow: hidden; }
.progress-fill { height: 100%; background: var(--hnb-color-primary); border-radius: 999px; transition: width 300ms ease; }
.action-error { color: var(--hnb-color-status-danger); font-size: var(--hnb-font-size-body); }
.action-row { display: flex; gap: var(--hnb-space-sm); }
.action-button {
  padding: 8px 18px; border: 0; border-radius: var(--hnb-radius-md); background: var(--hnb-color-primary); color: #fff; cursor: pointer; font-size: var(--hnb-font-size-body);
}
.action-button.danger { background: var(--hnb-color-status-danger); }
.action-button:disabled { opacity: 0.55; cursor: not-allowed; }
.steps-table { width: 100%; border-collapse: collapse; font-size: var(--hnb-font-size-body); }
.steps-table th { text-align: left; font-weight: var(--hnb-font-weight-semibold); color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); padding: var(--hnb-space-sm); border-bottom: 1px solid var(--hnb-color-divider); }
.steps-table td { padding: var(--hnb-space-sm); border-bottom: 1px solid var(--hnb-color-divider); white-space: nowrap; }
</style>
