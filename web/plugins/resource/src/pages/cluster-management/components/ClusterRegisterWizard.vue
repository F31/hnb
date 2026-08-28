<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton } from '@hnb/ui-kit'
import * as api from '../api/clusterApi'
import { currentClusterScope, getClusterNavigate } from '../api/clusterApi'
import { createOperationPoller } from '../api/operationApi'
import { isTerminalStatus } from '../types/operation'
import type { OperationDetail } from '../types/operation'
import { getPluginMarketCatalog } from '../api/p4Api'
import { parseKubeSummary } from '../utils/kubeconfig'
import type { KubeSummary } from '../utils/kubeconfig'
import StaleChallengeDialog from './StaleChallengeDialog.vue'
import AgentOnboardingGuide from './AgentOnboardingGuide.vue'
import { useStaleSubmit } from '../composables/useStaleSubmit'
import type { MarketPlugin } from '../types/p4'
import type { RuntimeIntentRecord } from '../types/cluster'

const props = withDefaults(defineProps<{
  modelValue: boolean
}>(), {
  modelValue: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submitted: [record: RuntimeIntentRecord]
}>()

const { t } = useI18n()

// STALE 风险确认（创建/导入提交收到 409 challenge 时弹窗，确认后携带 riskConfirmation 重提）
const { staleChallenge, staleActionLabel, submit: submitWithStaleChallenge, resolveStaleConfirm } = useStaleSubmit()

type SourceMode = 'create' | 'import'

const step = ref(1)
const mode = ref<SourceMode>('create')
const sourceKind = ref<'kubernetes' | 'edge'>('kubernetes')
const form = ref({
  name: '',
  targetVersion: '1.36',
  replicas: 1,
  architecture: 'X86_64',
  description: '',
  plugins: [] as string[],
  controlPlaneSchedulingEnabled: false,
  containerNetwork: 'Kube-OVN',
  ipv6DualStack: false,
  managementVip: '',
  managementNicName: '',
  clusterVip: '',
  clusterNicName: '',
  podCidr: '',
  serviceCidr: '',
  joinCidr: '',
  customCertSANs: '',
  clusterDomain: '',
  alertNotifyChannel: 'email',
  alertNotifyInterval: 60,
  alertNotifyOnResolved: false,
  alertContacts: [] as string[],
  cloudcoreEndpoint: '',
  nodeGroup: '',
  secretRef: '',
  domainResolverIp: '',
  kubeConfig: '',
  kubeConfigSource: 'paste',
  kubeConfigFileName: '',
})
const submitError = ref('')
const submitting = ref(false)
const userOptions = ref<api.WizardUserOption[]>([])

// ---------------------------------------------------------------------------
// 提交前预检：解析 kubeconfig 提取接入目标（server / 集群名），仅展示非敏感信息。
// 参考 KubeSphere / Rancher 在接入确认步向用户披露“将接入哪个目标”，降低接错风险。
// ---------------------------------------------------------------------------
const kubeSummary = ref<KubeSummary>({ recognizable: false, structurallyValid: false, currentContext: '', clusters: [], errors: [] })

const isEdgeImport = computed(() => mode.value === 'import' && sourceKind.value === 'edge')

watch(
  () => form.value.kubeConfig,
  (text) => {
    kubeSummary.value = isEdgeImport.value
      ? { recognizable: false, structurallyValid: false, currentContext: '', clusters: [], errors: [] }
      : parseKubeSummary(text)
  },
  { immediate: true },
)

// ---------------------------------------------------------------------------
// 提交后内联 Operation 进度跟踪（闭环：提交 → 轮询至终态，参考 Rancher 的
// “集群注册中”实况与 KubeSphere 的创建进度）。复用共享 Operation 轮询客户端。
// ---------------------------------------------------------------------------
interface SubmissionTrack {
  record: RuntimeIntentRecord
}
const submission = ref<SubmissionTrack | null>(null)
const opDetail = ref<OperationDetail | null>(null)

function isOperationDetail(v: unknown): v is OperationDetail {
  return !!v && typeof v === 'object' && typeof (v as OperationDetail).status === 'string'
}

const opPoller = createOperationPoller({
  onUpdate: (next) => {
    if (isOperationDetail(next)) opDetail.value = next
  },
  onTerminal: (next) => {
    if (isOperationDetail(next)) opDetail.value = next
  },
  onError: () => {
    // 轮询失败由客户端退避重试，不打断用户；进度视图保持“等待”态。
  },
})

watch(
  () => opDetail.value?.status,
  (status) => {
    if (typeof status === 'string' && isTerminalStatus(status)) {
      opPoller.stop()
    }
  },
)

/**
 * 闭环最终环节：仅当「导入 Kubernetes 集群」且接入操作成功后，才展示
 * cluster-agent 接入指引（create 由平台侧控制面自动装机；edge 走 CloudCore）。
 * 目标集群 ID 取 Operation 的 targetId（导入操作落库的 RuntimeTarget ID）。
 */
const onboardingReady = computed(
  () =>
    submission.value !== null &&
    mode.value === 'import' &&
    !isEdgeImport.value &&
    opDetail.value?.status === 'succeeded' &&
    typeof opDetail.value?.targetId === 'string' &&
    opDetail.value.targetId.length > 0,
)
const onboardingClusterId = computed(() => opDetail.value?.targetId ?? '')

function goTrackOperation(operationId: string): void {
  getClusterNavigate()(`/resource/operations/${encodeURIComponent(operationId)}`)
}

/** 关闭内联进度并释放轮询（用户点“完成/关闭”时：关闭抽屉回到列表，列表已随 submitted 刷新） */
function endSubmission(): void {
  opPoller.cancel()
  submission.value = null
  opDetail.value = null
  emit('update:modelValue', false)
}

const versionOptions = ['1.36']
const replicaOptions = [1, 3, 5]
const architectureOptions = ['X86_64', 'AARCH64']
const containerNetworkOptions = ['Kube-OVN']

const marketPlugins = ref<MarketPlugin[]>([])

const dialogEl = ref<HTMLElement | null>(null)
let previousFocus: HTMLElement | null = null
let previousBodyOverflow = ''
let scrollLocked = false

const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function close(): void {
  if (submitting.value) return
  opPoller.cancel()
  emit('update:modelValue', false)
}

function focusDrawer(): void {
  const firstFocusable = dialogEl.value?.querySelector<HTMLElement>(focusableSelector)
  ;(firstFocusable || dialogEl.value)?.focus()
}

function restorePage(): void {
  if (scrollLocked) {
    document.body.style.overflow = previousBodyOverflow
    scrollLocked = false
  }
  if (previousFocus?.isConnected) previousFocus.focus()
  previousFocus = null
}

function onKeydown(event: KeyboardEvent): void {
  if (!props.modelValue) return
  const drawers = document.querySelectorAll<HTMLElement>('.cluster-register-drawer')
  if (drawers[drawers.length - 1] !== dialogEl.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
    return
  }
  if (event.key !== 'Tab' || !dialogEl.value) return
  const focusable = Array.from(dialogEl.value.querySelectorAll<HTMLElement>(focusableSelector))
  if (focusable.length === 0) {
    event.preventDefault()
    dialogEl.value.focus()
    return
  }
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && (document.activeElement === first || !dialogEl.value.contains(document.activeElement))) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

async function loadMarketPlugins(): Promise<void> {
  try {
    marketPlugins.value = await getPluginMarketCatalog()
  } catch {
    marketPlugins.value = []
  }
}

const pluginOptions = computed(() =>
  marketPlugins.value.map((p) => ({ value: p.name, label: p.name })),
)

const alertChannelOptions = computed(() => [
  { value: 'email', label: t('resource.clusterMgmt.form.alertChannel.email') },
  { value: 'sms', label: t('resource.clusterMgmt.form.alertChannel.sms') },
])

function reset(): void {
  step.value = 1
  mode.value = 'create'
  sourceKind.value = 'kubernetes'
  form.value = {
    name: '',
    targetVersion: '1.36',
    replicas: 1,
    architecture: 'X86_64',
    description: '',
    plugins: [],
    controlPlaneSchedulingEnabled: false,
    containerNetwork: 'Kube-OVN',
    ipv6DualStack: false,
    managementVip: '',
    managementNicName: '',
    clusterVip: '',
    clusterNicName: '',
    podCidr: '',
    serviceCidr: '',
    joinCidr: '',
    customCertSANs: '',
    clusterDomain: '',
    alertNotifyChannel: 'email',
    alertNotifyInterval: 60,
    alertNotifyOnResolved: false,
    alertContacts: [],
    cloudcoreEndpoint: '',
    nodeGroup: '',
    secretRef: '',
    domainResolverIp: '',
    kubeConfig: '',
    kubeConfigSource: 'paste',
    kubeConfigFileName: '',
  }
  submitError.value = ''
  submitting.value = false
  userOptions.value = []
  kubeSummary.value = { recognizable: false, structurallyValid: false, currentContext: '', clusters: [], errors: [] }
  opPoller.cancel()
  submission.value = null
  opDetail.value = null
}

async function loadUsers(): Promise<void> {
  try {
    userOptions.value = await api.listWizardUsers()
  } catch {
    userOptions.value = []
  }
}

watch(
  () => props.modelValue,
  async (visible) => {
    if (visible) {
      reset()
      loadUsers()
      loadMarketPlugins()
      previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
      previousBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      scrollLocked = true
      window.addEventListener('keydown', onKeydown)
      await nextTick()
      focusDrawer()
    } else {
      window.removeEventListener('keydown', onKeydown)
      restorePage()
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  opPoller.cancel()
  restorePage()
})

watch(
  () => form.value.kubeConfigSource,
  () => {
    form.value.kubeConfig = ''
    form.value.kubeConfigFileName = ''
  },
)

function nextStep(): void {
  if (step.value < 2) step.value += 1
}

function prevStep(): void {
  if (step.value > 1) step.value -= 1
}

const nameInvalid = computed(() => !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(form.value.name.trim()))

const stepOneValid = computed(() => {
  if (!form.value.name.trim() || nameInvalid.value) return false
  // 所有模式都需要凭据（k8s → kubeconfig；edge → cloudcore-client）。
  if (!form.value.kubeConfig.trim()) return false
  if (mode.value === 'create') {
    return form.value.targetVersion.trim().length > 0
  }
  if (isEdgeImport.value) {
    return form.value.cloudcoreEndpoint.trim().length > 0 && form.value.nodeGroup.trim().length > 0
  }
  return true
})

async function submit(): Promise<void> {
  if (!stepOneValid.value) return
  submitError.value = ''
  submitting.value = true
  try {
    const scope = currentClusterScope()
    const displayName = form.value.name.trim()
    // 凭据明文仅一次性上送，服务端加密落库；意图中只引用 SecretReference。
    const reg = await api.registerClusterSecret({
      purpose: isEdgeImport.value ? 'cloudcore-client' : 'kubeconfig',
      scope,
      name: `${displayName}-credential`,
      value: api.base64Encode(form.value.kubeConfig),
    })
    const credentialSecretRef = api.toSecretReference(reg)

    const createParams: Record<string, string | number | boolean | null> = {
      replicas: form.value.replicas,
      architecture: form.value.architecture.trim() || null,
      description: form.value.description.trim() || null,
      plugins: form.value.plugins.length ? form.value.plugins.join(',') : null,
      controlPlaneSchedulingEnabled: form.value.controlPlaneSchedulingEnabled,
      containerNetwork: form.value.containerNetwork.trim() || null,
      ipv6DualStack: form.value.ipv6DualStack,
      managementVip: form.value.managementVip.trim() || null,
      clusterVip: form.value.clusterVip.trim() || null,
      podCidr: form.value.podCidr.trim() || null,
      serviceCidr: form.value.serviceCidr.trim() || null,
    }
    const importParams: Record<string, string | number | boolean | null> = {
      description: form.value.description.trim() || null,
      clusterDomain: form.value.clusterDomain.trim() || null,
    }

    let envelope
    if (mode.value === 'create') {
      envelope = api.buildCreateIntent(displayName, credentialSecretRef, {
        kubernetesVersion: form.value.targetVersion.trim() || undefined,
        parameters: createParams,
      })
    } else if (isEdgeImport.value) {
      envelope = api.buildImportIntent('edge', displayName, credentialSecretRef, {
        cloudCoreEndpoint: form.value.cloudcoreEndpoint.trim(),
        parameters: importParams,
      })
    } else {
      envelope = api.buildImportIntent('kubernetes', displayName, credentialSecretRef, {
        parameters: importParams,
      })
    }

    const result = await submitWithStaleChallenge(envelope, sourceLabel(mode.value))
    if (result === 'cancelled') return
    // 进入内联进度视图（闭环）：提交成功即通知列表刷新；若返回 operationId 则开始
    // 轮询 Operation，把 REGISTERING→PROVISIONING→RUNNING 进度呈现给用户。
    submission.value = { record: result }
    emit('submitted', result)
    if (result.operationId) {
      opDetail.value = null
      opPoller.start(result.operationId)
    }
  } catch (err) {
    submitError.value = err instanceof Error ? err.message : String(err)
  } finally {
    submitting.value = false
  }
}

const kubeConfigFileInput = ref<HTMLInputElement | null>(null)

function pickKubeConfigFile(): void {
  kubeConfigFileInput.value?.click()
}

function onKubeConfigFile(e: Event): void {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    form.value.kubeConfig = String(reader.result ?? '')
    form.value.kubeConfigFileName = file.name
  }
  reader.readAsText(file)
  input.value = ''
}

function sourceLabel(m: SourceMode): string {
  return m === 'create'
    ? t('resource.clusterMgmt.wizard.createLabel')
    : t('resource.clusterMgmt.wizard.importLabel')
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="modelValue"
      class="cluster-register-drawer-layer"
      @click.self="close"
    >
      <section
        ref="dialogEl"
        class="cluster-register-drawer"
        role="dialog"
        aria-modal="true"
        aria-label="接入集群"
        tabindex="-1"
      >
        <header class="cluster-register-drawer__header">
          <h2 class="cluster-register-drawer__title">{{ t('resource.clusterMgmt.wizard.title') }}</h2>
          <button
            class="cluster-register-drawer__close"
            type="button"
            :disabled="submitting"
            aria-label="关闭"
            @click="close"
          >
            &times;
          </button>
        </header>

        <ol v-if="!submission" class="cluster-register-wizard-steps">
          <li
            v-for="(label, index) in [t('resource.clusterMgmt.wizard.stepSource'), t('resource.clusterMgmt.wizard.stepConfirm')]"
            :key="index"
            :class="{ active: step === index + 1, done: step > index + 1 }"
          >
            {{ label }}
          </li>
        </ol>

        <div class="cluster-register-drawer__body">
          <p v-if="submitError" class="cluster-register-submit-error" role="alert">{{ submitError }}</p>

          <!-- 提交后内联 Operation 进度（闭环：提交 → 轮询至终态） -->
          <section v-if="submission" class="cluster-register-step-progress">
            <div class="progress-head">
              <h3>{{ t('resource.clusterMgmt.operation.progressTitle') }}</h3>
              <p>{{ t('resource.clusterMgmt.operation.progressDesc', { name: form.name }) }}</p>
            </div>

            <template v-if="opDetail">
              <div class="progress-row">
                <span class="progress-label">{{ t('resource.clusterMgmt.operation.status') }}</span>
                <span class="progress-status" :data-status="opDetail.status">
                  {{ t(`resource.operationCenter.status.${opDetail.status}`) }}
                </span>
              </div>
              <div class="progress-row">
                <span class="progress-label">{{ t('resource.clusterMgmt.operation.progress') }}</span>
                <span class="progress-caption">{{ opDetail.progress.completedSteps }} / {{ opDetail.progress.totalSteps }}</span>
              </div>
              <div class="progress-track" role="progressbar" :aria-valuenow="opDetail.progress.percent" aria-valuemin="0" aria-valuemax="100">
                <div class="progress-fill" :style="{ width: opDetail.progress.percent + '%' }"></div>
              </div>
              <div v-if="opDetail.failure" class="progress-failure" role="alert">
                <strong>{{ opDetail.failure.code }}</strong>
                <span>{{ opDetail.failure.message }}</span>
              </div>
              <ul v-if="opDetail.steps && opDetail.steps.length" class="progress-steps">
                <li v-for="opStep in opDetail.steps" :key="opStep.stepId" class="progress-step" :data-status="opStep.status">
                  <span class="progress-step-dot"></span>
                  <span class="progress-step-name">{{ opStep.name }}</span>
                  <span class="progress-step-state">{{ t(`resource.operationCenter.step.status.${opStep.status}`) }}</span>
                </li>
              </ul>
            </template>
            <template v-else>
              <p class="progress-waiting" role="status">
                {{ submitting ? t('resource.clusterMgmt.common.submitting') : t('resource.clusterMgmt.operation.progressWaiting') }}
              </p>
            </template>

            <p class="operation-id" v-if="submission.record.operationId">
              {{ t('resource.clusterMgmt.operation.operationId') }}: {{ submission.record.operationId }}
            </p>

            <!-- 闭环最终环节：导入 Kubernetes 集群且操作成功后，提供 cluster-agent 接入指引 -->
            <div v-if="onboardingReady" class="agent-onboarding-wizard-block">
              <AgentOnboardingGuide
                :cluster-id="onboardingClusterId"
                :cluster-name="form.name"
              />
            </div>
          </section>

          <section v-else-if="step === 1" class="cluster-register-step1">
            <div class="source-toggle" role="tablist">
              <button
                v-for="m in (['create', 'import'] as SourceMode[])"
                :key="m"
                type="button"
                role="tab"
                :class="{ active: mode === m }"
                @click="mode = m"
              >
                {{ sourceLabel(m) }}
              </button>
            </div>

            <!-- 提交前预检：展示将接入的目标（kubeconfig 解析，非敏感；edge 凭据非 kubeconfig 跳过） -->
            <div v-if="!isEdgeImport" class="preflight-panel" role="note">
              <template v-if="kubeSummary.recognizable && kubeSummary.targetCluster">
                <span class="preflight-label">{{ t('resource.clusterMgmt.wizard.targetTitle') }}</span>
                <span class="preflight-value">{{ kubeSummary.targetCluster.name }}</span>
                <span class="preflight-url">{{ kubeSummary.targetCluster.server }}</span>
                <span v-if="kubeSummary.currentContext" class="preflight-ctx">
                  {{ t('resource.clusterMgmt.wizard.targetContext') }} {{ kubeSummary.currentContext }}
                </span>
                <span v-if="!kubeSummary.structurallyValid" class="preflight-warn">
                  {{ t('resource.clusterMgmt.wizard.targetIncomplete') }}
                </span>
              </template>
              <template v-else-if="form.kubeConfig.trim()">
                <span class="preflight-warn">{{ t('resource.clusterMgmt.wizard.targetNotRecognized') }}</span>
              </template>
              <template v-else>
                <span class="preflight-hint">{{ t('resource.clusterMgmt.wizard.targetHint') }}</span>
              </template>
            </div>

            <div class="form-grid">
              <label class="field-name">
                <span>{{ t('resource.clusterMgmt.form.name') }}</span>
                <input v-model="form.name" :placeholder="t('resource.clusterMgmt.form.namePlaceholder')" />
                <small v-if="nameInvalid" class="field-error">{{ t('resource.clusterMgmt.form.nameInvalid') }}</small>
              </label>

              <template v-if="mode === 'create'">
                <label>
                  <span>{{ t('resource.clusterMgmt.form.architecture') }}</span>
                  <select v-model="form.architecture">
                    <option v-for="a in architectureOptions" :key="a" :value="a">{{ a }}</option>
                  </select>
                </label>
                <label class="switch-field">
                  <input v-model="form.controlPlaneSchedulingEnabled" type="checkbox" />
                  <span>{{ t('resource.clusterMgmt.form.controlPlaneScheduling') }}</span>
                </label>

                <label class="field-full">
                  <span>{{ t('resource.clusterMgmt.form.description') }}</span>
                  <textarea v-model="form.description" rows="2" :placeholder="t('resource.clusterMgmt.form.descriptionPlaceholder')"></textarea>
                </label>

                <div class="field-full checkbox-field">
                  <span class="field-title">{{ t('resource.clusterMgmt.form.plugins') }}</span>
                  <div class="checkbox-group">
                    <label v-for="p in pluginOptions" :key="p.value">
                      <input v-model="form.plugins" type="checkbox" :value="p.value" />
                      <span>{{ p.label }}</span>
                    </label>
                    <span v-if="!pluginOptions.length" class="empty-hint">{{ t('resource.clusterMgmt.form.pluginsEmpty') }}</span>
                  </div>
                </div>

                <label class="switch-field">
                  <input v-model="form.ipv6DualStack" type="checkbox" />
                  <span>{{ t('resource.clusterMgmt.form.ipv6DualStack') }}</span>
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.containerNetwork') }}</span>
                  <select v-model="form.containerNetwork">
                    <option v-for="n in containerNetworkOptions" :key="n" :value="n">{{ n }}</option>
                  </select>
                </label>

                <label>
                  <span>{{ t('resource.clusterMgmt.form.managementVip') }}</span>
                  <input v-model="form.managementVip" :placeholder="t('resource.clusterMgmt.form.ipPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.managementNicName') }}</span>
                  <input v-model="form.managementNicName" :placeholder="t('resource.clusterMgmt.form.nicPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.clusterVip') }}</span>
                  <input v-model="form.clusterVip" :placeholder="t('resource.clusterMgmt.form.ipPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.clusterNicName') }}</span>
                  <input v-model="form.clusterNicName" :placeholder="t('resource.clusterMgmt.form.nicPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.podCidr') }}</span>
                  <input v-model="form.podCidr" :placeholder="t('resource.clusterMgmt.form.cidrPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.serviceCidr') }}</span>
                  <input v-model="form.serviceCidr" :placeholder="t('resource.clusterMgmt.form.cidrPlaceholder')" />
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.joinCidr') }}</span>
                  <input v-model="form.joinCidr" :placeholder="t('resource.clusterMgmt.form.cidrPlaceholder')" />
                </label>

                <label>
                  <span>{{ t('resource.clusterMgmt.form.targetVersion') }}</span>
                  <select v-model="form.targetVersion">
                    <option v-for="v in versionOptions" :key="v" :value="v">{{ v }}</option>
                  </select>
                </label>
                <label>
                  <span>{{ t('resource.clusterMgmt.form.replicas') }}</span>
                  <select v-model.number="form.replicas">
                    <option v-for="r in replicaOptions" :key="r" :value="r">{{ r }}</option>
                  </select>
                </label>

                <details class="field-full advanced">
                  <summary>{{ t('resource.clusterMgmt.form.advancedTitle') }}</summary>
                  <div class="advanced-body">
                    <label class="field-full">
                      <span>{{ t('resource.clusterMgmt.form.customCertSANs') }}</span>
                      <textarea v-model="form.customCertSANs" rows="3" :placeholder="t('resource.clusterMgmt.form.sanPlaceholder')"></textarea>
                    </label>
                    <label>
                      <span>{{ t('resource.clusterMgmt.form.clusterDomain') }}</span>
                      <input v-model="form.clusterDomain" :placeholder="t('resource.clusterMgmt.form.domainPlaceholder')" />
                    </label>

                    <div class="field-full">
                      <span class="field-title">{{ t('resource.clusterMgmt.form.alertConfig') }}</span>
                      <div class="alert-grid">
                        <label>
                          <span>{{ t('resource.clusterMgmt.form.alertChannelLabel') }}</span>
                          <select v-model="form.alertNotifyChannel">
                            <option v-for="c in alertChannelOptions" :key="c.value" :value="c.value">{{ c.label }}</option>
                          </select>
                        </label>
                        <label>
                          <span>{{ t('resource.clusterMgmt.form.alertInterval') }}</span>
                          <input v-model.number="form.alertNotifyInterval" type="number" min="1" />
                        </label>
                        <label class="switch-field">
                          <input v-model="form.alertNotifyOnResolved" type="checkbox" />
                          <span>{{ t('resource.clusterMgmt.form.alertOnResolved') }}</span>
                        </label>
                      </div>
                      <div class="checkbox-field">
                        <span class="field-title">{{ t('resource.clusterMgmt.form.alertContacts') }}</span>
                        <div class="checkbox-group">
                          <label v-for="u in userOptions" :key="u.id">
                            <input v-model="form.alertContacts" type="checkbox" :value="u.username" />
                            <span>{{ u.username }}</span>
                          </label>
                          <span v-if="!userOptions.length" class="empty-hint">{{ t('resource.clusterMgmt.form.contactsEmpty') }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </details>

                <label class="field-full">
                  <span>{{ t('resource.clusterMgmt.form.credentialTitle') }}</span>
                  <textarea
                    v-model="form.kubeConfig"
                    rows="8"
                    class="kubeconfig-input"
                    :placeholder="t('resource.clusterMgmt.form.createCredentialPlaceholder')"
                    spellcheck="false"
                  ></textarea>
                  <small class="field-hint">{{ t('resource.clusterMgmt.form.credentialHint') }}</small>
                </label>
              </template>

              <template v-else>
                <label class="field-source">
                  <span>{{ t('resource.clusterMgmt.form.sourceKind') }}</span>
                  <select v-model="sourceKind">
                    <option value="kubernetes">{{ t('resource.clusterMgmt.kind.kubernetes') }}</option>
                    <option value="edge">{{ t('resource.clusterMgmt.kind.edge') }}</option>
                  </select>
                </label>

                <template v-if="sourceKind === 'edge'">
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.cloudcoreEndpoint') }}</span>
                    <input v-model="form.cloudcoreEndpoint" :placeholder="'wss://cloudcore:10002'" />
                  </label>
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.nodeGroup') }}</span>
                    <input v-model="form.nodeGroup" :placeholder="t('resource.clusterMgmt.form.nodeGroupPlaceholder')" />
                  </label>
                  <label class="field-full">
                    <span>{{ t('resource.clusterMgmt.form.credentialTitle') }}</span>
                    <textarea
                      v-model="form.kubeConfig"
                      rows="6"
                      class="kubeconfig-input"
                      :placeholder="t('resource.clusterMgmt.form.edgeCredentialPlaceholder')"
                      spellcheck="false"
                    ></textarea>
                    <small class="field-hint">{{ t('resource.clusterMgmt.form.credentialHint') }}</small>
                  </label>
                </template>

                <template v-else>
                  <label class="field-full">
                    <span>{{ t('resource.clusterMgmt.form.description') }}</span>
                    <textarea v-model="form.description" rows="2" :placeholder="t('resource.clusterMgmt.form.descriptionPlaceholder')"></textarea>
                  </label>
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.clusterDomain') }}</span>
                    <input v-model="form.clusterDomain" :placeholder="t('resource.clusterMgmt.form.domainPlaceholder')" />
                  </label>
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.domainResolverIp') }}</span>
                    <input v-model="form.domainResolverIp" :placeholder="t('resource.clusterMgmt.form.ipPlaceholder')" />
                  </label>
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.managementVip') }}</span>
                    <input v-model="form.managementVip" :placeholder="t('resource.clusterMgmt.form.ipPlaceholder')" />
                  </label>
                  <label>
                    <span>{{ t('resource.clusterMgmt.form.clusterVip') }}</span>
                    <input v-model="form.clusterVip" :placeholder="t('resource.clusterMgmt.form.ipPlaceholder')" />
                  </label>
                  <div class="field-full kubeconfig-field">
                    <label class="field-title" for="kubeconfig-textarea">{{ t('resource.clusterMgmt.form.kubeConfig') }}</label>

                    <div class="source-radio" role="radiogroup" :aria-label="t('resource.clusterMgmt.form.kubeConfig')">
                      <label class="source-radio__item">
                        <input type="radio" value="paste" v-model="form.kubeConfigSource" />
                        <span>{{ t('resource.clusterMgmt.form.kubeConfigPaste') }}</span>
                      </label>
                      <label class="source-radio__item">
                        <input type="radio" value="file" v-model="form.kubeConfigSource" />
                        <span>{{ t('resource.clusterMgmt.form.kubeConfigFile') }}</span>
                      </label>
                    </div>

                    <template v-if="form.kubeConfigSource === 'paste'">
                      <textarea
                        id="kubeconfig-textarea"
                        v-model="form.kubeConfig"
                        rows="10"
                        class="kubeconfig-input"
                        :placeholder="t('resource.clusterMgmt.form.kubeConfigPlaceholder')"
                        spellcheck="false"
                      ></textarea>
                    </template>
                    <template v-else>
                      <input
                        ref="kubeConfigFileInput"
                        type="file"
                        accept=".yaml,.yml,.kubeconfig,.txt"
                        class="file-input"
                        @change="onKubeConfigFile"
                      />
                      <div class="upload-row">
                        <button class="secondary-button" type="button" @click="pickKubeConfigFile">
                          {{ t('resource.clusterMgmt.form.pickFile') }}
                        </button>
                        <span v-if="form.kubeConfigFileName" class="file-name">📄 {{ form.kubeConfigFileName }}</span>
                        <small class="field-hint">{{ t('resource.clusterMgmt.form.kubeConfigFileHint') }}</small>
                      </div>
                      <textarea
                        id="kubeconfig-textarea"
                        v-model="form.kubeConfig"
                        rows="10"
                        readonly
                        class="kubeconfig-input"
                        :placeholder="t('resource.clusterMgmt.form.kubeConfigPlaceholder')"
                        spellcheck="false"
                      ></textarea>
                    </template>
                  </div>
                </template>
              </template>
            </div>
          </section>

          <section v-else class="cluster-register-step2">
            <dl class="confirm-list">
              <dt>{{ t('resource.clusterMgmt.wizard.source') }}</dt>
              <dd>{{ sourceLabel(mode) }}</dd>
              <dt>{{ t('resource.clusterMgmt.form.name') }}</dt>
              <dd>{{ form.name }}</dd>
              <template v-if="!isEdgeImport && kubeSummary.recognizable && kubeSummary.targetCluster">
                <dt>{{ t('resource.clusterMgmt.wizard.targetTitle') }}</dt>
                <dd>
                  {{ kubeSummary.targetCluster.name }}
                  <span class="confirm-url">{{ kubeSummary.targetCluster.server }}</span>
                </dd>
              </template>
              <template v-if="mode === 'create'">
                <dt>{{ t('resource.clusterMgmt.form.architecture') }}</dt>
                <dd>{{ form.architecture }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.targetVersion') }}</dt>
                <dd>{{ form.targetVersion }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.replicas') }}</dt>
                <dd>{{ form.replicas }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.plugins') }}</dt>
                <dd>{{ form.plugins.join(', ') || '--' }}</dd>
              </template>
              <template v-else-if="sourceKind === 'edge'">
                <dt>{{ t('resource.clusterMgmt.form.cloudcoreEndpoint') }}</dt>
                <dd>{{ form.cloudcoreEndpoint }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.nodeGroup') }}</dt>
                <dd>{{ form.nodeGroup }}</dd>
              </template>
              <template v-else>
                <dt>{{ t('resource.clusterMgmt.form.clusterDomain') }}</dt>
                <dd>{{ form.clusterDomain || '--' }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.domainResolverIp') }}</dt>
                <dd>{{ form.domainResolverIp || '--' }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.managementVip') }}</dt>
                <dd>{{ form.managementVip || '--' }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.clusterVip') }}</dt>
                <dd>{{ form.clusterVip || '--' }}</dd>
                <dt>{{ t('resource.clusterMgmt.form.kubeConfig') }}</dt>
                <dd>{{ form.kubeConfig.trim() ? t('resource.clusterMgmt.form.kubeConfigProvided') : '--' }}</dd>
              </template>
            </dl>
            <p class="secret-hint">{{ t('resource.clusterMgmt.wizard.credentialsHint') }}</p>
          </section>
        </div>

        <footer class="cluster-register-drawer__footer">
          <HNBButton v-if="!submission" variant="secondary" :disabled="submitting" @click="close">
            {{ t('resource.clusterMgmt.common.cancel') }}
          </HNBButton>
          <div class="cluster-register-drawer__footer-right">
            <template v-if="submission">
              <HNBButton
                v-if="submission.record.operationId"
                variant="secondary"
                @click="goTrackOperation(submission.record.operationId)"
              >
                {{ t('resource.clusterMgmt.operation.track') }}
              </HNBButton>
              <HNBButton variant="primary" @click="endSubmission">
                {{ t('resource.clusterMgmt.operation.done') }}
              </HNBButton>
            </template>
            <template v-else>
              <HNBButton v-if="step > 1" variant="secondary" :disabled="submitting" @click="prevStep">
                {{ t('resource.clusterMgmt.common.back') }}
              </HNBButton>
              <HNBButton v-if="step < 2" variant="primary" :disabled="!stepOneValid || submitting" @click="nextStep">
                {{ t('resource.clusterMgmt.common.next') }}
              </HNBButton>
              <HNBButton v-else variant="primary" :loading="submitting" :disabled="submitting" @click="submit">
                {{ submitting ? t('resource.clusterMgmt.common.submitting') : t('resource.clusterMgmt.common.submit') }}
              </HNBButton>
            </template>
          </div>
        </footer>
      </section>
    </div>

    <!-- STALE 风险确认（高于抽屉层级） -->
    <StaleChallengeDialog
      :challenge="staleChallenge"
      :action-label="staleActionLabel"
      @confirm="resolveStaleConfirm(true)"
      @cancel="resolveStaleConfirm(false)"
    />
  </Teleport>
</template>

<style scoped>
.cluster-register-drawer-layer {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  justify-content: flex-end;
  background: rgba(18, 23, 42, 0.4);
}

.cluster-register-drawer {
  display: flex;
  flex-direction: column;
  width: 720px;
  max-width: 92vw;
  height: 100%;
  box-sizing: border-box;
  color: var(--hnb-color-text-primary);
  background: var(--hnb-color-bg-surface);
  border-left: 1px solid var(--hnb-color-border);
  box-shadow: var(--hnb-shadow-4);
  animation: cluster-register-drawer-in 0.22s ease-out;
}

.cluster-register-drawer__header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: var(--hnb-space-md);
  padding: var(--hnb-space-md) var(--hnb-space-lg);
  border-bottom: 1px solid var(--hnb-color-border);
}

.cluster-register-drawer__title {
  margin: 0;
  font-size: var(--hnb-font-size-lg);
  font-weight: var(--hnb-font-weight-semibold);
}

.cluster-register-drawer__close {
  width: 32px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: var(--hnb-radius-md);
  color: var(--hnb-color-text-secondary);
  background: transparent;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
}

.cluster-register-drawer__close:hover:not(:disabled) {
  color: var(--hnb-color-text-primary);
  background: var(--hnb-color-bg-elevated);
}

.cluster-register-drawer__close:disabled { cursor: not-allowed; opacity: 0.55; }
.cluster-register-drawer__close:focus-visible { outline: 2px solid var(--hnb-color-focus); outline-offset: 2px; }

.cluster-register-wizard-steps {
  display: flex;
  gap: var(--hnb-space-sm);
  list-style: none;
  padding: var(--hnb-space-sm) var(--hnb-space-lg) 0;
  margin: 0;
  flex: 0 0 auto;
}

.cluster-register-wizard-steps li {
  flex: 1;
  text-align: center;
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-tertiary);
  padding: var(--hnb-space-sm) 0;
  border-bottom: 2px solid var(--hnb-color-divider);
}

.cluster-register-wizard-steps li.active { color: var(--hnb-color-primary); border-color: var(--hnb-color-primary); }
.cluster-register-wizard-steps li.done { color: var(--hnb-color-status-success); border-color: var(--hnb-color-status-success); }

.cluster-register-drawer__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: var(--hnb-space-md) var(--hnb-space-lg);
  scrollbar-width: thin;
  scrollbar-color: var(--hnb-color-text-tertiary) transparent;
  scrollbar-gutter: stable;
}

.cluster-register-drawer__body::-webkit-scrollbar { width: 6px; }
.cluster-register-drawer__body::-webkit-scrollbar-thumb { background: var(--hnb-color-text-tertiary); border-radius: 3px; }
.cluster-register-drawer__body::-webkit-scrollbar-track { background: transparent; }

.cluster-register-submit-error {
  margin: 0 0 var(--hnb-space-md);
  color: var(--hnb-color-status-danger);
  font-size: var(--hnb-font-size-caption);
}

.cluster-register-drawer__footer {
  display: flex;
  flex: 0 0 auto;
  justify-content: space-between;
  gap: var(--hnb-space-sm);
  padding: var(--hnb-space-md) var(--hnb-space-lg);
  border-top: 1px solid var(--hnb-color-border);
  background: var(--hnb-color-bg-surface);
}

.cluster-register-drawer__footer-right {
  display: flex;
  gap: var(--hnb-space-sm);
}

.source-toggle {
  display: flex;
  gap: var(--hnb-space-sm);
  margin-bottom: var(--hnb-space-md);
}

.source-toggle button {
  flex: 1;
  padding: var(--hnb-space-sm);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-secondary);
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
}

.source-toggle button.active {
  border-color: var(--hnb-color-primary);
  color: var(--hnb-color-primary);
  background: color-mix(in srgb, var(--hnb-color-primary) 10%, transparent);
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--hnb-space-md);
  align-items: start;
}

.form-grid .field-name { grid-column: 1 / -1; }
.form-grid .field-full { grid-column: 1 / -1; }
.form-grid .field-source { grid-column: 1 / -1; }

.form-grid label,
.form-grid .checkbox-field {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-xs);
  font-size: var(--hnb-font-size-caption);
  color: var(--hnb-color-text-secondary);
}

.form-grid .switch-field {
  flex-direction: row;
  align-items: center;
  gap: var(--hnb-space-sm);
  align-self: end;
  padding-bottom: 8px;
}

.form-grid .switch-field input { width: 16px; height: 16px; accent-color: var(--hnb-color-primary); }

.form-grid input,
.form-grid select,
.form-grid textarea {
  padding: 8px 10px;
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-sm);
  background: var(--hnb-color-bg-elevated);
  color: var(--hnb-color-text-primary);
  font-size: var(--hnb-font-size-body);
  font-family: inherit;
}

.form-grid textarea { resize: vertical; }

.kubeconfig-input { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; line-height: 1.5; width: 100%; }

.upload-row { display: flex; align-items: center; gap: var(--hnb-space-sm); }
.file-input { position: absolute; width: 1px; height: 1px; opacity: 0; overflow: hidden; }
.file-name { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-secondary); }

.source-radio { display: flex; gap: var(--hnb-space-md); }
.source-radio__item { flex-direction: row !important; align-items: center; gap: 6px; cursor: pointer; }
.source-radio__item input { width: 14px; height: 14px; accent-color: var(--hnb-color-primary); }

.kubeconfig-field > .field-title { display: block; margin-bottom: 2px; }
.field-error { color: var(--hnb-color-status-danger); }
.field-hint { color: var(--hnb-color-text-tertiary); }
.field-title { font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-secondary); }

.checkbox-group { display: flex; flex-wrap: wrap; gap: 8px 16px; }
.checkbox-group label { flex-direction: row; align-items: center; gap: 6px; cursor: pointer; }
.checkbox-group input { width: 14px; height: 14px; accent-color: var(--hnb-color-primary); }
.empty-hint { color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); }

.advanced {
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  padding: 0;
}

.advanced summary {
  cursor: pointer;
  padding: 10px 12px;
  font-size: var(--hnb-font-size-body);
  color: var(--hnb-color-text-primary);
  font-weight: 600;
  user-select: none;
}

.advanced-body {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--hnb-space-md);
  padding: 0 12px 12px;
}

.advanced-body .field-full { grid-column: 1 / -1; }

.alert-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--hnb-space-md);
  margin-bottom: var(--hnb-space-sm);
}

.alert-grid label { flex-direction: column; align-items: flex-start; }

.confirm-list {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: var(--hnb-space-sm) var(--hnb-space-md);
  margin: 0;
}

.confirm-list dt { color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.confirm-list dd { margin: 0; word-break: break-all; }

.secret-hint { margin: var(--hnb-space-md) 0 0; font-size: var(--hnb-font-size-caption); color: var(--hnb-color-text-tertiary); }

.cluster-register-step2 { padding-top: var(--hnb-space-sm); }

/* 提交前预检（kubeconfig 接入目标披露） */
.confirm-url { display: block; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.preflight-panel {
  grid-column: 1 / -1;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 12px;
  padding: var(--hnb-space-sm) var(--hnb-space-md);
  margin-bottom: var(--hnb-space-sm);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-md);
  background: var(--hnb-color-bg-elevated);
  font-size: var(--hnb-font-size-caption);
}
.preflight-label { color: var(--hnb-color-text-secondary); }
.preflight-value { font-weight: var(--hnb-font-weight-semibold); color: var(--hnb-color-text-primary); }
.preflight-url { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--hnb-color-text-secondary); }
.preflight-ctx { color: var(--hnb-color-text-tertiary); }
.preflight-warn { color: var(--hnb-color-status-danger); }
.preflight-hint { color: var(--hnb-color-text-tertiary); }

/* 提交后内联 Operation 进度（闭环） */
.cluster-register-step-progress {
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
  padding-top: var(--hnb-space-sm);
}
.progress-head h3 { margin: 0; font-size: var(--hnb-font-size-lg); }
.progress-head p { margin: var(--hnb-space-xs) 0 0; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.progress-row { display: flex; align-items: center; gap: var(--hnb-space-md); }
.progress-label { width: 90px; color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.progress-status {
  display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: var(--hnb-font-size-caption);
  background: color-mix(in srgb, var(--hnb-color-text-tertiary) 16%, transparent); color: var(--hnb-color-text-secondary);
}
.progress-status[data-status='succeeded'] { background: color-mix(in srgb, var(--hnb-color-status-success) 14%, transparent); color: var(--hnb-color-status-success); }
.progress-status[data-status='failed'], .progress-status[data-status='cancelled'] { background: color-mix(in srgb, var(--hnb-color-status-danger) 14%, transparent); color: var(--hnb-color-status-danger); }
.progress-status[data-status='in_progress'], .progress-status[data-status='pending'], .progress-status[data-status='queued'], .progress-status[data-status='pending_approval'], .progress-status[data-status='queued_offline'] { background: color-mix(in srgb, var(--hnb-color-status-info) 14%, transparent); color: var(--hnb-color-status-info); }
.progress-caption { color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.progress-track { height: 10px; border-radius: 999px; background: var(--hnb-color-divider); overflow: hidden; }
.progress-fill { height: 100%; background: var(--hnb-color-primary); border-radius: 999px; transition: width 300ms ease; }
.progress-failure {
  display: flex; flex-direction: column; gap: var(--hnb-space-xs);
  padding: var(--hnb-space-sm) var(--hnb-space-md); border-radius: var(--hnb-radius-md);
  background: color-mix(in srgb, var(--hnb-color-status-danger) 10%, transparent); color: var(--hnb-color-status-danger); font-size: var(--hnb-font-size-body);
}
.progress-waiting { margin: 0; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-body); }
.operation-id { margin: 0; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); word-break: break-all; }
.agent-onboarding-wizard-block { margin-top: 12px; }
.progress-steps { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--hnb-space-xs); }
.progress-step {
  display: flex; align-items: center; gap: var(--hnb-space-sm);
  padding: var(--hnb-space-xs) var(--hnb-space-sm);
  border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm);
  font-size: var(--hnb-font-size-caption);
}
.progress-step-dot { width: 8px; height: 8px; border-radius: 999px; background: var(--hnb-color-text-tertiary); flex: 0 0 auto; }
.progress-step[data-status='succeeded'] .progress-step-dot { background: var(--hnb-color-status-success); }
.progress-step[data-status='in_progress'] .progress-step-dot { background: var(--hnb-color-status-info); }
.progress-step[data-status='failed'] .progress-step-dot { background: var(--hnb-color-status-danger); }
.progress-step-name { flex: 1; color: var(--hnb-color-text-primary); }
.progress-step-state { color: var(--hnb-color-text-tertiary); }

@keyframes cluster-register-drawer-in {
  from { transform: translateX(40px); opacity: 0.6; }
  to { transform: translateX(0); opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .cluster-register-drawer { animation: none; }
}
</style>