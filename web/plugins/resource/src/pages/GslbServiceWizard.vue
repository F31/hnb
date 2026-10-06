<template>
  <div v-if="modelValue" class="wizard-overlay" @click.self="close">
    <div class="wizard-dialog" role="dialog" aria-modal="true" :aria-label="dialogTitle">
      <header class="wizard-head">
        <h3>{{ dialogTitle }}</h3>
        <button type="button" class="wizard-close" :aria-label="t('common.cancel')" @click="close">×</button>
      </header>

      <form class="wizard-body" @submit.prevent="submit">
        <!-- 基本信息（池编辑模式不展示） -->
        <section v-if="mode !== 'editPools'" class="wizard-section">
          <h4>{{ t('resource.gslb.wizard.sectionBasic') }}</h4>
          <label class="wizard-field">
            <span>{{ t('resource.gslb.wizard.name') }}</span>
            <input
              v-model.trim="form.name"
              type="text"
              class="wizard-input"
              :placeholder="mode === 'editService' ? t('resource.gslb.wizard.nameKeepHint') : t('resource.gslb.wizard.namePlaceholder')"
            />
          </label>
          <p v-if="mode === 'editService'" class="wizard-hint">{{ t('resource.gslb.wizard.nameKeepHint') }}</p>
          <label class="wizard-field">
            <span>{{ t('resource.gslb.wizard.domain') }}</span>
            <input
              v-model.trim="form.domain"
              type="text"
              class="wizard-input mono"
              :placeholder="t('resource.gslb.wizard.domainPlaceholder')"
              :readonly="mode === 'editService'"
              :disabled="mode === 'editService'"
            />
          </label>
          <p v-if="mode === 'editService'" class="wizard-hint">{{ t('resource.gslb.wizard.domainReadonly') }}</p>
          <template v-if="mode === 'editService'">
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.requireApproval') }}</span>
              <select v-model="approvalChoice" class="gslb-select">
                <option value="">{{ t('resource.gslb.wizard.approvalKeep') }}</option>
                <option value="true">{{ t('resource.gslb.wizard.approvalOn') }}</option>
                <option value="false">{{ t('resource.gslb.wizard.approvalOff') }}</option>
              </select>
            </label>
          </template>
          <template v-else>
            <label class="wizard-check">
              <input v-model="form.requireApproval" type="checkbox" />
              <span>{{ t('resource.gslb.wizard.requireApproval') }}</span>
            </label>
            <p class="wizard-hint">{{ t('resource.gslb.wizard.requireApprovalHint') }}</p>
          </template>
        </section>

        <!-- 地址池编排（服务编辑模式不展示） -->
        <section v-if="mode !== 'editService'" class="wizard-section">
          <h4>{{ t('resource.gslb.wizard.sectionPools') }}</h4>
          <p v-if="mode === 'editPools'" class="wizard-hint">{{ t('resource.gslb.wizard.poolsEditHint') }}</p>
          <div v-if="clustersError" class="wizard-error" role="alert">{{ clustersError }}</div>
          <article v-for="(pool, pi) in pools" :key="pi" class="wizard-pool">
            <header class="wizard-pool-head">
              <label class="wizard-field">
                <span>{{ t('resource.gslb.wizard.poolName') }}</span>
                <input v-model.trim="pool.name" type="text" class="wizard-input" :placeholder="`pool-${pi}`" />
              </label>
              <label class="wizard-field wizard-field-narrow">
                <span>{{ t('resource.gslb.wizard.priority') }}</span>
                <input v-model.number="pool.priority" type="number" min="0" step="1" class="wizard-input" />
              </label>
              <button
                type="button"
                class="btn-small danger"
                :disabled="pools.length <= 1"
                @click="removePool(pi)"
              >{{ t('resource.gslb.wizard.removePool') }}</button>
            </header>
            <div v-for="(member, mi) in pool.members" :key="mi" class="wizard-member">
              <select v-model="member.clusterId" class="gslb-select wizard-member-cluster">
                <option value="" disabled>{{ t('resource.gslb.wizard.selectCluster') }}</option>
                <option v-for="c in clusters" :key="c.clusterId" :value="c.clusterId">
                  {{ c.displayName || c.clusterId }}
                </option>
              </select>
              <label class="wizard-member-weight">
                <span>{{ t('resource.gslb.wizard.weight') }}</span>
                <input v-model.number="member.weight" type="number" min="0" max="100" step="1" class="wizard-input" />
              </label>
              <button
                type="button"
                class="btn-small danger"
                :disabled="pool.members.length <= 1"
                @click="removeMember(pi, mi)"
              >{{ t('resource.gslb.wizard.removeMember') }}</button>
            </div>
            <button type="button" class="btn-small" @click="addMember(pi)">{{ t('resource.gslb.wizard.addMember') }}</button>
          </article>
          <button type="button" class="btn-small" @click="addPool">{{ t('resource.gslb.wizard.addPool') }}</button>
        </section>

        <!-- 健康检查（池编辑模式不展示） -->
        <section v-if="mode !== 'editPools'" class="wizard-section">
          <h4>{{ t('resource.gslb.wizard.sectionHealthCheck') }}</h4>
          <div class="wizard-hc-grid">
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.protocol') }}</span>
              <select v-model="healthCheck.protocol" class="gslb-select">
                <option value="http">http</option>
                <option value="https">https</option>
              </select>
            </label>
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.path') }}</span>
              <input v-model.trim="healthCheck.path" type="text" class="wizard-input mono" placeholder="/healthz" />
            </label>
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.intervalSeconds') }}</span>
              <input v-model.number="healthCheck.intervalSeconds" type="number" min="5" max="300" step="1" class="wizard-input" />
            </label>
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.timeoutSeconds') }}</span>
              <input v-model.number="healthCheck.timeoutSeconds" type="number" min="1" max="30" step="1" class="wizard-input" />
            </label>
            <label class="wizard-field">
              <span>{{ t('resource.gslb.wizard.failureThreshold') }}</span>
              <input v-model.number="healthCheck.failureThreshold" type="number" min="1" max="10" step="1" class="wizard-input" />
            </label>
          </div>
        </section>

        <div v-if="formError" class="wizard-error" role="alert">{{ formError }}</div>

        <footer class="wizard-foot">
          <button type="button" class="btn-small" :disabled="submitting" @click="close">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn-small wizard-submit" :disabled="submitting">
            {{ submitting ? submittingLabel : submitLabel }}
          </button>
        </footer>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  createGslbService,
  replaceGslbPools,
  updateGslbService,
  type GslbPool,
  type GslbReadModel,
  type GslbServiceDetail,
  type UpdateGslbServicePayload,
} from '../gslbApi'
import { listClusters } from './cluster-management/api/clusterApi'
import type { ClusterSummary } from './cluster-management/types/cluster'

/**
 * 三种模式：
 * - create：创建向导（基本信息 + 池编排 + 健康检查 → POST /services）
 * - editService：服务编辑（名称/审批/健康检查合并式 PATCH；domain 只读）
 * - editPools：地址池整组替换（预填现有 pools → PUT /services/{id}/pools）
 */
type WizardMode = 'create' | 'editService' | 'editPools'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    mode?: WizardMode
    /** editService/editPools 模式的目标服务（editService 预填域名与健康检查） */
    service?: GslbReadModel | null
    /** editPools 模式预填的现有地址池 */
    initialPools?: GslbPool[]
  }>(),
  { mode: 'create', service: null, initialPools: () => [] },
)
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  created: [service: GslbReadModel]
  updated: [detail: GslbServiceDetail]
  poolsReplaced: []
}>()

const { t } = useI18n()

interface MemberForm {
  clusterId: string
  weight: number
}
interface PoolForm {
  name: string
  priority: number
  members: MemberForm[]
}

const DEFAULT_HEALTH_CHECK = {
  protocol: 'http' as 'http' | 'https',
  path: '/healthz',
  intervalSeconds: 10,
  timeoutSeconds: 3,
  failureThreshold: 3,
}

const form = ref({ name: '', domain: '', requireApproval: true })
// editService：'' = 保持不变（合并式 PATCH 不提交该字段）
const approvalChoice = ref<'' | 'true' | 'false'>('')
const pools = ref<PoolForm[]>([])
const healthCheck = ref({ ...DEFAULT_HEALTH_CHECK })
const clusters = ref<ClusterSummary[]>([])
const clustersError = ref<string | null>(null)
const formError = ref<string | null>(null)
const submitting = ref(false)

const mode = computed(() => props.mode)

const dialogTitle = computed(() => {
  if (mode.value === 'editService') return t('resource.gslb.wizard.editTitle')
  if (mode.value === 'editPools') return t('resource.gslb.wizard.editPoolsTitle')
  return t('resource.gslb.wizard.title')
})

const submitLabel = computed(() =>
  mode.value === 'create' ? t('resource.gslb.wizard.submit') : t('resource.gslb.wizard.submitSave'),
)
const submittingLabel = computed(() =>
  mode.value === 'create' ? t('resource.gslb.wizard.submitting') : t('resource.gslb.wizard.saving'),
)

const DOMAIN_RE = /^(?=.{1,253}$)([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$/
// 与后端 gslbNamePattern 一致
const NAME_RE = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/

function reset(): void {
  formError.value = null
  submitting.value = false
  if (mode.value === 'editService') {
    const svc = props.service
    form.value = { name: '', domain: svc?.domain ?? '', requireApproval: true }
    approvalChoice.value = ''
    healthCheck.value = svc?.healthCheck
      ? { ...svc.healthCheck }
      : { ...DEFAULT_HEALTH_CHECK }
    return
  }
  if (mode.value === 'editPools') {
    form.value = { name: '', domain: props.service?.domain ?? '', requireApproval: true }
    pools.value =
      props.initialPools && props.initialPools.length > 0
        ? props.initialPools.map((pool) => ({
            name: pool.name,
            priority: pool.priority,
            members: pool.members.map((member) => ({ clusterId: member.clusterId, weight: member.weight })),
          }))
        : [{ name: '', priority: 0, members: [{ clusterId: '', weight: 100 }] }]
    return
  }
  form.value = { name: '', domain: '', requireApproval: true }
  pools.value = [{ name: '', priority: 0, members: [{ clusterId: '', weight: 100 }] }]
  healthCheck.value = { ...DEFAULT_HEALTH_CHECK }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    reset()
    clustersError.value = null
    // 服务编辑模式不涉及池编排，无需加载集群列表。
    if (mode.value === 'editService') return
    try {
      const res = await listClusters({ pageSize: 100 })
      clusters.value = res.items ?? []
    } catch (err) {
      clusters.value = []
      clustersError.value = err instanceof Error ? err.message : String(err)
    }
  },
)

function addPool(): void {
  const maxPriority = pools.value.reduce((max, p) => Math.max(max, Number(p.priority) || 0), -1)
  pools.value.push({ name: '', priority: maxPriority + 1, members: [{ clusterId: '', weight: 100 }] })
}

function removePool(index: number): void {
  if (pools.value.length <= 1) return
  pools.value.splice(index, 1)
}

function addMember(poolIndex: number): void {
  pools.value[poolIndex]?.members.push({ clusterId: '', weight: 100 })
}

function removeMember(poolIndex: number, memberIndex: number): void {
  const pool = pools.value[poolIndex]
  if (!pool || pool.members.length <= 1) return
  pool.members.splice(memberIndex, 1)
}

function close(): void {
  if (submitting.value) return
  emit('update:modelValue', false)
}

function validatePools(): string | null {
  if (pools.value.length === 0) return 'resource.gslb.wizard.validation.poolRequired'
  const priorities = new Set<number>()
  for (const pool of pools.value) {
    if (!pool.name) return 'resource.gslb.wizard.validation.poolNameRequired'
    const priority = Number(pool.priority)
    if (!Number.isInteger(priority) || priority < 0) return 'resource.gslb.wizard.validation.priorityInvalid'
    if (priorities.has(priority)) return 'resource.gslb.wizard.validation.priorityDuplicate'
    priorities.add(priority)
    if (pool.members.length === 0) return 'resource.gslb.wizard.validation.memberRequired'
    for (const member of pool.members) {
      if (!member.clusterId) return 'resource.gslb.wizard.validation.clusterRequired'
      const weight = Number(member.weight)
      if (!Number.isInteger(weight) || weight < 0 || weight > 100) return 'resource.gslb.wizard.validation.weightRange'
    }
  }
  return null
}

function validateHealthCheck(): string | null {
  if (!healthCheck.value.path.startsWith('/')) return 'resource.gslb.wizard.validation.pathInvalid'
  if (!inRange(healthCheck.value.intervalSeconds, 5, 300)) return 'resource.gslb.wizard.validation.intervalRange'
  if (!inRange(healthCheck.value.timeoutSeconds, 1, 30)) return 'resource.gslb.wizard.validation.timeoutRange'
  if (!inRange(healthCheck.value.failureThreshold, 1, 10)) return 'resource.gslb.wizard.validation.thresholdRange'
  return null
}

/** 返回 i18n key；null 表示校验通过 */
function validate(): string | null {
  if (mode.value === 'editPools') return validatePools()
  if (mode.value === 'editService') {
    // 合并式 PATCH：名称留空表示不修改；填写时须符合后端命名规则。
    if (form.value.name && !NAME_RE.test(form.value.name)) return 'resource.gslb.wizard.validation.nameInvalid'
    return validateHealthCheck()
  }
  if (!form.value.name) return 'resource.gslb.wizard.validation.nameRequired'
  if (!form.value.domain) return 'resource.gslb.wizard.validation.domainRequired'
  if (!DOMAIN_RE.test(form.value.domain)) return 'resource.gslb.wizard.validation.domainInvalid'
  return validatePools() ?? validateHealthCheck()
}

function inRange(value: number, min: number, max: number): boolean {
  return Number.isInteger(Number(value)) && Number(value) >= min && Number(value) <= max
}

function healthCheckPayload() {
  return {
    protocol: healthCheck.value.protocol,
    path: healthCheck.value.path,
    intervalSeconds: Number(healthCheck.value.intervalSeconds),
    timeoutSeconds: Number(healthCheck.value.timeoutSeconds),
    failureThreshold: Number(healthCheck.value.failureThreshold),
  }
}

function poolsPayload() {
  return pools.value.map((pool) => ({
    name: pool.name,
    priority: Number(pool.priority),
    members: pool.members.map((member) => ({ clusterId: member.clusterId, weight: Number(member.weight) })),
  }))
}

async function submit(): Promise<void> {
  if (submitting.value) return
  const invalid = validate()
  if (invalid) {
    formError.value = t(invalid)
    return
  }
  submitting.value = true
  formError.value = null
  try {
    if (mode.value === 'editService') {
      if (!props.service) throw new Error('missing service for edit')
      // 合并式 PATCH：仅提交变更字段；健康检查始终提交（后端幂等，未变更则 200 不变更）。
      const patch: UpdateGslbServicePayload = { healthCheck: healthCheckPayload() }
      if (form.value.name) patch.name = form.value.name
      if (approvalChoice.value !== '') patch.requireApproval = approvalChoice.value === 'true'
      const detail = await updateGslbService(props.service.serviceId, patch, '*')
      emit('updated', detail)
    } else if (mode.value === 'editPools') {
      if (!props.service) throw new Error('missing service for pools edit')
      await replaceGslbPools(props.service.serviceId, poolsPayload(), '*')
      emit('poolsReplaced')
    } else {
      const service = await createGslbService({
        name: form.value.name,
        domain: form.value.domain,
        requireApproval: form.value.requireApproval,
        pools: poolsPayload(),
        healthCheck: healthCheckPayload(),
      })
      emit('created', service)
    }
    emit('update:modelValue', false)
  } catch (err) {
    // 409 等后端错误（If-Match 过期 / active 池被删 / FailingOver）直接展示后端 message。
    formError.value = err instanceof Error ? err.message : String(err)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.wizard-overlay {
  position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center;
  background: rgba(10, 14, 24, 0.6); padding: 24px;
}
.wizard-dialog {
  width: 640px; max-width: 100%; max-height: 90vh; overflow-y: auto;
  border: 1px solid var(--hnb-color-border, #29344a); border-radius: 10px;
  background: var(--hnb-color-bg-base, #10152a); color: var(--hnb-color-text-primary, #edeff5);
}
.wizard-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 18px; border-bottom: 1px solid var(--hnb-color-divider, #222b3d);
}
.wizard-head h3 { margin: 0; font-size: 15px; }
.wizard-close {
  border: none; background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2);
  font-size: 18px; cursor: pointer; line-height: 1;
}
.wizard-body { padding: 16px 18px; }
.wizard-section { margin-bottom: 18px; }
.wizard-section h4 { margin: 0 0 10px; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); }
.wizard-field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; font-size: 13px; }
.wizard-field > span { color: var(--hnb-color-text-tertiary, #8a94a3); font-size: 12px; }
.wizard-field-narrow { max-width: 140px; }
.wizard-input {
  padding: 6px 10px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5);
  font-size: 13px;
}
.wizard-input:focus { outline: none; border-color: var(--hnb-color-primary, #5b8dff); }
.wizard-input:disabled { opacity: 0.6; cursor: not-allowed; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.wizard-check { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.wizard-hint { margin: 4px 0 0; font-size: 12px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.wizard-pool {
  border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px;
  padding: 12px; margin-bottom: 10px; background: var(--hnb-color-bg-elevated, #171d31);
}
.wizard-pool-head { display: flex; align-items: flex-end; gap: 12px; flex-wrap: wrap; }
.wizard-pool-head .wizard-field { flex: 1; min-width: 160px; margin-bottom: 0; }
.wizard-member { display: flex; align-items: center; gap: 10px; margin-top: 10px; }
.wizard-member-cluster { flex: 1; }
.gslb-select {
  padding: 5px 8px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5);
  font-size: 13px;
}
.wizard-member-weight { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--hnb-color-text-tertiary, #8a94a3); }
.wizard-member-weight .wizard-input { width: 80px; }
.wizard-pool > .btn-small { margin-top: 10px; }
.wizard-hc-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 0 12px; }
.wizard-error {
  margin-bottom: 12px; padding: 8px 12px; border-radius: 6px; font-size: 13px;
  background: rgba(240, 68, 56, 0.1); color: var(--hnb-color-status-danger, #f04438);
}
.wizard-foot { display: flex; justify-content: flex-end; gap: 8px; }
.btn-small {
  padding: 6px 12px; border-radius: 6px; border: 1px solid var(--hnb-color-border, #29344a);
  background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5);
  font-size: 13px; cursor: pointer;
}
.btn-small:hover:not(:disabled) { border-color: var(--hnb-color-primary, #5b8dff); }
.btn-small.danger { color: var(--hnb-color-status-danger, #f04438); }
.btn-small:disabled { opacity: 0.5; cursor: not-allowed; }
.wizard-submit { border-color: var(--hnb-color-primary, #5b8dff); color: var(--hnb-color-primary, #5b8dff); }
</style>
