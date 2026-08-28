<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ApplicationDrawer from '../components/ApplicationDrawer.vue'

type AppMode = 'monolith' | 'microservice'
type WizardStep = 1 | 2 | 3

defineProps<{ mode: AppMode; appKindLabel: string }>()
const emit = defineEmits<{ close: []; submit: [] }>()
const { t } = useI18n()

const step = ref<WizardStep>(1)
const packageSelectorOpen = ref(false)
const selectedPackageName = ref('demo-jdk1.8')
const selectedPackageVersion = ref('v1-1782284889599')
const advancedHealthOpen = ref(false)
const showAdvanced = ref(false)

const packageNames = ['demo-jdk1.8', 'nginx', 'vsfdv', 'hello-jdk1.8', 'paas-jdk1.8']
const packageVersions = [
  { value: 'v1-1782284889599', arch: 'linux/amd64', time: '2026-06-24 15:08:36' },
  { value: 'v1-1776073488551', arch: 'linux/amd64', time: '2026-04-13 17:45:02' },
  { value: 'v1-1776073464977', arch: 'linux/amd64', time: '2026-04-13 17:44:39' },
  { value: 'v1-1776073439316', arch: 'linux/amd64', time: '2026-04-13 17:43:59' },
]
const packageTypes = ['image', 'jar', 'war', 'legacy', 'frontend'] as const
const repositoryTypes = ['private', 'public', 'thirdParty', 'upload'] as const
const resourceTypes = ['cluster', 'edge', 'vm'] as const
const workloadTypes = ['stateless', 'stateful', 'daemon'] as const
const checkTypes = ['off', 'command', 'httpGet', 'tcpSocket'] as const
const hookTypes = ['off', 'command', 'httpGet'] as const
const stepItems = computed(() => ['basic', 'runtime', 'confirm'].map((key) => t(`application.createWizard.steps.${key}`)))

const form = ref({
  name: '', version: '', description: '', project: 'CLOUD-HCI-Test',
  packageType: 'image', repositoryType: 'private', installPackage: '',
  deployMode: 'traditional', resourceType: 'cluster', cluster: 'default', fixedIp: false,
  workloadType: 'stateless', envName: '', envValue: '',
  requestCpu: 2, requestMemory: 4, limitCpu: 2, limitMemory: 4, gpuMode: 'off', replicas: 1,
  liveness: 'off', readiness: 'off', startup: 'off',
  postStart: 'off', preStop: 'off', terminationGrace: 30,
  updateStrategy: 'rolling', fillMode: 'percentage', maxSurge: 25, maxUnavailable: 25,
  serviceName: '', serviceType: 'external', ipv6: 'no', sessionAffinity: false,
  accessPath: '', command: '', args: '',
  privileged: false, hostNetwork: 'no', hostPid: 'no', hostIpc: 'no', readOnlyRoot: false, runAsUser: '',
  autoscaleCpu: false, autoscaleMemory: false, diagnostics: false, logEnabled: false,
  defaultGateway: true, fixedNetworkIp: false, qos: false,
})

function next() { if (step.value < 3) step.value = (step.value + 1) as WizardStep }
function prev() { if (step.value > 1) step.value = (step.value - 1) as WizardStep }
function confirmPackage() { form.value.installPackage = `${selectedPackageName.value}:${selectedPackageVersion.value}`; packageSelectorOpen.value = false }
function submit() { emit('submit') }
function onDrawerVisibilityChange(open: boolean) { if (!open) emit('close') }
</script>

<template>
  <ApplicationDrawer
    :model-value="true"
    :title="t('application.createWizard.title')"
    :width="960"
    hide-confirm
    @update:model-value="onDrawerVisibilityChange"
  >
      <div class="breadcrumb">{{ t('application.createWizard.breadcrumbList') }} / {{ t('application.createWizard.title') }}</div>

      <ol class="stepper">
        <li v-for="(label, index) in stepItems" :key="label" :class="{ active: step === index + 1, done: step > index + 1 }">
          <span class="step-num">{{ step > index + 1 ? '✓' : index + 1 }}</span>
          <strong>{{ label }}</strong>
        </li>
      </ol>

      <main class="wizard-content">
        <section v-if="step === 1" class="wizard-section">
          <div class="section-title">{{ t('application.createWizard.basic.title') }}</div>
          <div class="form-card form-grid two-columns">
            <label class="required">
              <span>{{ t('application.createWizard.basic.name') }}</span>
              <input v-model="form.name" :placeholder="t('application.createWizard.basic.namePlaceholder')" />
            </label>
            <label class="required">
              <span>{{ t('application.createWizard.basic.version') }}</span>
              <input v-model="form.version" :placeholder="t('application.createWizard.basic.versionPlaceholder')" />
            </label>
            <label class="full">
              <span>{{ t('application.createWizard.basic.description') }}</span>
              <textarea v-model="form.description" rows="4" />
            </label>
            <label class="full">
              <span>{{ t('application.createWizard.basic.tenant') }}</span>
              <select v-model="form.project"><option>CLOUD-HCI-Test</option></select>
            </label>

            <div class="full field-block required">
              <span>{{ t('application.createWizard.basic.packageType') }}</span>
              <div class="choice-grid five">
                <button v-for="item in packageTypes" :key="item" type="button" class="choice-card" :class="['pkg-' + item, { selected: form.packageType === item }]" @click="form.packageType = item">
                  <span class="pkg-icon">
                    <svg v-if="item === 'image'" width="24" height="24" viewBox="0 0 24 24" fill="none"><rect x="2" y="2" width="20" height="20" rx="4" stroke="#f97316" stroke-width="1.5"/><circle cx="9" cy="9" r="2" stroke="#f97316" stroke-width="1.5"/><path d="M2 16l5-4 4 3 4-5 7 7" stroke="#f97316" stroke-width="1.5" stroke-linejoin="round"/></svg>
                    <svg v-else-if="item === 'jar'" width="24" height="24" viewBox="0 0 24 24" fill="none"><path d="M6 4h12l2 4v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8l2-4z" stroke="#22c55e" stroke-width="1.5"/><path d="M8 2v2M16 2v2" stroke="#22c55e" stroke-width="1.5"/><text x="12" y="16" text-anchor="middle" fill="#22c55e" font-size="8" font-weight="bold">JAR</text></svg>
                    <svg v-else-if="item === 'war'" width="24" height="24" viewBox="0 0 24 24" fill="none"><path d="M6 4h12l2 4v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8l2-4z" stroke="#06b6d4" stroke-width="1.5"/><path d="M8 2v2M16 2v2" stroke="#06b6d4" stroke-width="1.5"/><text x="12" y="16" text-anchor="middle" fill="#06b6d4" font-size="8" font-weight="bold">WAR</text></svg>
                    <svg v-else-if="item === 'legacy'" width="24" height="24" viewBox="0 0 24 24" fill="none"><path d="M3 7h18M3 7l2 13h14l2-13M3 7l3-4h12l3 4" stroke="#3b82f6" stroke-width="1.5" stroke-linejoin="round"/><path d="M9 12v5M15 12v5" stroke="#3b82f6" stroke-width="1.5"/></svg>
                    <svg v-else-if="item === 'frontend'" width="24" height="24" viewBox="0 0 24 24" fill="none"><rect x="3" y="3" width="18" height="18" rx="3" stroke="#a855f7" stroke-width="1.5"/><path d="M3 9h18M9 21V9" stroke="#a855f7" stroke-width="1.5"/></svg>
                  </span>
                  <strong>{{ t(`application.createWizard.packageTypes.${item}.title`) }}</strong>
                </button>
              </div>
            </div>

            <div class="full field-block required">
              <span>{{ t('application.createWizard.basic.installPackage') }}</span>
              <div class="segmented">
                <button v-for="item in repositoryTypes" :key="item" type="button" :disabled="item === 'thirdParty'" :class="{ active: form.repositoryType === item }" @click="form.repositoryType = item">{{ t(`application.createWizard.repositories.${item}`) }}</button>
              </div>
              <div class="package-picker">
                <input :value="form.installPackage" disabled :placeholder="t('application.createWizard.basic.packagePlaceholder')" />
                <button type="button" @click="packageSelectorOpen = true">{{ t('application.createWizard.basic.choosePackage') }}</button>
              </div>
            </div>

            <div class="full field-block required">
              <span>{{ t('application.createWizard.basic.resourceType') }}</span>
              <div class="choice-grid three">
                <button v-for="item in resourceTypes" :key="item" type="button" class="resource-card" :disabled="item === 'vm'" :class="[`resource-${item}`, { selected: form.resourceType === item }]" @click="form.resourceType = item">
                  <span class="resource-icon">
                    <svg v-if="item === 'cluster'" width="28" height="28" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="5" r="2.5" stroke="#637bff" stroke-width="1.5"/><circle cx="5" cy="19" r="2.5" stroke="#637bff" stroke-width="1.5"/><circle cx="19" cy="19" r="2.5" stroke="#637bff" stroke-width="1.5"/><path d="M12 7.5v4M7 17l2-3M17 17l-2-3" stroke="#637bff" stroke-width="1.5"/></svg>
                    <svg v-else-if="item === 'edge'" width="28" height="28" viewBox="0 0 24 24" fill="none"><path d="M12 2L2 7l10 5 10-5-10-5z" stroke="#22c55e" stroke-width="1.5" stroke-linejoin="round"/><path d="M2 17l10 5 10-5M2 12l10 5 10-5" stroke="#22c55e" stroke-width="1.5" stroke-linejoin="round"/></svg>
                    <svg v-else-if="item === 'vm'" width="28" height="28" viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="14" rx="2" stroke="#6f7886" stroke-width="1.5"/><path d="M8 22h8M12 18v4" stroke="#6f7886" stroke-width="1.5"/></svg>
                  </span>
                  <strong>{{ t(`application.createWizard.resources.${item}.title`) }}</strong>
                  <small>{{ t(`application.createWizard.resources.${item}.desc`) }}</small>
                </button>
              </div>
            </div>

            <label class="full required">
              <span>{{ t('application.createWizard.basic.cluster') }}</span>
              <div class="inline-control">
                <select v-model="form.cluster"><option>default</option></select>
                <button class="icon-btn" type="button">↻</button>
                <a href="#">{{ t('application.createWizard.basic.createCluster') }}</a>
              </div>
            </label>

            <label class="switch-line">
              <span>{{ t('application.createWizard.basic.fixedIp') }}</span>
              <div class="toggle-group">
                <span :class="{ active: !form.fixedIp }">{{ t('application.common.no') }}</span>
                <input v-model="form.fixedIp" type="checkbox" />
                <span :class="{ active: form.fixedIp }">{{ t('application.common.yes') }}</span>
              </div>
            </label>
          </div>
        </section>

        <section v-else-if="step === 2" class="wizard-section">
          <div class="section-title">{{ t('application.createWizard.runtime.title') }}</div>

          <!-- Basic Attributes -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.basicAttrs') }}</h3>
            <div class="card-body">
              <div class="segmented">
                <button v-for="item in workloadTypes" :key="item" type="button" :class="{ active: form.workloadType === item }" @click="form.workloadType = item">{{ t(`application.createWizard.workloads.${item}`) }}</button>
              </div>
              <div class="env-section">
                <span class="section-label">{{ t('application.createWizard.runtime.envVars') }}</span>
                <p class="field-hint">{{ t('application.createWizard.runtime.envHint') }}</p>
                <div class="table-toolbar">
                  <button class="link-button" type="button">{{ t('application.common.add') }}</button>
                  <button class="link-button" type="button">{{ t('application.createWizard.runtime.quickAdd') }}</button>
                </div>
                <div class="env-table">
                  <div class="env-head"><span>{{ t('application.createWizard.runtime.envConfigMode') }}</span><span>{{ t('application.createWizard.runtime.envName') }}</span><span>{{ t('application.createWizard.runtime.envValue') }}</span><span>{{ t('application.common.operations') }}</span></div>
                  <div class="env-row"><select><option>{{ t('application.createWizard.runtime.manualInput') }}</option></select><input /><input /><button class="text-button">{{ t('application.marketPage.actions.delete') }}</button></div>
                </div>
              </div>
              <div class="quota-section">
                <span class="section-label">{{ t('application.createWizard.runtime.containerSpecs') }}</span>
                <p class="field-hint">{{ t('application.createWizard.runtime.quotaHint', { cpu: 4, memory: 8192 }) }}</p>
                <div class="quota-grid">
                  <fieldset class="quota-group">
                    <legend>{{ t('application.createWizard.runtime.requestLimit') }}</legend>
                    <p class="field-hint">{{ t('application.createWizard.runtime.requestHint') }}</p>
                    <label><span>{{ t('application.createWizard.runtime.requestCpu') }}</span><input v-model.number="form.requestCpu" type="number" min="0.1" step="0.1" />{{ t('application.createWizard.runtime.cores') }}</label>
                    <label><span>{{ t('application.createWizard.runtime.requestMemory') }}</span><input v-model.number="form.requestMemory" type="number" min="0.1" step="0.1" /><select><option>GB</option><option>MB</option></select></label>
                  </fieldset>
                  <fieldset class="quota-group">
                    <legend>{{ t('application.createWizard.runtime.limitLimit') }}</legend>
                    <p class="field-hint">{{ t('application.createWizard.runtime.limitHint') }}</p>
                    <label><span>{{ t('application.createWizard.runtime.limitCpu') }}</span><input v-model.number="form.limitCpu" type="number" min="0.1" step="0.1" />{{ t('application.createWizard.runtime.cores') }}<small class="quota-note">{{ t('application.createWizard.runtime.availableCpu', { quota: 999968 }) }}</small></label>
                    <label><span>{{ t('application.createWizard.runtime.limitMemory') }}</span><input v-model.number="form.limitMemory" type="number" min="0.1" step="0.1" /><select><option>GB</option><option>MB</option></select><small class="quota-note">{{ t('application.createWizard.runtime.availableMemory', { quota: 999968 }) }}</small></label>
                  </fieldset>
                </div>
                <p class="field-hint tiny">{{ t('application.createWizard.runtime.quotaNote') }}</p>
              </div>
              <div class="gpu-section">
                <span class="section-label">{{ t('application.createWizard.runtime.gpu') }}</span>
                <div class="segmented compact">
                  <button v-for="item in ['off','exclusive','hami']" :key="item" type="button" :class="{ active: form.gpuMode === item }" @click="form.gpuMode = item">{{ t(`application.createWizard.gpu.${item}`) }}</button>
                </div>
              </div>
              <div class="config-section">
                <span class="section-label">{{ t('application.createWizard.runtime.configFiles') }}</span>
                <div class="table-toolbar"><button class="link-button" type="button">{{ t('application.common.add') }}</button></div>
                <div class="env-table"><div class="env-head"><span>{{ t('application.createWizard.runtime.configFile') }}</span><span>{{ t('application.createWizard.runtime.configPath') }}</span><span>{{ t('application.common.operations') }}</span></div></div>
              </div>
              <div class="label-section">
                <span class="section-label">{{ t('application.createWizard.runtime.controllerLabels') }}</span>
                <div class="table-toolbar"><button class="link-button" type="button">{{ t('application.common.add') }}</button></div>
              </div>
              <div class="label-section">
                <span class="section-label">{{ t('application.createWizard.runtime.controllerAnnotations') }}</span>
                <div class="table-toolbar"><button class="link-button" type="button">{{ t('application.common.add') }}</button></div>
              </div>
              <label class="field-row">
                <span>{{ t('application.createWizard.runtime.replicas') }}</span>
                <input v-model.number="form.replicas" type="number" min="1" />
              </label>
            </div>
          </div>

          <!-- Health Check -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.health') }}</h3>
            <div class="card-body">
              <p class="field-hint">{{ t('application.createWizard.runtime.healthDesc') }}</p>
              <div v-for="key in ['liveness','readiness','startup']" :key="key" class="check-row">
                <b>{{ t(`application.createWizard.runtime.${key}`) }}</b>
                <div class="segmented compact">
                  <button v-for="item in checkTypes" :key="item" type="button" :class="{ active: form[key as 'liveness'] === item }" @click="form[key as 'liveness'] = item">{{ item }}</button>
                </div>
              </div>
              <button class="link-button" type="button" @click="showAdvanced = !showAdvanced">
                {{ showAdvanced ? t('application.createWizard.runtime.hideAdvanced') : t('application.createWizard.runtime.showAdvanced') }}
              </button>
            </div>
          </div>

          <template v-if="showAdvanced">

          <!-- Lifecycle -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.lifecycle') }}</h3>
            <div class="card-body">
              <p class="field-hint">{{ t('application.createWizard.runtime.lifecycleDesc') }}</p>
              <div v-for="key in ['postStart','preStop']" :key="key" class="check-row">
                <b>{{ t(`application.createWizard.runtime.${key}`) }}</b>
                <div class="segmented compact">
                  <button v-for="item in hookTypes" :key="item" type="button" :class="{ active: form[key as 'postStart'] === item }" @click="form[key as 'postStart'] = item">{{ item }}</button>
                </div>
              </div>
              <label class="field-row">
                <span>{{ t('application.createWizard.runtime.terminationGrace') }}</span>
                <input v-model.number="form.terminationGrace" type="number" min="0" />
                <span class="unit">s</span>
              </label>
            </div>
          </div>

          <!-- Upgrade Strategy -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.updateStrategy') }}</h3>
            <div class="card-body">
              <p class="field-hint">{{ t('application.createWizard.runtime.updateHint', { cpu: 4, memory: 8192 }) }}</p>
              <div class="segmented">
                <button :class="{ active: form.updateStrategy === 'rolling' }" @click="form.updateStrategy = 'rolling'">{{ t('application.createWizard.runtime.rollingUpdate') }}</button>
                <button :class="{ active: form.updateStrategy === 'recreate' }" @click="form.updateStrategy = 'recreate'">{{ t('application.createWizard.runtime.recreate') }}</button>
              </div>
              <div class="segmented compact">
                <button :class="{ active: form.fillMode === 'count' }" @click="form.fillMode = 'count'">{{ t('application.createWizard.runtime.fillCount') }}</button>
                <button :class="{ active: form.fillMode === 'percentage' }" @click="form.fillMode = 'percentage'">{{ t('application.createWizard.runtime.fillPercentage') }}</button>
              </div>
              <div class="quota-grid">
                <label><span>{{ t('application.createWizard.runtime.maxSurge') }}</span><input v-model.number="form.maxSurge" type="number" min="0" />%</label>
                <label><span>{{ t('application.createWizard.runtime.maxUnavailable') }}</span><input v-model.number="form.maxUnavailable" type="number" min="0" />%</label>
              </div>
            </div>
          </div>

          <!-- Service Access -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.serviceAccess') }}</h3>
            <div class="card-body">
              <label class="field-row">
                <span>{{ t('application.createWizard.runtime.serviceName') }}</span>
                <input v-model="form.serviceName" />
              </label>
              <div class="segmented">
                <button :class="{ active: form.serviceType === 'external' }" @click="form.serviceType = 'external'">{{ t('application.createWizard.runtime.externalAccess') }}</button>
                <button :class="{ active: form.serviceType === 'internal' }" @click="form.serviceType = 'internal'">{{ t('application.createWizard.runtime.internalAccess') }}</button>
              </div>
              <div class="segmented compact">
                <button :class="{ active: form.ipv6 === 'yes' }" @click="form.ipv6 = 'yes'">{{ t('application.common.yes') }}</button>
                <button :class="{ active: form.ipv6 === 'no' }" @click="form.ipv6 = 'no'">{{ t('application.common.no') }}</button>
              </div>
              <div class="table-toolbar"><button class="link-button" type="button">{{ t('application.common.add') }}</button></div>
              <label class="switch-line">
                <span>{{ t('application.createWizard.runtime.sessionAffinity') }}</span>
                <div class="toggle-group">
                  <span :class="{ active: !form.sessionAffinity }">{{ t('application.common.no') }}</span>
                  <input v-model="form.sessionAffinity" type="checkbox" />
                  <span :class="{ active: form.sessionAffinity }">{{ t('application.common.yes') }}</span>
                </div>
                <small class="field-hint">{{ t('application.createWizard.runtime.sessionHint') }}</small>
              </label>
            </div>
          </div>

          <!-- Access Path -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.accessPath') }}</h3>
            <div class="card-body">
              <p class="field-hint">{{ t('application.createWizard.runtime.accessPathHint') }}</p>
              <input v-model="form.accessPath" placeholder="/" />
            </div>
          </div>

          <!-- Startup Command -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.command') }}</h3>
            <div class="card-body">
              <textarea v-model="form.command" rows="3" :placeholder="t('application.createWizard.runtime.commandPlaceholder')" />
              <textarea v-model="form.args" rows="3" :placeholder="t('application.createWizard.runtime.argsPlaceholder')" />
            </div>
          </div>

          <!-- Container Permissions -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.security') }}</h3>
            <div class="card-body">
              <div class="switch-grid">
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.privileged') }}</span><div class="toggle-group"><span :class="{ active: !form.privileged }">{{ t('application.common.no') }}</span><input v-model="form.privileged" type="checkbox" /><span :class="{ active: form.privileged }">{{ t('application.common.yes') }}</span></div><small class="field-hint">{{ t('application.createWizard.runtime.privilegedHint') }}</small></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.hostNetwork') }}</span><div class="toggle-group"><span :class="{ active: form.hostNetwork === 'no' }">{{ t('application.common.no') }}</span><input v-model="form.hostNetwork" type="checkbox" true-value="yes" false-value="no" /><span :class="{ active: form.hostNetwork === 'yes' }">{{ t('application.common.yes') }}</span></div></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.hostPid') }}</span><div class="toggle-group"><span :class="{ active: form.hostPid === 'no' }">{{ t('application.common.no') }}</span><input v-model="form.hostPid" type="checkbox" true-value="yes" false-value="no" /><span :class="{ active: form.hostPid === 'yes' }">{{ t('application.common.yes') }}</span></div></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.hostIpc') }}</span><div class="toggle-group"><span :class="{ active: form.hostIpc === 'no' }">{{ t('application.common.no') }}</span><input v-model="form.hostIpc" type="checkbox" true-value="yes" false-value="no" /><span :class="{ active: form.hostIpc === 'yes' }">{{ t('application.common.yes') }}</span></div></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.readOnlyRoot') }}</span><div class="toggle-group"><span :class="{ active: !form.readOnlyRoot }">{{ t('application.common.no') }}</span><input v-model="form.readOnlyRoot" type="checkbox" /><span :class="{ active: form.readOnlyRoot }">{{ t('application.common.yes') }}</span></div></label>
              </div>
              <label class="field-row"><span>{{ t('application.createWizard.runtime.runAsUser') }}</span><select><option>{{ t('application.createWizard.runtime.manualInput') }}</option></select><input v-model="form.runAsUser" placeholder="1000" /></label>
              <label class="field-row"><span>{{ t('application.createWizard.runtime.linuxCapabilities') }}</span><select><option>{{ t('application.createWizard.runtime.capPlaceholder') }}</option></select></label>
            </div>
          </div>

          <!-- Auto Scaling -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.autoScaling') }}</h3>
            <div class="card-body">
              <label class="switch-line"><input v-model="form.autoscaleCpu" type="checkbox" />{{ t('application.createWizard.runtime.autoscaleCpu') }}<input v-if="form.autoscaleCpu" type="number" min="0" max="100" />%<small class="field-hint">{{ t('application.createWizard.runtime.autoscaleCpuHint') }}</small></label>
              <label class="switch-line"><input v-model="form.autoscaleMemory" type="checkbox" />{{ t('application.createWizard.runtime.autoscaleMemory') }}<input v-if="form.autoscaleMemory" type="number" min="0" max="100" />%<small class="field-hint">{{ t('application.createWizard.runtime.autoscaleMemoryHint') }}</small></label>
            </div>
          </div>

          <!-- Diagnostics & Logging -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.diagnostics') }}</h3>
            <div class="card-body">
              <label class="switch-line"><span>{{ t('application.createWizard.runtime.enableDiagnostics') }}</span><div class="toggle-group"><span :class="{ active: !form.diagnostics }">{{ t('application.common.no') }}</span><input v-model="form.diagnostics" type="checkbox" /><span :class="{ active: form.diagnostics }">{{ t('application.common.yes') }}</span></div></label>
            </div>
          </div>
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.logging') }}</h3>
            <div class="card-body">
              <label class="switch-line"><span>{{ t('application.createWizard.runtime.enableLogging') }}</span><div class="toggle-group"><span :class="{ active: !form.logEnabled }">{{ t('application.common.no') }}</span><input v-model="form.logEnabled" type="checkbox" /><span :class="{ active: form.logEnabled }">{{ t('application.common.yes') }}</span></div></label>
            </div>
          </div>

          </template>

          <!-- Network Configuration -->
          <div class="runtime-card">
            <h3>{{ t('application.createWizard.runtime.network') }}</h3>
            <div class="card-body">
              <div class="mini-table">
                <b>{{ t('application.createWizard.runtime.networkInterface') }}</b>
                <button class="link-button" type="button">{{ t('application.common.add') }}</button>
              </div>
              <div class="env-table">
                <div class="env-head"><span>{{ t('application.createWizard.runtime.networkContainer') }}</span><span>{{ t('application.createWizard.runtime.networkType') }}</span><span>{{ t('application.createWizard.runtime.networkName') }}</span><span>{{ t('application.createWizard.runtime.networkSubnet') }}</span></div>
                <div class="env-row"><select><option>{{ t('application.createWizard.runtime.vpc') }}</option></select><span>ovn-cluster-default (11.0.0.0/10)</span><span>ovn-default-default (11.0.0.0/10)</span></div>
              </div>
              <p class="field-hint">IPv4: 4194768 / IPv6: 0</p>
              <div class="switch-grid">
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.defaultGateway') }}</span><div class="toggle-group"><span :class="{ active: !form.defaultGateway }">{{ t('application.common.off') }}</span><input v-model="form.defaultGateway" type="checkbox" /><span :class="{ active: form.defaultGateway }">{{ t('application.common.on') }}</span></div></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.elasticIp') }}</span><div class="toggle-group"><span :class="{ active: !form.fixedNetworkIp }">{{ t('application.common.off') }}</span><input v-model="form.fixedNetworkIp" type="checkbox" /><span :class="{ active: form.fixedNetworkIp }">{{ t('application.common.on') }}</span></div></label>
                <label class="switch-line"><span>{{ t('application.createWizard.runtime.qos') }}</span><div class="toggle-group"><span :class="{ active: !form.qos }">{{ t('application.common.off') }}</span><input v-model="form.qos" type="checkbox" /><span :class="{ active: form.qos }">{{ t('application.common.on') }}</span></div></label>
              </div>
              <button class="link-button" type="button" @click="showAdvanced = !showAdvanced">
                {{ showAdvanced ? t('application.createWizard.runtime.hideAdvanced') : t('application.createWizard.runtime.showAdvanced') }}
              </button>
            </div>
          </div>
        </section>

        <section v-else class="wizard-section">
          <div class="section-title">{{ t('application.createWizard.confirm.title') }}</div>
          <div class="confirm-grid">
            <div><span>{{ t('application.createWizard.confirm.kind') }}</span><strong>{{ appKindLabel }}</strong></div>
            <div><span>{{ t('application.createWizard.basic.name') }}</span><strong>{{ form.name || '-' }}</strong></div>
            <div><span>{{ t('application.createWizard.basic.version') }}</span><strong>{{ form.version || '-' }}</strong></div>
            <div><span>{{ t('application.createWizard.basic.installPackage') }}</span><strong>{{ form.installPackage || '-' }}</strong></div>
            <div><span>{{ t('application.createWizard.basic.resourceType') }}</span><strong>{{ t(`application.createWizard.resources.${form.resourceType}.title`) }}</strong></div>
            <div><span>{{ t('application.createWizard.runtime.replicas') }}</span><strong>{{ form.replicas }}</strong></div>
          </div>
        </section>
      </main>

      <template #footer>
        <button class="secondary-button" type="button" @click="emit('close')">{{ t('application.common.cancel') }}</button>
        <button v-if="step > 1" class="secondary-button" type="button" @click="prev">{{ t('application.common.prev') }}</button>
        <button v-if="step < 3" class="primary-button" type="button" @click="next">{{ t('application.common.next') }}</button>
        <button v-else class="primary-button" type="button" @click="submit">{{ t('application.createWizard.confirm.deploy') }}</button>
      </template>
  </ApplicationDrawer>

  <ApplicationDrawer
    v-model="packageSelectorOpen"
    :title="t('application.createWizard.packageModal.title')"
    :width="700"
    hide-confirm
  >
        <div class="package-body">
          <aside>
            <input :placeholder="t('application.createWizard.packageModal.search')" />
            <button v-for="name in packageNames" :key="name" type="button" :class="{ active: selectedPackageName === name }" @click="selectedPackageName = name">{{ name }}</button>
          </aside>
          <main>
            <label v-for="version in packageVersions" :key="version.value" class="version-row">
              <input v-model="selectedPackageVersion" type="radio" :value="version.value" />
              <strong>{{ version.value }}</strong>
              <small>{{ t('application.createWizard.packageModal.arch') }}: {{ version.arch }}</small>
              <small>{{ t('application.createWizard.packageModal.createdAt') }}: {{ version.time }}</small>
            </label>
          </main>
        </div>
        <template #footer>
          <button class="link-button package-upload" type="button">+ {{ t('application.createWizard.packageModal.upload') }}</button>
          <button class="secondary-button" type="button" @click="packageSelectorOpen = false">{{ t('application.common.cancel') }}</button>
          <button class="primary-button" type="button" @click="confirmPackage">{{ t('application.common.confirm') }}</button>
        </template>
  </ApplicationDrawer>
</template>

<style scoped>
.breadcrumb { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; margin-bottom: 4px; }

/* ───────────── Stepper ───────────── */
.stepper { display: flex; gap: 0; padding: 16px 24px; margin: 0; list-style: none; border-bottom: 1px solid var(--hnb-color-divider, #222b3d); }
.stepper li { flex: 1; display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: var(--hnb-radius-md, 6px); color: var(--hnb-color-text-tertiary, #6b7a8a); }
.stepper li.active { color: var(--hnb-color-text-primary, #edeff5); background: color-mix(in srgb, var(--hnb-color-primary, #5b8dff) 15%, transparent); }
.stepper li.done { color: var(--hnb-color-status-success, #12b76a); }
.step-num { width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; border-radius: 50%; font-size: 12px; font-weight: 700; background: var(--hnb-color-border, #29344a); color: var(--hnb-color-text-tertiary, #6b7a8a); }
.stepper li.active .step-num { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.stepper li.done .step-num { background: var(--hnb-color-status-success, #12b76a); color: #fff; }
.stepper li strong { font-size: 13px; }

/* ───────────── Content ───────────── */
.wizard-content { flex: 1; overflow-y: auto; padding: 0; }
.wizard-section { padding: 20px 24px; }
.section-title { font-size: 15px; font-weight: 600; color: var(--hnb-color-text-primary, #edeff5); margin-bottom: 16px; }

/* ───────────── Form Card ───────────── */
.form-card { background: var(--hnb-color-bg-elevated, #171d31); border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-lg, 10px); padding: 20px; }
.form-grid { display: grid; gap: 16px; }
.form-grid.two-columns { grid-template-columns: 1fr 1fr; }
.form-grid label { display: flex; flex-direction: column; gap: 6px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.form-grid label.full { grid-column: 1 / -1; }
.form-grid .full { grid-column: 1 / -1; }
.form-grid .required span::after { content: ' *'; color: var(--hnb-color-status-danger, #f04438); }
.form-grid input, .form-grid textarea, .form-grid select { width: 100%; padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-md, 6px); background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.form-grid input:focus, .form-grid textarea:focus, .form-grid select:focus { outline: none; border-color: var(--hnb-color-primary, #5b8dff); }
.form-grid textarea { resize: vertical; min-height: 80px; }
.field-block { display: flex; flex-direction: column; gap: 8px; }
.field-block span { color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; }
.field-block.required > span::after { content: ' *'; color: var(--hnb-color-status-danger, #f04438); }
.field-hint { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.field-hint.tiny { font-size: 11px; margin-top: 8px; }
.radio-line { display: flex; flex-direction: row !important; align-items: center; gap: 8px; }

/* ───────────── Choice Grid ───────────── */
.choice-grid { display: grid; gap: 10px; }
.choice-grid.five { grid-template-columns: repeat(5, 1fr); }
.choice-grid.three { grid-template-columns: repeat(3, 1fr); }
.choice-card, .resource-card { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 14px 8px; border: 2px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-lg, 10px); background: var(--hnb-color-bg-surface, #101425); cursor: pointer; transition: border-color var(--hnb-duration-fast, 120ms); color: var(--hnb-color-text-primary, #edeff5); }
.choice-card:hover, .resource-card:hover { border-color: var(--hnb-color-primary, #5b8dff); }
.choice-card.selected, .resource-card.selected { border-color: var(--hnb-color-primary, #5b8dff); background: color-mix(in srgb, var(--hnb-color-primary, #5b8dff) 10%, transparent); }
.choice-card strong { font-size: 11px; font-weight: 600; }
.choice-card .pkg-icon { display: flex; align-items: center; justify-content: center; height: 32px; }
.resource-card { align-items: flex-start; text-align: left; padding: 16px; min-height: 100px; }
.resource-card .resource-icon { margin-bottom: 4px; }
.resource-card strong { font-size: 14px; }
.resource-card small { font-size: 11px; color: var(--hnb-color-text-tertiary, #6b7a8a); line-height: 1.4; }
.resource-card:disabled { opacity: 0.5; cursor: not-allowed; border-color: var(--hnb-color-border, #29344a); }
.resource-card:disabled:hover { border-color: var(--hnb-color-border, #29344a); }

/* ───────────── Segmented Control ───────────── */
.segmented { display: flex; gap: 4px; }
.segmented button { flex: 1; padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-md, 6px); background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 12px; cursor: pointer; }
.segmented button.active { background: var(--hnb-color-primary, #5b8dff); border-color: var(--hnb-color-primary, #5b8dff); color: #fff; }
.segmented button:disabled { opacity: 0.4; cursor: not-allowed; }
.segmented.compact button { flex: 0 1 auto; padding: 4px 10px; font-size: 11px; }

/* ───────────── Package Picker ───────────── */
.package-picker { display: flex; gap: 8px; }
.package-picker input { flex: 1; }
.package-picker button { padding: 8px 16px; border: 0; border-radius: var(--hnb-radius-md, 6px); background: var(--hnb-color-primary, #5b8dff); color: #fff; font-size: 12px; cursor: pointer; }

/* ───────────── Inline Control ───────────── */
.inline-control { display: flex; align-items: center; gap: 8px; }
.inline-control select { flex: 1; }
.inline-control .icon-btn { width: 32px; height: 32px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-sm, 4px); background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); cursor: pointer; }
.inline-control a { color: var(--hnb-color-primary, #5b8dff); font-size: 12px; text-decoration: none; white-space: nowrap; }

/* ───────────── Toggle Group ───────────── */
.toggle-group { display: inline-flex; align-items: center; gap: 4px; }
.toggle-group span { padding: 2px 8px; border-radius: var(--hnb-radius-sm, 4px); font-size: 12px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.toggle-group span.active { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.toggle-group input[type="checkbox"] { accent-color: var(--hnb-color-primary, #5b8dff); }
.switch-line { display: flex !important; flex-direction: row !important; align-items: center; gap: 8px; flex-wrap: wrap; }
.switch-line input[type="checkbox"] { accent-color: var(--hnb-color-primary, #5b8dff); }

/* ───────────── Runtime Card (Step 2) ───────────── */
.runtime-card { margin-bottom: 16px; background: var(--hnb-color-bg-elevated, #171d31); border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-lg, 10px); overflow: hidden; }
.runtime-card h3 { margin: 0; padding: 14px 18px; font-size: 14px; font-weight: 600; color: var(--hnb-color-text-primary, #edeff5); border-bottom: 1px solid var(--hnb-color-divider, #222b3d); }
.card-body { padding: 16px 18px; display: flex; flex-direction: column; gap: 20px; }
.card-body input, .card-body textarea, .card-body select { padding: 8px 12px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-md, 6px); background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.card-body input:focus, .card-body textarea:focus { outline: none; border-color: var(--hnb-color-primary, #5b8dff); }
.card-body textarea { resize: vertical; min-height: 60px; }
.section-label { font-size: 13px; font-weight: 600; color: var(--hnb-color-text-primary, #edeff5); display: block; }

/* ───────────── Quota Grid ───────────── */
.quota-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.quota-group { border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-md, 6px); padding: 12px; display: flex; flex-direction: column; gap: 8px; }
.quota-group legend { font-size: 12px; font-weight: 600; color: var(--hnb-color-text-secondary, #a9b2c2); padding: 0 4px; }
.quota-group label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--hnb-color-text-secondary, #a9b2c2); flex-wrap: wrap; }
.quota-group label input { width: 60px; }
.quota-note { display: block; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 10px; }

/* ───────────── Env Table ───────────── */
.env-table { border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-md, 6px); overflow: hidden; }
.env-head, .env-row { display: grid; grid-template-columns: 1fr 1fr 1fr 80px; gap: 4px; padding: 8px 12px; }
.env-head { background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; font-weight: 500; }
.env-row { background: transparent; }
.env-row input, .env-row select { padding: 4px 8px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-sm, 4px); background: var(--hnb-color-bg-surface, #101425); color: var(--hnb-color-text-primary, #edeff5); font-size: 12px; }
.text-button { background: none; border: none; color: var(--hnb-color-status-danger, #f04438); cursor: pointer; font-size: 12px; padding: 0; }
.table-toolbar { display: flex; align-items: center; gap: 8px; }
.link-button { background: none; border: none; color: var(--hnb-color-primary, #5b8dff); cursor: pointer; font-size: 12px; padding: 0; }

/* ───────────── Check Row ───────────── */
.check-row { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.check-row b { min-width: 80px; font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); font-weight: 500; }

/* ───────────── Field Row ───────────── */
.field-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.field-row span { font-size: 13px; color: var(--hnb-color-text-secondary, #a9b2c2); min-width: 80px; }
.field-row input { width: 100px; }
.unit { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }

/* ───────────── Switch Grid ───────────── */
.switch-grid { display: flex; flex-direction: column; gap: 8px; }

/* ───────────── Confirm Grid ───────────── */
.confirm-grid { display: grid; gap: 8px; background: var(--hnb-color-bg-elevated, #171d31); border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-lg, 10px); padding: 20px; }
.confirm-grid div { display: flex; justify-content: space-between; gap: 16px; padding: 10px 12px; border: 1px solid var(--hnb-color-divider, #222b3d); border-radius: var(--hnb-radius-md, 6px); background: var(--hnb-color-bg-surface, #101425); }
.confirm-grid span { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 13px; }
.confirm-grid strong { color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }

.primary-button, .secondary-button { padding: 8px 20px; border: 0; border-radius: var(--hnb-radius-md, 6px); cursor: pointer; font-size: 13px; font-weight: 600; }
.primary-button { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.primary-button:hover { background: var(--hnb-color-primary-hover, #7aa2ff); }
.secondary-button { background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-secondary, #a9b2c2); border: 1px solid var(--hnb-color-border, #29344a); }
.secondary-button:hover { background: var(--hnb-color-bg-surface, #101425); }

.package-body { display: flex; min-height: min(520px, 70vh); overflow: hidden; }
.package-body aside { width: 180px; padding: 12px; border-right: 1px solid var(--hnb-color-divider, #222b3d); display: flex; flex-direction: column; gap: 4px; }
.package-body aside input { padding: 6px 10px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: var(--hnb-radius-sm, 4px); background: var(--hnb-color-bg-elevated, #171d31); color: var(--hnb-color-text-primary, #edeff5); }
.package-body aside button { padding: 6px 10px; border: 0; border-radius: var(--hnb-radius-sm, 4px); background: transparent; color: var(--hnb-color-text-secondary, #a9b2c2); text-align: left; cursor: pointer; font-size: 12px; }
.package-body aside button.active { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.package-body main { flex: 1; padding: 12px; overflow-y: auto; }
.version-row { display: flex; align-items: center; gap: 8px; padding: 8px; border-radius: var(--hnb-radius-sm, 4px); cursor: pointer; }
.version-row:hover { background: var(--hnb-color-bg-elevated, #171d31); }
.version-row strong { font-size: 13px; color: var(--hnb-color-text-primary, #edeff5); }
.version-row small { font-size: 11px; color: var(--hnb-color-text-tertiary, #6b7a8a); }
.package-upload { margin-right: auto; }
</style>
