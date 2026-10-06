<template>
  <section class="gslb-page">
    <div class="gslb-header">
      <div class="gslb-header-text">
        <h2>{{ t('resource.gslb.title') }}</h2>
        <p class="gslb-desc">{{ t('resource.gslb.desc') }}</p>
      </div>
      <button type="button" class="btn-small gslb-create-btn" @click="wizardOpen = true">
        {{ t('resource.gslb.create') }}
      </button>
    </div>

    <div class="gslb-summary">
      <article>
        <span>{{ t('resource.gslb.summary.total') }}</span>
        <strong>{{ services.length }}</strong>
      </article>
      <article>
        <span>{{ t('resource.gslb.summary.active') }}</span>
        <strong>{{ activeCount }}</strong>
      </article>
      <article>
        <span>{{ t('resource.gslb.summary.risk') }}</span>
        <strong>{{ riskCount }}</strong>
      </article>
    </div>

    <div v-if="loading" class="gslb-loading">{{ t('resource.gslb.loading') }}</div>

    <div v-else-if="error" class="gslb-error" role="alert">{{ error }}</div>

    <div v-else-if="services.length === 0" class="gslb-empty-panel">
      <div class="empty-copy">
        <h3>{{ t('resource.gslb.empty.title') }}</h3>
        <p>{{ t('resource.gslb.empty.desc') }}</p>
      </div>
      <div class="empty-steps" aria-label="GSLB 工作流">
        <span>{{ t('resource.gslb.empty.step1') }}</span>
        <span>{{ t('resource.gslb.empty.step2') }}</span>
        <span>{{ t('resource.gslb.empty.step3') }}</span>
      </div>
    </div>

    <template v-else>
      <div class="gslb-table">
        <table>
          <thead>
            <tr>
              <th>{{ t('resource.gslb.table.domain') }}</th>
              <th>{{ t('resource.gslb.table.status') }}</th>
              <th>{{ t('resource.gslb.table.healthyPools') }}</th>
              <th>{{ t('resource.gslb.table.dnsTargets') }}</th>
              <th>{{ t('resource.gslb.table.lastSwitch') }}</th>
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
                >{{ t('resource.gslb.table.detail') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="selected" class="gslb-detail">
        <div class="gslb-detail-head">
          <h3>{{ selected.domain }}</h3>
          <span class="state-pill" :class="stateClass(selected.lifecycleState)">
            {{ gslbStateLabel(selected.lifecycleState) }}
          </span>
        </div>

        <div class="gslb-tabs" role="tablist">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            class="gslb-tab"
            :class="{ active: activeTab === tab.key }"
            @click="setTab(tab.key)"
          >{{ tab.label }}</button>
        </div>

        <!-- 概览 -->
        <div v-if="activeTab === 'overview'" class="gslb-tab-panel" role="tabpanel">
          <div class="gslb-svc-actions">
            <button type="button" class="btn-small" :disabled="acting" @click="editOpen = true">
              {{ t('resource.gslb.overview.edit') }}
            </button>
            <button type="button" class="btn-small danger" :disabled="acting" @click="openDelete">
              {{ t('resource.gslb.overview.delete') }}
            </button>
          </div>
          <dl class="gslb-overview">
            <div>
              <dt>{{ t('resource.gslb.overview.domain') }}</dt>
              <dd>{{ selected.domain }}</dd>
            </div>
            <div>
              <dt>{{ t('resource.gslb.overview.currentTargets') }}</dt>
              <dd class="mono">{{ selected.currentDnsTargets.join(', ') || '—' }}</dd>
            </div>
            <div>
              <dt>{{ t('resource.gslb.overview.primaryPool') }}</dt>
              <dd class="mono">{{ primaryPoolLabel }}</dd>
            </div>
            <div>
              <dt>{{ t('resource.gslb.overview.lastSwitchAt') }}</dt>
              <dd>{{ selected.lastSwitchAt ? new Date(selected.lastSwitchAt).toLocaleString() : '—' }}</dd>
            </div>
            <div v-if="selected.healthCheck">
              <dt>{{ t('resource.gslb.overview.healthCheck') }}</dt>
              <dd class="mono">{{ healthCheckSummary }}</dd>
            </div>
            <div>
              <dt>{{ t('resource.gslb.overview.lastDrill') }}</dt>
              <dd>
                <span v-if="selected.lastDrillVerdict" class="state-pill" :class="verdictClass(selected.lastDrillVerdict)">
                  {{ verdictLabel(selected.lastDrillVerdict) }}
                </span>
                <span v-if="selected.lastDrillAt" class="gslb-drill-time">
                  {{ new Date(selected.lastDrillAt).toLocaleString() }}
                </span>
                <span v-if="!selected.lastDrillVerdict">—</span>
              </dd>
            </div>
          </dl>

          <div class="gslb-actions">
            <label v-if="failoverCandidates.length > 0" class="gslb-target">
              {{ t('resource.gslb.overview.targetPool') }}
              <select v-model="targetPoolId" class="gslb-select">
                <option v-for="p in failoverCandidates" :key="p" :value="p">{{ poolLabel(p) }}</option>
              </select>
            </label>
            <button
              type="button"
              class="btn-small danger"
              :disabled="acting || selected.lifecycleState === 'FailingOver' || failoverCandidates.length === 0"
              @click="run('gslb.failover', t('resource.gslb.overview.failoverReason'))"
            >{{ t('resource.gslb.overview.failover') }}</button>
            <button
              type="button"
              class="btn-small"
              :disabled="acting || selected.lifecycleState === 'FailingOver'"
              @click="run('gslb.switchback', t('resource.gslb.overview.switchbackReason'))"
            >{{ t('resource.gslb.overview.switchback') }}</button>
            <button
              type="button"
              class="btn-small"
              :disabled="acting"
              @click="run('gslb.drill', t('resource.gslb.overview.drillReason'))"
            >{{ t('resource.gslb.overview.drill') }}</button>
          </div>

          <div v-if="acting" class="gslb-loading">{{ t('resource.gslb.overview.submitting') }}</div>

          <div v-if="lastRequest" class="gslb-request" :class="requestClass(lastRequest.status)">
            <strong>{{ gslbKindLabel(lastRequest.intentKind) }}</strong>
            {{ t('resource.gslb.overview.lastRequestStatus') }}：{{ gslbSwitchStatusLabel(lastRequest.status) }}（{{ lastRequest.status }}）
            <span v-if="lastRequest.requireApproval">{{ t('resource.gslb.overview.requireApproval') }}</span>
            <span v-if="lastRequest.error"> · {{ lastRequest.error }}</span>
            <div v-if="lastRequest.status === 'PendingApproval'" class="approval-actions">
              <button type="button" class="btn-small" @click="setTab('switchHistory')">
                {{ t('resource.gslb.overview.goApprove') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 地址池 -->
        <div v-else-if="activeTab === 'pools'" class="gslb-tab-panel" role="tabpanel">
          <div class="gslb-pools-toolbar">
            <button
              type="button"
              class="btn-small"
              :disabled="acting || poolsLoading || !!poolsError || !poolsLoaded"
              @click="poolsEditOpen = true"
            >{{ t('resource.gslb.pools.editConfig') }}</button>
          </div>
          <div v-if="poolsLoading" class="gslb-loading">{{ t('resource.gslb.pools.loading') }}</div>
          <div v-else-if="poolsError" class="gslb-error" role="alert">{{ poolsError }}</div>
          <div v-else-if="sortedPools.length === 0" class="gslb-drill-empty">{{ t('resource.gslb.pools.empty') }}</div>
          <div v-else class="gslb-pools">
            <article v-for="pool in sortedPools" :key="pool.poolId" class="gslb-pool" :class="{ active: isPrimaryPool(pool) }">
              <header class="gslb-pool-head">
                <strong class="gslb-pool-name">{{ pool.name || pool.poolId }}</strong>
                <span class="pool-role" :class="isPrimaryPool(pool) ? 'primary' : 'backup'">
                  {{ isPrimaryPool(pool) ? t('resource.gslb.pools.primary') : t('resource.gslb.pools.backup') }}
                </span>
                <span class="state-pill" :class="poolHealthClass(pool)">{{ poolHealthLabel(pool) }}</span>
                <span class="gslb-pool-priority">{{ t('resource.gslb.pools.priority') }} {{ pool.priority }}</span>
                <button
                  type="button"
                  class="btn-small"
                  :disabled="acting || pool.members.length === 0"
                  @click="toggleWeightEdit(pool)"
                >{{ t('resource.gslb.pools.adjustWeight') }}</button>
              </header>
              <div v-if="pool.members.length === 0" class="gslb-drill-empty">{{ t('resource.gslb.pools.emptyMembers') }}</div>
              <ul v-else class="gslb-pool-members">
                <li v-for="member in pool.members" :key="member.clusterId">
                  <span class="mono">{{ member.clusterId }}</span>
                  <span class="weight-bar" :style="{ '--weight': weightPercent(pool, member) + '%' }">
                    <i></i>
                  </span>
                  <span class="weight-value">{{ t('resource.gslb.pools.weight') }} {{ member.weight }}</span>
                </li>
              </ul>
              <div v-if="weightEditPoolId === pool.poolId" class="gslb-weight-edit">
                <div v-for="member in pool.members" :key="member.clusterId" class="gslb-weight-row">
                  <span class="mono">{{ member.clusterId }}</span>
                  <input
                    v-model.number="weightEditValues[member.clusterId]"
                    type="number"
                    min="0"
                    max="100"
                    step="1"
                    class="gslb-weight-input"
                  />
                </div>
                <div class="gslb-weight-sum" :class="{ exceeded: weightEditSum > 100 }">
                  {{ t('resource.gslb.pools.weightSum') }}：{{ weightEditSum }} / 100
                </div>
                <p class="gslb-weight-hint">{{ t('resource.gslb.pools.weightHint') }}</p>
                <div v-if="weightEditError" class="gslb-weight-error" role="alert">{{ weightEditError }}</div>
                <div class="gslb-weight-actions">
                  <button
                    type="button"
                    class="btn-small"
                    :disabled="acting || weightEditSum > 100"
                    @click="submitWeightUpdate(pool)"
                  >{{ t('resource.gslb.pools.weightSubmit') }}</button>
                  <button type="button" class="btn-small" :disabled="acting" @click="weightEditPoolId = ''">
                    {{ t('common.cancel') }}
                  </button>
                </div>
              </div>
            </article>
          </div>
          <div v-if="lastRequest && lastRequest.intentKind === 'gslb.weight-update'" class="gslb-request" :class="requestClass(lastRequest.status)">
            <strong>{{ gslbKindLabel(lastRequest.intentKind) }}</strong>
            {{ t('resource.gslb.overview.lastRequestStatus') }}：{{ gslbSwitchStatusLabel(lastRequest.status) }}（{{ lastRequest.status }}）
            <span v-if="lastRequest.error"> · {{ lastRequest.error }}</span>
          </div>
        </div>

        <!-- 切换记录 -->
        <div v-else-if="activeTab === 'switchHistory'" class="gslb-tab-panel" role="tabpanel">
          <div class="gslb-switch-filter">
            <select v-model="switchStatusFilter" class="gslb-select" @change="loadSwitchRequests(true)">
              <option value="">{{ t('resource.gslb.switch.filterAll') }}</option>
              <option v-for="s in switchStatuses" :key="s" :value="s">{{ gslbSwitchStatusLabel(s) }}</option>
            </select>
          </div>
          <div v-if="switchLoading" class="gslb-loading">{{ t('resource.gslb.switch.loading') }}</div>
          <div v-else-if="switchError" class="gslb-error" role="alert">{{ switchError }}</div>
          <div v-else-if="switchRequests.length === 0" class="gslb-drill-empty">{{ t('resource.gslb.switch.empty') }}</div>
          <div v-else class="gslb-table gslb-switch-table">
            <table>
              <thead>
                <tr>
                  <th>{{ t('resource.gslb.switch.colTime') }}</th>
                  <th>{{ t('resource.gslb.switch.colKind') }}</th>
                  <th>{{ t('resource.gslb.switch.colStatus') }}</th>
                  <th>{{ t('resource.gslb.switch.colActor') }}</th>
                  <th>{{ t('resource.gslb.switch.colReason') }}</th>
                  <th>{{ t('resource.gslb.switch.colError') }}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="req in switchRequests" :key="req.id">
                  <td>{{ new Date(req.createdAt).toLocaleString() }}</td>
                  <td>{{ gslbKindLabel(req.intentKind) }}</td>
                  <td>
                    <span class="state-pill" :class="requestClass(req.status)">
                      {{ gslbSwitchStatusLabel(req.status) }}
                    </span>
                  </td>
                  <td>{{ req.actorId || '—' }}</td>
                  <td>{{ req.reason || '—' }}</td>
                  <td class="gslb-error-text">{{ req.error || '—' }}</td>
                  <td>
                    <div v-if="req.status === 'PendingApproval'" class="approval-actions">
                      <button type="button" class="btn-small" :disabled="acting" @click="approveRequest(req)">
                        {{ t('resource.gslb.switch.approve') }}
                      </button>
                      <button type="button" class="btn-small danger" :disabled="acting" @click="rejectRequest(req)">
                        {{ t('resource.gslb.switch.reject') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- 演练报告 -->
        <div v-else class="gslb-tab-panel" role="tabpanel">
          <div class="gslb-drills">
            <div v-if="drillsLoading" class="gslb-loading">{{ t('common.loading') }}</div>
            <div v-else-if="drillsError" class="gslb-error" role="alert">{{ drillsError }}</div>
            <div v-else-if="drills.length === 0" class="gslb-drill-empty">{{ t('resource.gslb.drills.empty') }}</div>
            <div v-for="drill in drills" :key="drill.id" class="gslb-drill" :class="verdictClass(drill.verdict)">
              <div class="gslb-drill-head">
                <span class="state-pill" :class="verdictClass(drill.verdict)">{{ verdictLabel(drill.verdict) }}</span>
                <span class="gslb-drill-time">{{ new Date(drill.createdAt).toLocaleString() }}</span>
              </div>
              <div class="gslb-drill-body">
                <div v-if="drill.report.targetPoolId" class="gslb-drill-diff">
                  <div class="gslb-drill-diff-col">
                    <span class="gslb-drill-diff-title">{{ t('resource.gslb.drills.currentTargets') }}</span>
                    <ul>
                      <li
                        v-for="target in drill.report.currentTargets"
                        :key="target"
                        class="mono"
                        :class="{ removed: isRemovedTarget(drill.report, target) }"
                      >
                        {{ target }}
                        <em v-if="isRemovedTarget(drill.report, target)">{{ t('resource.gslb.drills.removed') }}</em>
                      </li>
                      <li v-if="drill.report.currentTargets.length === 0" class="gslb-drill-empty">—</li>
                    </ul>
                  </div>
                  <div class="gslb-drill-diff-col">
                    <span class="gslb-drill-diff-title">{{ t('resource.gslb.drills.projectedTargets') }}</span>
                    <ul>
                      <li
                        v-for="target in drill.report.projectedTargets"
                        :key="target"
                        class="mono"
                        :class="{ added: isAddedTarget(drill.report, target) }"
                      >
                        {{ target }}
                        <em v-if="isAddedTarget(drill.report, target)">{{ t('resource.gslb.drills.added') }}</em>
                      </li>
                      <li v-if="drill.report.projectedTargets.length === 0" class="gslb-drill-empty">—</li>
                    </ul>
                  </div>
                </div>
                <div v-else>
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
      </div>
    </template>

    <GslbServiceWizard v-model="wizardOpen" @created="onServiceCreated" />
    <GslbServiceWizard
      v-model="editOpen"
      mode="editService"
      :service="selected"
      @updated="onServiceUpdated"
    />
    <GslbServiceWizard
      v-model="poolsEditOpen"
      mode="editPools"
      :service="selected"
      :initial-pools="pools"
      @pools-replaced="onPoolsReplaced"
    />

    <!-- 删除确认对话框（危险操作；Active 等状态需强制删除） -->
    <div v-if="deleteOpen" class="gslb-dialog-overlay" @click.self="cancelDelete">
      <div class="gslb-dialog" role="dialog" aria-modal="true" :aria-label="t('resource.gslb.delete.title')">
        <h3>{{ t('resource.gslb.delete.title') }}</h3>
        <p class="gslb-dialog-desc">{{ t('resource.gslb.delete.desc', { domain: selected?.domain ?? '' }) }}</p>
        <template v-if="deleteNeedsForce">
          <p class="gslb-dialog-warn">{{ t('resource.gslb.delete.forceHint') }}</p>
          <label class="gslb-dialog-check">
            <input v-model="deleteForce" type="checkbox" />
            <span>{{ t('resource.gslb.delete.forceLabel') }}</span>
          </label>
        </template>
        <div v-if="deleteError" class="gslb-dialog-error" role="alert">{{ deleteError }}</div>
        <footer class="gslb-dialog-foot">
          <button type="button" class="btn-small" :disabled="deleting" @click="cancelDelete">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn-small danger"
            :disabled="deleting || (deleteNeedsForce && !deleteForce)"
            @click="confirmDelete"
          >{{ deleting ? t('resource.gslb.delete.deleting') : t('resource.gslb.delete.confirm') }}</button>
        </footer>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  approveSwitchRequest,
  deleteGslbService,
  getGslbService,
  getGslbSwitchRequest,
  GSLB_TERMINAL_STATUSES,
  gslbKindLabel,
  gslbStateLabel,
  gslbSwitchStatusLabel,
  listGslbDrillReports,
  listGslbPools,
  listGslbServices,
  listGslbSwitchRequests,
  rejectSwitchRequest,
  submitGslbIntent,
  type GslbDrillReport,
  type GslbDrillReportPayload,
  type GslbDrillVerdict,
  type GslbIntentKind,
  type GslbPool,
  type GslbPoolMember,
  type GslbReadModel,
  type GslbSwitchRequest,
} from '../gslbApi'
import GslbServiceWizard from './GslbServiceWizard.vue'

const { t } = useI18n()

type TabKey = 'overview' | 'pools' | 'switchHistory' | 'drills'

const loading = ref(true)
const error = ref<string | null>(null)
const services = ref<GslbReadModel[]>([])
const selected = ref<GslbReadModel | null>(null)
const acting = ref(false)
const lastRequest = ref<GslbSwitchRequest | null>(null)
const targetPoolId = ref('')

const activeTab = ref<TabKey>('overview')
const pools = ref<GslbPool[]>([])
const poolsLoading = ref(false)
const poolsError = ref<string | null>(null)
const poolsLoaded = ref(false)
const switchRequests = ref<GslbSwitchRequest[]>([])
const switchLoading = ref(false)
const switchError = ref<string | null>(null)
const switchLoaded = ref(false)
const switchStatusFilter = ref('')
const drills = ref<GslbDrillReport[]>([])
const drillsLoading = ref(false)
const drillsError = ref<string | null>(null)
const drillsLoaded = ref(false)

const wizardOpen = ref(false)
const editOpen = ref(false)
const poolsEditOpen = ref(false)
// 删除确认对话框状态
const deleteOpen = ref(false)
const deleteForce = ref(false)
const deleting = ref(false)
const deleteError = ref<string | null>(null)
// 调权编辑态：按 poolId 展开，values 以 clusterId 为键
const weightEditPoolId = ref('')
const weightEditValues = ref<Record<string, number>>({})
const weightEditError = ref<string | null>(null)

const switchStatuses = [
  'PendingApproval',
  'Approved',
  'Dispatched',
  'Succeeded',
  'Failed',
  'Rejected',
  'DrillCompleted',
]

const tabs = computed(() => [
  { key: 'overview' as TabKey, label: t('resource.gslb.tabs.overview') },
  { key: 'pools' as TabKey, label: t('resource.gslb.tabs.pools') },
  { key: 'switchHistory' as TabKey, label: t('resource.gslb.tabs.switchHistory') },
  { key: 'drills' as TabKey, label: t('resource.gslb.tabs.drills') },
])

const activeCount = computed(() => services.value.filter((svc) => svc.lifecycleState === 'Active').length)
const riskCount = computed(() => services.value.filter((svc) => svc.lifecycleState === 'Degraded' || svc.lifecycleState === 'FailingOver').length)

// 故障转移只允许切到健康且非当前主池的池（后端强制 targetPoolId）。
const failoverCandidates = computed(() => {
  const svc = selected.value
  if (!svc) return [] as string[]
  return svc.healthyPools.filter((p) => p && p !== svc.activePoolId)
})

const sortedPools = computed(() => [...pools.value].sort((a, b) => a.priority - b.priority))

const primaryPoolLabel = computed(() => {
  const svc = selected.value
  if (!svc?.activePoolId) return '—'
  const pool = pools.value.find((p) => p.poolId === svc.activePoolId)
  return pool ? pool.name || pool.poolId : svc.activePoolId
})

// 概览页签的健康检查摘要：协议://路径 · 间隔 Xs · 超时 Xs · 阈值 N
const healthCheckSummary = computed(() => {
  const hc = selected.value?.healthCheck
  if (!hc) return ''
  return t('resource.gslb.overview.healthCheckSummary', {
    protocol: hc.protocol,
    path: hc.path,
    interval: hc.intervalSeconds,
    timeout: hc.timeoutSeconds,
    threshold: hc.failureThreshold,
  })
})

const weightEditSum = computed(() =>
  Object.values(weightEditValues.value).reduce((sum, w) => sum + (Number(w) || 0), 0),
)

function isPrimaryPool(pool: GslbPool): boolean {
  return pool.isActive || pool.poolId === selected.value?.activePoolId
}

function poolHealthClass(pool: GslbPool): string {
  if (pool.healthy === true) return 'ok'
  if (pool.healthy === false) return 'err'
  return 'muted'
}

function poolHealthLabel(pool: GslbPool): string {
  if (pool.healthy === true) return t('resource.gslb.pools.health.healthy')
  if (pool.healthy === false) return t('resource.gslb.pools.health.unhealthy')
  return t('resource.gslb.pools.health.unknown')
}

// 权重条按池内权重总和归一；总和为 0 时退化为仅显示数值。
function weightPercent(pool: GslbPool, member: GslbPoolMember): number {
  const total = pool.members.reduce((sum, m) => sum + (m.weight || 0), 0)
  if (total <= 0) return 0
  return Math.round(((member.weight || 0) / total) * 100)
}

// 演练 diff：仅在投影目标中存在的为新增，仅在当前目标中存在的为移除。
function isAddedTarget(report: GslbDrillReportPayload, target: string): boolean {
  return !report.currentTargets.includes(target)
}

function isRemovedTarget(report: GslbDrillReportPayload, target: string): boolean {
  return !report.projectedTargets.includes(target)
}

function toggleWeightEdit(pool: GslbPool): void {
  if (weightEditPoolId.value === pool.poolId) {
    weightEditPoolId.value = ''
    return
  }
  weightEditPoolId.value = pool.poolId
  weightEditError.value = null
  const values: Record<string, number> = {}
  for (const member of pool.members) values[member.clusterId] = member.weight
  weightEditValues.value = values
}

/** 调权：免审批意图，提交后沿用 lastRequest/轮询/刷新模式（灰度生效） */
async function submitWeightUpdate(pool: GslbPool): Promise<void> {
  if (!selected.value || acting.value) return
  weightEditError.value = null
  const weights: Record<string, number> = {}
  for (const member of pool.members) {
    const weight = Number(weightEditValues.value[member.clusterId])
    if (!Number.isInteger(weight) || weight < 0 || weight > 100) {
      weightEditError.value = t('resource.gslb.pools.weightRangeError')
      return
    }
    weights[member.clusterId] = weight
  }
  if (Object.values(weights).reduce((sum, w) => sum + w, 0) > 100) {
    weightEditError.value = t('resource.gslb.pools.weightSumError')
    return
  }
  acting.value = true
  lastRequest.value = null
  error.value = null
  try {
    lastRequest.value = await submitGslbIntent(
      selected.value.serviceId,
      'gslb.weight-update',
      undefined,
      t('resource.gslb.pools.weightReason'),
      weights,
    )
    weightEditPoolId.value = ''
    await refreshSelected()
    if (lastRequest.value && !GSLB_TERMINAL_STATUSES.has(lastRequest.value.status)) {
      void pollRequest(lastRequest.value.id)
    }
  } catch (err) {
    weightEditError.value = toMessage(err)
  } finally {
    acting.value = false
  }
}

/** 创建成功：刷新列表并自动选中新服务进详情 */
async function onServiceCreated(service: GslbReadModel): Promise<void> {
  await load()
  if (service?.serviceId) await select(service.serviceId)
}

/** 编辑保存成功：走统一投影刷新（refreshSelected 会联动已加载页签） */
async function onServiceUpdated(): Promise<void> {
  await refreshSelected()
}

/** 池配置整组替换成功：刷新投影与池列表 */
async function onPoolsReplaced(): Promise<void> {
  await refreshSelected()
}

// 仅 Inactive/Paused 可直接删除；其余状态须显式 force=true（后端同样校验）。
const deleteNeedsForce = computed(() => {
  const state = selected.value?.lifecycleState
  return state !== 'Inactive' && state !== 'Paused'
})

function openDelete(): void {
  deleteForce.value = false
  deleteError.value = null
  deleteOpen.value = true
}

function cancelDelete(): void {
  if (deleting.value) return
  deleteOpen.value = false
}

/** 删除成功：清空选中并返回列表刷新；409（在途切换请求）展示后端错误 */
async function confirmDelete(): Promise<void> {
  const svc = selected.value
  if (!svc || deleting.value) return
  deleting.value = true
  deleteError.value = null
  try {
    await deleteGslbService(svc.serviceId, deleteNeedsForce.value && deleteForce.value)
    deleteOpen.value = false
    selected.value = null
    await load()
  } catch (err) {
    deleteError.value = toMessage(err)
  } finally {
    deleting.value = false
  }
}

function poolLabel(poolId: string): string {
  return poolId === selected.value?.activePoolId
    ? `${poolId.slice(0, 8)}…（${t('resource.gslb.pools.primary')}）`
    : `${poolId.slice(0, 8)}…`
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

function toMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    const res = await listGslbServices()
    services.value = res.items ?? []
  } catch (err) {
    error.value = toMessage(err)
  } finally {
    loading.value = false
  }
}

async function refreshSelected(): Promise<void> {
  const svc = selected.value
  if (!svc) return
  selected.value = await getGslbService(svc.serviceId)
  const idx = services.value.findIndex((item) => item.serviceId === svc.serviceId)
  if (idx >= 0) services.value[idx] = selected.value
  // 已加载过的页签跟随投影一并刷新，未加载的保持懒加载。
  await Promise.all([
    poolsLoaded.value ? loadPools() : Promise.resolve(),
    switchLoaded.value ? loadSwitchRequests() : Promise.resolve(),
    drillsLoaded.value ? loadDrills() : Promise.resolve(),
  ])
}

async function select(serviceId: string): Promise<void> {
  try {
    error.value = null
    lastRequest.value = null
    activeTab.value = 'overview'
    poolsLoaded.value = false
    switchLoaded.value = false
    drillsLoaded.value = false
    switchStatusFilter.value = ''
    pools.value = []
    switchRequests.value = []
    drills.value = []
    weightEditPoolId.value = ''
    weightEditError.value = null
    selected.value = await getGslbService(serviceId)
    const idx = services.value.findIndex((svc) => svc.serviceId === serviceId)
    if (idx >= 0) services.value[idx] = selected.value
    targetPoolId.value = failoverCandidates.value[0] ?? ''
  } catch (err) {
    error.value = toMessage(err)
  }
}

function setTab(tab: TabKey): void {
  activeTab.value = tab
  if (tab === 'pools' && !poolsLoaded.value) void loadPools()
  else if (tab === 'switchHistory' && !switchLoaded.value) void loadSwitchRequests(true)
  else if (tab === 'drills' && !drillsLoaded.value) void loadDrills()
}

async function loadPools(): Promise<void> {
  const svc = selected.value
  if (!svc || poolsLoading.value) return
  poolsLoading.value = true
  poolsError.value = null
  try {
    const res = await listGslbPools(svc.serviceId)
    pools.value = res.items ?? []
    poolsLoaded.value = true
  } catch (err) {
    poolsError.value = toMessage(err)
  } finally {
    poolsLoading.value = false
  }
}

async function loadSwitchRequests(showLoading = false): Promise<void> {
  const svc = selected.value
  if (!svc || switchLoading.value) return
  if (showLoading) switchLoading.value = true
  switchError.value = null
  try {
    const res = await listGslbSwitchRequests(svc.serviceId, switchStatusFilter.value || undefined)
    switchRequests.value = res.items ?? []
    switchLoaded.value = true
  } catch (err) {
    switchError.value = toMessage(err)
  } finally {
    switchLoading.value = false
  }
}

async function loadDrills(): Promise<void> {
  const svc = selected.value
  if (!svc || drillsLoading.value) return
  drillsLoading.value = true
  drillsError.value = null
  try {
    const res = await listGslbDrillReports(svc.serviceId)
    drills.value = res.items ?? []
    drillsLoaded.value = true
  } catch (err) {
    drillsError.value = toMessage(err)
  } finally {
    drillsLoading.value = false
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
      error.value = t('resource.gslb.overview.noFailoverTarget')
      acting.value = false
      return
    }
  } else if (kind === 'gslb.switchback') {
    target = targetPoolId.value
    if (!target) {
      error.value = t('resource.gslb.overview.noSwitchbackTarget')
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
    await refreshSelected()
    if (lastRequest.value && !GSLB_TERMINAL_STATUSES.has(lastRequest.value.status)) {
      void pollRequest(lastRequest.value.id)
    }
  } catch (err) {
    error.value = toMessage(err)
  } finally {
    acting.value = false
  }
}

/** 审批闭环：切换记录页签内批准，随后轮询至终态 */
async function approveRequest(request: GslbSwitchRequest): Promise<void> {
  if (acting.value) return
  acting.value = true
  error.value = null
  try {
    lastRequest.value = await approveSwitchRequest(request.id)
    void pollRequest(request.id)
  } catch (err) {
    error.value = toMessage(err)
  } finally {
    acting.value = false
  }
}

async function rejectRequest(request: GslbSwitchRequest): Promise<void> {
  if (acting.value) return
  acting.value = true
  error.value = null
  try {
    lastRequest.value = await rejectSwitchRequest(request.id)
    await refreshSelected()
  } catch (err) {
    error.value = toMessage(err)
  } finally {
    acting.value = false
  }
}

async function pollRequest(requestId: string): Promise<void> {
  for (let i = 0; i < 10; i += 1) {
    await new Promise((resolve) => window.setTimeout(resolve, 1000))
    try {
      const next = await getGslbSwitchRequest(requestId)
      lastRequest.value = next
      await refreshSelected()
      if (GSLB_TERMINAL_STATUSES.has(next.status)) return
    } catch (err) {
      error.value = toMessage(err)
      return
    }
  }
}

onMounted(load)
</script>

<style scoped>
.gslb-page { padding: 16px; color: var(--hnb-color-text-primary, #edeff5); }
.gslb-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.gslb-header h2 { margin: 0 0 4px; font-size: 18px; }
.gslb-create-btn { flex-shrink: 0; margin-top: 2px; border-color: var(--hnb-color-primary, #5b8dff); color: var(--hnb-color-primary, #5b8dff); }
.gslb-desc { margin: 0 0 16px; font-size: 13px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.gslb-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
.gslb-summary article { padding: 14px 16px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: var(--hnb-color-bg-elevated, #171d31); }
.gslb-summary span { display: block; margin-bottom: 8px; color: var(--hnb-color-text-tertiary, #8a94a3); font-size: 12px; }
.gslb-summary strong { color: var(--hnb-color-text-primary, #edeff5); font-size: 24px; }
.gslb-loading, .gslb-error { padding: 32px; text-align: center; font-size: 14px; }
.gslb-tab-panel .gslb-loading, .gslb-tab-panel .gslb-error { padding: 20px; }
.gslb-empty-panel { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 20px; align-items: center; padding: 28px; border: 1px dashed var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-elevated, #171d31); }
.empty-copy h3 { margin: 0 0 8px; font-size: 16px; color: var(--hnb-color-text-primary, #edeff5); }
.empty-copy p { margin: 0; max-width: 680px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; line-height: 1.7; }
.empty-steps { display: flex; flex-direction: column; gap: 8px; min-width: 150px; }
.empty-steps span { padding: 8px 10px; border-radius: 999px; background: rgba(47, 111, 237, 0.12); color: var(--hnb-color-primary, #2f6fed); font-size: 12px; text-align: center; }
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
.state-pill.err { background: rgba(240, 68, 56, 0.15); color: #f04438; }
.state-pill.pending { background: rgba(91, 141, 255, 0.15); color: #5b8dff; }
.gslb-detail { margin-top: 16px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; padding: 16px; }
.gslb-detail-head { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.gslb-detail-head h3 { margin: 0; font-size: 15px; }
.gslb-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-border, #29344a); margin-bottom: 16px; }
.gslb-tab {
  padding: 8px 14px; border: none; border-bottom: 2px solid transparent; background: transparent;
  color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; cursor: pointer;
}
.gslb-tab:hover { color: var(--hnb-color-text-primary, #edeff5); }
.gslb-tab.active { color: var(--hnb-color-primary, #5b8dff); border-bottom-color: var(--hnb-color-primary, #5b8dff); }
.gslb-overview { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; margin: 0 0 16px; }
.gslb-overview dt { margin-bottom: 4px; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.gslb-overview dd { margin: 0; font-size: 13px; color: var(--hnb-color-text-primary, #edeff5); display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
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
.approval-actions { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
.gslb-switch-table .approval-actions { margin-top: 0; }
.gslb-switch-filter { display: flex; justify-content: flex-end; margin-bottom: 10px; }
.gslb-error-text { color: var(--hnb-color-status-danger, #f04438); }
.gslb-pools { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
.gslb-pool { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; padding: 12px 14px; background: var(--hnb-color-bg-elevated, #171d31); }
.gslb-pool.active { border-color: var(--hnb-color-primary, #5b8dff); }
.gslb-pool-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 10px; }
.gslb-pool-name { font-size: 14px; }
.gslb-pool-priority { margin-left: auto; font-size: 12px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.pool-role { padding: 2px 8px; border-radius: 999px; font-size: 12px; }
.pool-role.primary { background: rgba(91, 141, 255, 0.15); color: #5b8dff; }
.pool-role.backup { background: rgba(138, 148, 163, 0.15); color: #8a94a3; }
.gslb-pool-members { margin: 0; padding: 0; list-style: none; }
.gslb-pool-members li { display: flex; align-items: center; gap: 10px; padding: 4px 0; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); }
.weight-bar { flex: 1; height: 6px; border-radius: 999px; background: rgba(138, 148, 163, 0.15); overflow: hidden; }
.weight-bar i { display: block; height: 100%; width: var(--weight, 0%); background: var(--hnb-color-primary, #5b8dff); border-radius: 999px; }
.weight-value { min-width: 64px; text-align: right; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.gslb-drills h4 { margin: 0 0 8px; font-size: 14px; }
.gslb-drill-empty { font-size: 13px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.gslb-drill { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; padding: 10px 12px; margin-bottom: 8px; font-size: 13px; }
.gslb-drill-head { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.gslb-drill-time { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.gslb-drill-body { color: var(--hnb-color-text-secondary, #a9b2c2); }
.gslb-drill-checks { margin: 6px 0 0; padding-left: 4px; list-style: none; }
.gslb-drill-checks li { padding: 2px 0; color: #12b76a; }
.gslb-drill-checks li.failed { color: var(--hnb-color-status-danger, #f04438); }
.gslb-weight-edit { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--hnb-color-divider, #222b3d); }
.gslb-weight-row { display: flex; align-items: center; gap: 10px; padding: 4px 0; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); }
.gslb-weight-row .mono { flex: 1; }
.gslb-weight-input {
  width: 90px; padding: 5px 8px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px;
}
.gslb-weight-input:focus { outline: none; border-color: var(--hnb-color-primary, #5b8dff); }
.gslb-weight-sum { margin-top: 8px; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.gslb-weight-sum.exceeded { color: var(--hnb-color-status-danger, #f04438); }
.gslb-weight-hint { margin: 6px 0 0; font-size: 12px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.gslb-weight-error {
  margin-top: 8px; padding: 8px 12px; border-radius: 6px; font-size: 13px;
  background: rgba(240, 68, 56, 0.1); color: var(--hnb-color-status-danger, #f04438);
}
.gslb-weight-actions { display: flex; gap: 8px; margin-top: 10px; }
.gslb-svc-actions { display: flex; gap: 8px; justify-content: flex-end; margin-bottom: 12px; }
.gslb-pools-toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; }
.gslb-dialog-overlay {
  position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center;
  background: rgba(10, 14, 24, 0.6); padding: 24px;
}
.gslb-dialog {
  width: 440px; max-width: 100%; padding: 18px;
  border: 1px solid var(--hnb-color-border, #29344a); border-radius: 10px;
  background: var(--hnb-color-bg-base, #10152a); color: var(--hnb-color-text-primary, #edeff5);
}
.gslb-dialog h3 { margin: 0 0 10px; font-size: 15px; }
.gslb-dialog-desc { margin: 0 0 10px; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); line-height: 1.6; }
.gslb-dialog-warn { margin: 0 0 10px; font-size: 13px; color: var(--hnb-color-status-warning, #f79009); }
.gslb-dialog-check { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 13px; }
.gslb-dialog-error {
  margin-bottom: 12px; padding: 8px 12px; border-radius: 6px; font-size: 13px;
  background: rgba(240, 68, 56, 0.1); color: var(--hnb-color-status-danger, #f04438);
}
.gslb-dialog-foot { display: flex; justify-content: flex-end; gap: 8px; }
.gslb-drill-diff { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; margin-bottom: 6px; }
.gslb-drill-diff-col { border: 1px solid var(--hnb-color-border, #29344a); border-radius: 6px; padding: 8px 10px; }
.gslb-drill-diff-title { display: block; margin-bottom: 6px; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.gslb-drill-diff-col ul { margin: 0; padding: 0; list-style: none; }
.gslb-drill-diff-col li { display: flex; align-items: center; gap: 6px; padding: 2px 0; }
.gslb-drill-diff-col li em { font-style: normal; font-size: 11px; }
.gslb-drill-diff-col li.added { color: #12b76a; }
.gslb-drill-diff-col li.added em { padding: 1px 6px; border-radius: 999px; background: rgba(18, 183, 106, 0.15); }
.gslb-drill-diff-col li.removed { color: var(--hnb-color-status-danger, #f04438); text-decoration: line-through; }
.gslb-drill-diff-col li.removed em { padding: 1px 6px; border-radius: 999px; background: rgba(240, 68, 56, 0.15); text-decoration: none; }

@media (max-width: 720px) {
  .gslb-summary { grid-template-columns: 1fr; }
  .gslb-empty-panel { grid-template-columns: 1fr; }
  .empty-steps { min-width: 0; }
}
</style>
