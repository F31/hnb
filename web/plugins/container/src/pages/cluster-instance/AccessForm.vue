<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { HNBButton } from '@hnb/ui-kit'
import { listNamespaces, listWorkspaceClusters, type ContainerCluster } from '../../api/containerApi'
import {
  listAccessIngresses,
  listAccessNetworkPolicies,
  listAccessServices,
  saveAccessIngress,
  saveAccessNetworkPolicy,
  saveAccessService,
  type AccessIngressRule,
  type AccessPolicyRule,
  type AccessServicePort,
  type NetworkProtocol,
  type ServiceAccessType,
} from '../../api/accessApi'
import NetworkDrawer from '../network/NetworkDrawer.vue'
import Access from './Access.vue'

type FormKind = 'service' | 'ingress' | 'networkPolicy'
type LabelRow = { key: string; value: string }

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const kind = computed<FormKind>(() => route.path.includes('/service/') ? 'service' : route.path.includes('/ingress/') ? 'ingress' : 'networkPolicy')
const editingName = computed(() => String(route.params.name ?? ''))
const editing = computed(() => Boolean(editingName.value))
const title = computed(() => t(`container.access.form.${kind.value === 'service' ? (editing.value ? 'serviceEdit' : 'serviceCreate') : kind.value === 'ingress' ? (editing.value ? 'ingressEdit' : 'ingressCreate') : (editing.value ? 'policyEdit' : 'policyCreate')}`))

const clusters = ref<ContainerCluster[]>([])
const namespaces = ref<string[]>([])
const clusterId = ref('')
const namespace = ref('default')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const drawerVisible = ref(false)

const name = ref('')
const ipv6 = ref(false)
const serviceType = ref<ServiceAccessType>('ClusterIP')
const ports = ref<AccessServicePort[]>([{ name: '', port: 80, targetPort: 8080, protocol: 'TCP' }])
const relatedApp = ref(true)
const appCategory = ref('stateful')
const appName = ref('os-elasticsearch')
const metadataLabels = ref<LabelRow[]>([])

const tls = ref(false)
const certificate = ref('')
const ingressRules = ref<AccessIngressRule[]>([{ host: '', path: '/', serviceName: '', servicePortName: '', servicePort: 80 }])
const serviceChoices = ref<string[]>([])

const description = ref('')
const podLabelPreset = ref('app')
const matchLabels = ref<LabelRow[]>([{ key: '', value: '' }])
const policyIngress = ref<AccessPolicyRule[]>([])
const policyEgress = ref<AccessPolicyRule[]>([])

const clusterOptions = computed(() => clusters.value.map((item) => ({ value: item.id, label: item.display_name || item.name })))
const namespaceOptions = computed(() => Array.from(new Set([kind.value === 'service' ? 'default' : 'argocd', ...namespaces.value])))
const namePattern = /^[a-z0-9]([-a-z0-9]{0,46}[a-z0-9])?$/

function addPort(): void {
  ports.value.push({ name: '', port: 80, targetPort: 8080, protocol: 'TCP' })
}

function addIngressRule(): void {
  ingressRules.value.push({ host: '', path: '/', serviceName: '', servicePortName: '', servicePort: 80 })
}

function addMetadataLabel(): void {
  metadataLabels.value.push({ key: '', value: '' })
}

function addMatchLabel(): void {
  matchLabels.value.push({ key: '', value: '' })
}

function addPresetLabel(): void {
  if (!matchLabels.value.some((item) => item.key === podLabelPreset.value)) matchLabels.value.push({ key: podLabelPreset.value, value: '' })
}

function addPolicyRule(direction: 'ingress' | 'egress'): void {
  const target = direction === 'ingress' ? policyIngress : policyEgress
  target.value.push({ namespace: '', port: undefined, protocol: 'TCP' })
}

function rowsToLabels(rows: LabelRow[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (!key || key in result) throw new Error(t('container.access.validation.labels'))
    result[key] = row.value.trim()
  }
  return result
}

function resetForm(): void {
  name.value = ''
  ipv6.value = false
  serviceType.value = 'ClusterIP'
  ports.value = [{ name: '', port: 80, targetPort: 8080, protocol: 'TCP' }]
  relatedApp.value = true
  appCategory.value = 'stateful'
  appName.value = 'os-elasticsearch'
  metadataLabels.value = []
  tls.value = false
  certificate.value = ''
  ingressRules.value = [{ host: '', path: '/', serviceName: '', servicePortName: '', servicePort: 80 }]
  description.value = ''
  matchLabels.value = [{ key: '', value: '' }]
  policyIngress.value = []
  policyEgress.value = []
}

async function loadServiceChoices(): Promise<void> {
  if (!clusterId.value || !namespace.value) return
  try {
    serviceChoices.value = (await listAccessServices(clusterId.value, namespace.value)).map((item) => item.name)
  } catch {
    serviceChoices.value = []
  }
}

async function loadExisting(): Promise<void> {
  if (!editing.value) return
  if (kind.value === 'service') {
    const item = (await listAccessServices(clusterId.value, namespace.value)).find((candidate) => candidate.name === editingName.value)
    if (!item) return
    name.value = item.name
    ipv6.value = item.ipv6
    serviceType.value = item.type
    ports.value = item.ports.map((port) => ({ ...port }))
    relatedApp.value = Boolean(item.appName)
    appCategory.value = item.appCategory || 'stateful'
    appName.value = item.appName || 'os-elasticsearch'
    metadataLabels.value = Object.entries(item.labels).map(([key, value]) => ({ key, value }))
  } else if (kind.value === 'ingress') {
    const item = (await listAccessIngresses(clusterId.value, namespace.value)).find((candidate) => candidate.name === editingName.value)
    if (!item) return
    name.value = item.name
    tls.value = item.tls
    certificate.value = item.certificate ?? ''
    ingressRules.value = item.rules.map((rule) => ({ ...rule }))
  } else {
    const item = (await listAccessNetworkPolicies(clusterId.value, namespace.value)).find((candidate) => candidate.name === editingName.value)
    if (!item) return
    name.value = item.name
    description.value = item.description
    matchLabels.value = Object.entries(item.matchLabels).map(([key, value]) => ({ key, value }))
    policyIngress.value = item.ingress.map((rule) => ({ ...rule }))
    policyEgress.value = item.egress.map((rule) => ({ ...rule }))
  }
}

async function initialize(): Promise<void> {
  loading.value = true
  error.value = ''
  resetForm()
  try {
    clusters.value = await listWorkspaceClusters()
    clusterId.value = String(route.query.cluster || clusters.value[0]?.id || '')
    namespace.value = String(route.query.namespace || (kind.value === 'service' ? 'default' : 'argocd'))
    const items = await listNamespaces({ clusterId: clusterId.value || undefined })
    namespaces.value = items.map((item) => item.name)
    await Promise.all([loadServiceChoices(), loadExisting()])
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : String(caught)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  error.value = ''
  if (!clusterId.value || !namespace.value || !namePattern.test(name.value.trim())) {
    error.value = !name.value.trim() ? t('container.access.validation.required') : t('container.access.validation.name')
    return
  }
  saving.value = true
  try {
    if (kind.value === 'service') {
      if (!ports.value.length || ports.value.some((item) => item.port < 1 || item.port > 65535 || Number(item.targetPort) < 1 || Number(item.targetPort) > 65535)) throw new Error(t('container.access.validation.ports'))
      const labels = metadataLabels.value.length ? rowsToLabels(metadataLabels.value) : {}
      await saveAccessService(clusterId.value, {
        name: name.value.trim(), namespace: namespace.value, type: serviceType.value, clusterIp: '', ipv6: ipv6.value,
        ports: ports.value.map((item) => ({ ...item })), selector: relatedApp.value ? { app: appName.value } : {},
        appCategory: relatedApp.value ? appCategory.value : '', appName: relatedApp.value ? appName.value : '', labels, createdAt: '',
      }, editingName.value || undefined)
    } else if (kind.value === 'ingress') {
      if (!ingressRules.value.length || ingressRules.value.some((item) => !item.host.trim() || !item.path.trim() || !item.serviceName.trim() || item.servicePort < 1)) throw new Error(t('container.access.validation.required'))
      await saveAccessIngress(clusterId.value, {
        name: name.value.trim(), namespace: namespace.value, tls: tls.value, certificate: tls.value ? certificate.value : '',
        rules: ingressRules.value.map((item) => ({ ...item })), labels: {}, createdAt: '',
      }, editingName.value || undefined)
    } else {
      const labels = rowsToLabels(matchLabels.value)
      const policyTypes: Array<'Ingress' | 'Egress'> = []
      if (policyIngress.value.length) policyTypes.push('Ingress')
      if (policyEgress.value.length) policyTypes.push('Egress')
      if (!policyTypes.length) policyTypes.push('Ingress')
      await saveAccessNetworkPolicy(clusterId.value, {
        name: name.value.trim(), namespace: namespace.value, description: description.value.trim(), matchLabels: labels,
        policyTypes, ingress: policyIngress.value.map((item) => ({ ...item })), egress: policyEgress.value.map((item) => ({ ...item })), labels: {}, createdAt: '',
      }, editingName.value || undefined)
    }
    await router.push({ path: '/container/instances/access', query: { tab: kind.value, saved: '1' } })
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : String(caught)
  } finally {
    saving.value = false
  }
}

function cancel(): void {
  void router.push({ path: '/container/instances/access', query: { tab: kind.value } })
}

watch(namespace, loadServiceChoices)
watch(() => route.fullPath, initialize)
onMounted(() => {
  drawerVisible.value = true
  void initialize()
})
</script>

<template>
  <Access />
  <NetworkDrawer v-model="drawerVisible" :title="title" :busy="loading || saving" :error="error" @cancel="cancel" @confirm="save">
    <p v-if="loading" class="form-loading" role="status">{{ t('container.access.loading') }}</p>
    <form v-else class="access-form" @submit.prevent="save">
      <div class="field-row"><label><span class="required">*</span>{{ t('container.access.form.cluster') }}</label><select v-model="clusterId"><option v-for="item in clusterOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></div>
      <div class="field-row"><label><span class="required">*</span>{{ t('container.access.form.namespace') }}</label><select v-model="namespace"><option v-for="item in namespaceOptions" :key="item" :value="item">{{ item }}</option></select></div>

      <template v-if="kind === 'service'">
        <div class="field-row"><label><span class="required">*</span>{{ t('container.access.form.serviceName') }}</label><input v-model="name" type="text" :disabled="editing"></div>
        <div class="field-row"><label>{{ t('container.access.form.ipv6') }}</label><div class="radio-group"><label><input v-model="ipv6" type="radio" :value="true">{{ t('container.access.form.yes') }}</label><label><input v-model="ipv6" type="radio" :value="false">{{ t('container.access.form.no') }}</label></div></div>
        <div class="field-row"><label><span class="required">*</span>{{ t('container.access.form.accessType') }}</label><select v-model="serviceType"><option value="ClusterIP">{{ t('container.access.serviceType.ClusterIP') }}</option><option value="NodePort">{{ t('container.access.serviceType.NodePort') }}</option><option value="LoadBalancer">{{ t('container.access.serviceType.LoadBalancer') }}</option></select></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.portMapping') }}</label><div class="repeat-section"><button class="add-link" type="button" @click="addPort">{{ t('container.access.action.add') }}</button><div v-for="(port, index) in ports" :key="index" class="port-row"><label><span>{{ t('container.access.form.portName') }}</span><input v-model="port.name" :placeholder="t('container.access.form.portNamePlaceholder')"><small>{{ t('container.access.form.portNameHelp') }}</small></label><label><span>{{ t('container.access.form.servicePort') }}</span><input v-model.number="port.port" type="number" min="1" max="65535" :placeholder="t('container.access.form.portRange')"></label><label><span>{{ t('container.access.form.targetPort') }}</span><input v-model="port.targetPort" type="number" min="1" max="65535" :placeholder="t('container.access.form.portRange')"></label><label><span>{{ t('container.access.form.protocol') }}</span><select v-model="port.protocol"><option>TCP</option><option>UDP</option><option>SCTP</option></select></label><button class="remove-row" type="button" :disabled="ports.length === 1" @click="ports.splice(index, 1)">×</button></div></div></div>
        <div class="field-row"><label>{{ t('container.access.form.relatedApp') }}</label><div class="radio-group"><label><input v-model="relatedApp" type="radio" :value="true">{{ t('container.access.form.yes') }}</label><label><input v-model="relatedApp" type="radio" :value="false">{{ t('container.access.form.no') }}</label></div></div>
        <div v-if="relatedApp" class="field-row"><label><span class="required">*</span>{{ t('container.access.form.appCategory') }}</label><select v-model="appCategory"><option value="stateful">{{ t('container.access.form.stateful') }}</option><option value="stateless">{{ t('container.access.form.stateless') }}</option></select></div>
        <div v-if="relatedApp" class="field-row"><label><span class="required">*</span>{{ t('container.access.form.appSelect') }}</label><select v-model="appName"><option>os-elasticsearch</option><option>api-gateway</option><option>redis</option></select></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.labels') }}</label><div class="repeat-section"><div v-for="(label, index) in metadataLabels" :key="index" class="label-row"><input v-model="label.key" :placeholder="t('container.access.form.keyPlaceholder')"><input v-model="label.value" :placeholder="t('container.access.form.valuePlaceholder')"><button type="button" @click="metadataLabels.splice(index, 1)">×</button></div><button class="add-link" type="button" @click="addMetadataLabel">{{ t('container.access.action.add') }}</button></div></div>
      </template>

      <template v-else-if="kind === 'ingress'">
        <div class="field-row field-row--top"><label><span class="required">*</span>{{ t('container.access.form.name') }}</label><div><input v-model="name" type="text" :disabled="editing"><small>{{ t('container.access.form.nameHelp') }}</small></div></div>
        <div class="field-row"><label>{{ t('container.access.form.tls') }}</label><div class="radio-group"><label><input v-model="tls" type="radio" :value="true">{{ t('container.access.form.yes') }}</label><label><input v-model="tls" type="radio" :value="false">{{ t('container.access.form.no') }}</label></div></div>
        <div v-if="tls" class="field-row"><label>{{ t('container.access.form.certificate') }}</label><select v-model="certificate"><option value="">{{ t('container.access.form.select') }}</option><option>argocd-tls</option><option>platform-tls</option></select></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.rules') }}</label><div class="repeat-section"><button class="add-link" type="button" @click="addIngressRule">{{ t('container.access.action.createRule') }}</button><div v-for="(rule, index) in ingressRules" :key="index" class="rule-card"><button class="remove-card" type="button" :disabled="ingressRules.length === 1" @click="ingressRules.splice(index, 1)">×</button><label><span>{{ t('container.access.form.host') }} *</span><input v-model="rule.host" :placeholder="t('container.access.form.hostPlaceholder')"></label><label><span>{{ t('container.access.form.path') }} *</span><input v-model="rule.path" :placeholder="t('container.access.form.pathPlaceholder')"></label><strong>{{ t('container.access.form.serviceConfig') }}</strong><div class="service-config"><label><span>{{ t('container.access.form.backendService') }}</span><select v-model="rule.serviceName"><option value="">{{ t('container.access.form.select') }}</option><option v-for="service in serviceChoices" :key="service" :value="service">{{ service }}</option></select></label><label><span>{{ t('container.access.form.servicePortName') }}</span><input v-model="rule.servicePortName"></label><label><span>{{ t('container.access.form.backendPort') }}</span><input v-model.number="rule.servicePort" type="number" min="1" max="65535"></label></div></div></div></div>
      </template>

      <template v-else>
        <div class="field-row field-row--top"><label><span class="required">*</span>{{ t('container.access.form.name') }}</label><div><input v-model="name" type="text" :disabled="editing"><small>{{ t('container.access.form.nameHelp') }}</small></div></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.description') }}</label><div class="textarea-counter"><textarea v-model="description" rows="4" maxlength="128" :placeholder="t('container.access.form.descriptionPlaceholder')" /><span>{{ description.length }}/128</span></div></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.podSelector') }}</label><div class="repeat-section"><div class="preset-label"><span>{{ t('container.access.form.podLabelAdd') }} ?</span><select v-model="podLabelPreset"><option value="app">app</option><option value="component">component</option><option value="app.kubernetes.io/name">app.kubernetes.io/name</option></select><HNBButton size="small" type="button" @click="addPresetLabel">{{ t('container.access.action.addLabel') }}</HNBButton></div><strong>{{ t('container.access.form.matchLabels') }}</strong><div v-for="(label, index) in matchLabels" :key="index" class="label-row"><input v-model="label.key" :placeholder="t('container.access.form.keyPlaceholder')"><input v-model="label.value" :placeholder="t('container.access.form.valuePlaceholder')"><button type="button" :disabled="matchLabels.length === 1" @click="matchLabels.splice(index, 1)">×</button></div><button class="add-link" type="button" @click="addMatchLabel">{{ t('container.access.action.add') }}</button></div></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.ingress') }}</label><div class="repeat-section"><button class="add-link" type="button" @click="addPolicyRule('ingress')">⊕ {{ t('container.access.action.add') }}</button><div v-for="(rule, index) in policyIngress" :key="index" class="policy-rule"><input v-model="rule.namespace" :placeholder="t('container.access.form.ruleNamespace')"><input v-model.number="rule.port" type="number" min="1" max="65535" :placeholder="t('container.access.form.rulePort')"><select v-model="rule.protocol"><option>TCP</option><option>UDP</option><option>SCTP</option></select><button type="button" @click="policyIngress.splice(index, 1)">×</button></div></div></div>
        <div class="field-row field-row--top"><label>{{ t('container.access.form.egress') }}</label><div class="repeat-section"><button class="add-link" type="button" @click="addPolicyRule('egress')">⊕ {{ t('container.access.action.add') }}</button><div v-for="(rule, index) in policyEgress" :key="index" class="policy-rule"><input v-model="rule.namespace" :placeholder="t('container.access.form.ruleNamespace')"><input v-model.number="rule.port" type="number" min="1" max="65535" :placeholder="t('container.access.form.rulePort')"><select v-model="rule.protocol"><option>TCP</option><option>UDP</option><option>SCTP</option></select><button type="button" @click="policyEgress.splice(index, 1)">×</button></div></div></div>
      </template>

    </form>
  </NetworkDrawer>
</template>

<style scoped>
.form-loading { margin: 0; padding: 9px 12px; border-radius: var(--hnb-radius-sm); color: var(--hnb-color-text-secondary); font-size: 13px; }
.access-form { display: flex; flex-direction: column; gap: 16px; }
.field-row { display: grid; grid-template-columns: 1fr; align-items: center; gap: 6px; }
.field-row--top { align-items: start; }
.field-row > label:first-child { color: var(--hnb-color-text-secondary); font-size: 13px; }
.required { margin-right: 4px; color: var(--hnb-color-status-danger); }
.field-row input, .field-row select, .field-row textarea, .repeat-section input, .repeat-section select { width: 100%; box-sizing: border-box; padding: 8px 10px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-sm); background: var(--hnb-color-bg-elevated); color: var(--hnb-color-text-primary); }
.field-row small, .port-row small { display: block; margin-top: 5px; color: var(--hnb-color-text-tertiary); font-size: 12px; }
.radio-group { display: flex; gap: 22px; align-items: center; }
.radio-group label { display: inline-flex; align-items: center; gap: 7px; color: var(--hnb-color-text-primary); }
.radio-group input { width: auto; accent-color: var(--hnb-color-primary); }
.repeat-section { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.add-link { align-self: flex-start; padding: 3px 0; border: 0; background: transparent; color: var(--hnb-color-primary); cursor: pointer; }
.port-row { position: relative; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 14px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.port-row label, .rule-card label { display: flex; flex-direction: column; gap: 6px; color: var(--hnb-color-text-secondary); font-size: 12px; }
.remove-row, .remove-card, .label-row button, .policy-rule button { border: 0; background: transparent; color: var(--hnb-color-status-danger); cursor: pointer; }
.remove-row:disabled, .remove-card:disabled, .label-row button:disabled { opacity: .35; cursor: not-allowed; }
.label-row { display: grid; grid-template-columns: 1fr 1fr 34px; gap: 8px; }
.rule-card { position: relative; display: grid; grid-template-columns: 1fr; gap: 12px; padding: 16px; border: 1px solid var(--hnb-color-border); border-radius: var(--hnb-radius-md); }
.rule-card strong, .rule-card .service-config { grid-column: auto; }
.remove-card { position: absolute; top: 6px; right: 6px; }
.service-config { display: grid; grid-template-columns: 1fr; gap: 10px; }
.textarea-counter { position: relative; }
.textarea-counter span { position: absolute; right: 9px; bottom: 7px; color: var(--hnb-color-text-tertiary); font-size: 11px; }
.preset-label { display: grid; grid-template-columns: auto 1fr auto; gap: 8px; align-items: center; color: var(--hnb-color-text-secondary); font-size: 13px; }
.policy-rule { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
@media (max-width: 760px) {
  .port-row, .policy-rule { grid-template-columns: 1fr; }
}
</style>
