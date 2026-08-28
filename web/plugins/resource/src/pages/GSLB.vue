<template>
  <section class="gslb-page">
    <div class="gslb-header">
      <h2>{{ t('resource.gslb.title') }}</h2>
      <p class="gslb-desc">{{ t('resource.gslb.desc') }}</p>
    </div>

    <div v-if="loading" class="gslb-loading">加载 GSLB 服务...</div>

    <div v-else-if="error" class="gslb-error" role="alert">{{ error }}</div>

    <div v-else-if="services.length === 0" class="gslb-empty">
      暂无 GSLB 服务（流量层容灾服务需在平台创建）。
    </div>

    <template v-else>
      <div class="gslb-table">
        <table>
          <thead>
            <tr>
              <th>域名</th>
              <th>状态</th>
              <th>健康池</th>
              <th>当前 DNS 目标</th>
              <th>最近切换</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="svc in services" :key="svc.serviceId">
              <td>{{ svc.domain }}</td>
              <td>
                <span class="state-pill" :class="stateClass(svc.lifecycleState)">
                  {{ gslbStateLabel(svc.lifecycleState) }}
                </span>
              </td>
              <td>{{ svc.healthyPools.length }}</td>
              <td class="mono">{{ svc.currentDnsTargets.join(', ') || '—' }}</td>
              <td>{{ svc.lastSwitchAt ? new Date(svc.lastSwitchAt).toLocaleString() : '—' }}</td>
              <td>
                <button
                  type="button"
                  class="btn-small"
                  :class="{ active: selected?.serviceId === svc.serviceId }"
                  @click="select(svc.serviceId)"
                >详情 / 操作</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="selected" class="gslb-detail">
        <h3>{{ selected.domain }} — 操作</h3>
        <p class="gslb-meta">
          状态：{{ gslbStateLabel(selected.lifecycleState) }} ·
          健康池 {{ selected.healthyPools.length }} 个 ·
          当前目标 {{ selected.currentDnsTargets.join(', ') || '无' }}
        </p>

        <div class="gslb-actions">
          <label v-if="failoverCandidates.length > 0" class="gslb-target">
            目标池
            <select v-model="targetPoolId" class="gslb-select">
              <option v-for="p in failoverCandidates" :key="p" :value="p">{{ poolLabel(p) }}</option>
            </select>
          </label>
          <button
            type="button"
            class="btn-small danger"
            :disabled="acting || selected.lifecycleState === 'FailingOver' || failoverCandidates.length === 0"
            @click="run('gslb.failover', '手动故障转移到备用池')"
          >故障转移</button>
          <button
            type="button"
            class="btn-small"
            :disabled="acting || selected.lifecycleState === 'FailingOver'"
            @click="run('gslb.switchback', '回切主池')"
          >回切</button>
          <button
            type="button"
            class="btn-small"
            :disabled="acting"
            @click="run('gslb.drill', '只读演练：模拟切换，不产生真实 DNS 变更')"
          >故障演练</button>
        </div>

        <div v-if="acting" class="gslb-loading">正在提交（failover/switchback 需审批）...</div>

        <div v-if="lastRequest" class="gslb-request" :class="requestClass(lastRequest.status)">
          <strong>{{ gslbKindLabel(lastRequest.intentKind) }}</strong>
          状态：{{ lastRequest.status }}
          <span v-if="lastRequest.requireApproval">（需审批）</span>
          <span v-if="lastRequest.error"> · {{ lastRequest.error }}</span>
        </div>

        <div class="gslb-drills">
          <h4>{{ t('resource.gslb.drills.title') }}</h4>
          <div v-if="drills.length === 0" class="gslb-drill-empty">{{ t('resource.gslb.drills.empty') }}</div>
          <div v-for="drill in drills" :key="drill.id" class="gslb-drill" :class="verdictClass(drill.verdict)">
            <div class="gslb-drill-head">
              <span class="state-pill" :class="verdictClass(drill.verdict)">{{ verdictLabel(drill.verdict) }}</span>
              <span class="gslb-drill-time">{{ new Date(drill.createdAt).toLocaleString() }}</span>
            </div>
            <div class="gslb-drill-body">
              <div v-if="drill.report.targetPoolId">
                {{ t('resource.gslb.drills.projectedTargets') }}:
                <span class="mono">{{ drill.report.projectedTargets.join(', ') || '—' }}</span>
              </div>
              <div>
                {{ t('resource.gslb.drills.currentTargets') }}:
                <span class="mono">{{ drill.report.currentTargets.join(', ') || '—' }}</span>
              </div>
              <ul class="gslb-drill-checks">
                <li v-for="check in drill.report.checks" :key="check.name" :class="{ failed: !check.passed }">
                  {{ check.passed ? '✓' : '✗' }} {{ check.name }}<span v-if="check.detail"> — {{ check.detail }}</span>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getGslbService,
  gslbKindLabel,
  gslbStateLabel,
  listGslbDrillReports,
  listGslbServices,
  submitGslbIntent,
  type GslbDrillReport,
  type GslbDrillVerdict,
  type GslbIntentKind,
  type GslbReadModel,
  type GslbSwitchRequest,
} from '../gslbApi'

const { t } = useI18n()

const loading = ref(true)
const error = ref<string | null>(null)
const services = ref<GslbReadModel[]>([])
const selected = ref<GslbReadModel | null>(null)
const acting = ref(false)
const lastRequest = ref<GslbSwitchRequest | null>(null)
const drills = ref<GslbDrillReport[]>([])
const targetPoolId = ref('')

// 故障转移只允许切到健康且非当前主池的池（后端强制 targetPoolId）。
const failoverCandidates = computed(() => {
  const svc = selected.value
  if (!svc) return [] as string[]
  return svc.healthyPools.filter((p) => p && p !== svc.activePoolId)
})

function poolLabel(poolId: string): string {
  return poolId === selected.value?.activePoolId ? `${poolId.slice(0, 8)}…（主池）` : `${poolId.slice(0, 8)}…`
}

function verdictClass(verdict: GslbDrillVerdict): string {
  switch (verdict) {
    case 'Ready':
      return 'ok'
    case 'Degraded':
      return 'warn'
    default:
      return 'err'
  }
}

function verdictLabel(verdict: GslbDrillVerdict): string {
  switch (verdict) {
    case 'Ready':
      return t('resource.gslb.drills.verdictReady')
    case 'Degraded':
      return t('resource.gslb.drills.verdictDegraded')
    default:
      return t('resource.gslb.drills.verdictBlocked')
  }
}

function stateClass(state: string): string {
  switch (state) {
    case 'Active':
      return 'ok'
    case 'Degraded':
      return 'warn'
    case 'FailingOver':
      return 'warn'
    default:
      return 'muted'
  }
}

function requestClass(status: string): string {
  if (status === 'Succeeded' || status === 'DrillCompleted') return 'ok'
  if (status === 'Failed' || status === 'Rejected') return 'err'
  return 'pending'
}

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const res = await listGslbServices()
    services.value = res.items ?? []
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function select(serviceId: string): Promise<void> {
  try {
    selected.value = await getGslbService(serviceId)
    targetPoolId.value = failoverCandidates.value[0] ?? ''
    const drillRes = await listGslbDrillReports(serviceId)
    drills.value = drillRes.items ?? []
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function run(kind: GslbIntentKind, reason: string): Promise<void> {
  if (!selected.value || acting.value) return
  acting.value = true
  lastRequest.value = null
  error.value = null
  let target: string | undefined
  if (kind === 'gslb.failover') {
    target = targetPoolId.value
    if (!target) {
      error.value = '没有可用的健康备用池（故障转移需要目标池）'
      acting.value = false
      return
    }
  } else if (kind === 'gslb.switchback') {
    target = selected.value.activePoolId
    if (!target) {
      error.value = '主池未知，无法回切'
      acting.value = false
      return
    }
  } else if (kind === 'gslb.drill') {
    // 演练携带目标池可生成投影目标，便于只读预览切换效果。
    target = targetPoolId.value || undefined
  }
  try {
    lastRequest.value = await submitGslbIntent(
      selected.value.serviceId,
      kind,
      target,
      reason,
    )
    // 刷新投影（演练/免审批请求立即完成；审批型进入 PendingApproval）
    await select(selected.value.serviceId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    acting.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.gslb-page { padding: 16px; color: var(--hnb-color-text-primary, #edeff5); }
.gslb-header h2 { margin: 0 0 4px; font-size: 18px; }
.gslb-desc { margin: 0 0 16px; font-size: 13px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.gslb-loading, .gslb-empty, .gslb-error { padding: 32px; text-align: center; font-size: 14px; }
.gslb-error { color: var(--hnb-color-status-danger, #f04438); }
.gslb-table { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; overflow: hidden; }
.gslb-table table { width: 100%; border-collapse: collapse; font-size: 13px; }
.gslb-table th, .gslb-table td { text-align: left; padding: 10px 12px; border-bottom: 1px solid var(--hnb-color-divider, #222b3d); }
.gslb-table th { background: var(--hnb-color-bg-elevated, #171d31); font-weight: 600; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.state-pill { display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: 12px; }
.state-pill.ok { background: rgba(18, 183, 106, 0.15); color: #12b76a; }
.state-pill.warn { background: rgba(247, 144, 9, 0.15); color: #f79009; }
.state-pill.muted { background: rgba(138, 148, 163, 0.15); color: #8a94a3; }
.gslb-detail { margin-top: 16px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; padding: 16px; }
.gslb-detail h3 { margin: 0 0 8px; font-size: 15px; }
.gslb-meta { margin: 0 0 12px; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); }
.gslb-actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.gslb-target { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; }
.gslb-select {
  padding: 5px 8px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5);
  font-size: 13px;
}
.btn-small {
  padding: 6px 12px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5);
  font-size: 13px; cursor: pointer;
}
.btn-small:hover:not(:disabled) { border-color: var(--hnb-color-primary, #5b8dff); }
.btn-small.danger { color: var(--hnb-color-status-danger, #f04438); }
.btn-small.active { border-color: var(--hnb-color-primary, #5b8dff); color: var(--hnb-color-primary, #5b8dff); }
.btn-small:disabled { opacity: 0.5; cursor: not-allowed; }
.gslb-request { margin-top: 12px; padding: 10px 12px; border-radius: 6px; font-size: 13px; }
.gslb-request.ok { background: rgba(18, 183, 106, 0.1); color: #12b76a; }
.gslb-request.err { background: rgba(240, 68, 56, 0.1); color: #f04438; }
.gslb-request.pending { background: rgba(91, 141, 255, 0.1); color: #5b8dff; }
.gslb-drills { margin-top: 16px; }
.gslb-drills h4 { margin: 0 0 8px; font-size: 14px; }
.gslb-drill-empty { font-size: 13px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.gslb-drill { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; padding: 10px 12px; margin-bottom: 8px; font-size: 13px; }
.gslb-drill-head { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.gslb-drill-time { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.gslb-drill-body { color: var(--hnb-color-text-secondary, #a9b2c2); }
.gslb-drill-checks { margin: 6px 0 0; padding-left: 4px; list-style: none; }
.gslb-drill-checks li { padding: 2px 0; color: #12b76a; }
.gslb-drill-checks li.failed { color: var(--hnb-color-status-danger, #f04438); }
.state-pill.err { background: rgba(240, 68, 56, 0.15); color: #f04438; }
</style>
