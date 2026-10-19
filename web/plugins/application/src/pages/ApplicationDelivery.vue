<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../deliveryApi'

type ActionName = 'deploy' | 'upgrade' | 'rollback' | 'delete'
const { t } = useI18n()
const deliveryId = ref('')
const readiness = ref<api.DeliveryReadiness | null>(null)
const preview = ref<api.DeliveryPreview | null>(null)
const planResult = ref<unknown>(null)
const composition = ref('')
const operation = ref<api.OperationRecord | null>(null)
const error = ref('')
const busy = ref(false)
const approve = ref(false)
const image = ref('docker.io/library/nginx@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10')
const sourceImage = ref('')
const targetImage = ref('')
const expectedUid = ref('')
const backupRef = ref('')
let poller: number | undefined

const operationId = computed(() => operation.value?.id || operation.value?.operationId || '')
const actionBusy = computed(() => busy.value)

onMounted(() => {
  deliveryId.value = localStorage.getItem('hnb.deliveryId') || ''
})
onBeforeUnmount(() => { if (poller) window.clearInterval(poller) })

function remember() {
  if (deliveryId.value) localStorage.setItem('hnb.deliveryId', deliveryId.value.trim())
}
function fail(err: unknown) {
  error.value = err instanceof Error ? err.message : String(err)
}
async function load() {
  error.value = ''; busy.value = true
  try {
    remember()
    readiness.value = await api.getReadiness(deliveryId.value.trim())
  } catch (err) { fail(err) } finally { busy.value = false }
}
async function previewPlan() {
  error.value = ''; busy.value = true
  try {
    let parsed: unknown
    try { parsed = JSON.parse(composition.value) } catch { throw new Error('composition must be valid JSON') }
    planResult.value = await api.previewPlan(parsed)
  } catch (err) { fail(err) } finally { busy.value = false }
}
async function loadPreview() {
  error.value = ''; busy.value = true
  try {
    remember()
    preview.value = await api.getDeletionPreview(deliveryId.value.trim())
  } catch (err) { fail(err) } finally { busy.value = false }
}
async function invoke(action: ActionName) {
  error.value = ''; busy.value = true
  try {
    remember()
    const payload: Record<string, unknown> = {
      action,
      idempotencyKey: `portal-${action}-${deliveryId.value.trim()}-${Date.now()}`,
    }
    if (action === 'deploy') payload.image = image.value
    if (action === 'upgrade' || action === 'rollback') {
      payload.sourceImage = sourceImage.value
      payload.targetImage = targetImage.value
      payload.expectedUid = expectedUid.value
      payload.backupRef = backupRef.value || `portal-${action}`
      if (action === 'upgrade') payload.dataCompatible = true
      else payload.dataIrreversible = false
    }
    if (action === 'delete') payload.expectedUid = expectedUid.value
    let op = await api.runAction(deliveryId.value.trim(), payload)
    if (approve.value && ['rollback', 'delete'].includes(action)) {
      const id = op.id || op.operationId
      if (id) op = await api.approveOperation(id)
    }
    operation.value = op
    startPolling()
  } catch (err) { fail(err) } finally { busy.value = false }
}
function startPolling() {
  if (poller) window.clearInterval(poller)
  poller = window.setInterval(async () => {
    const id = operationId.value
    if (!id) return
    try {
      const next = await api.getOperation(id)
      operation.value = next
      if (['succeeded', 'failed', 'cancelled'].includes(next.status || '')) {
        window.clearInterval(poller); poller = undefined
        await load()
      }
    } catch { /* transient; keep polling */ }
  }, 2000)
}
</script>

<template>
  <section class="delivery-page">
    <header class="page-head">
      <div>
        <h2>{{ t('application.delivery.title') }}</h2>
        <p>{{ t('application.delivery.desc') }}</p>
      </div>
    </header>

    <div class="panel">
      <label class="field">
        <span>{{ t('application.delivery.deliveryId') }}</span>
        <input v-model="deliveryId" placeholder="application delivery UUID" />
      </label>
      <div class="row">
        <button class="btn" :disabled="busy || !deliveryId" @click="load">{{ t('application.delivery.status') }}</button>
        <button class="btn" :disabled="busy || !deliveryId" @click="loadPreview">{{ t('application.delivery.preview') }}</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>

      <div v-if="readiness" class="result">
        <span :class="['badge', readiness.ready ? 'ok' : 'warn']">{{ readiness.ready ? 'Ready' : 'NotReady' }}</span>
        <span>revision {{ readiness.revision }}</span>
        <span>execution {{ readiness.execution }}</span>
        <span>observation {{ readiness.observation }}</span>
        <span>external {{ readiness.externalProbe }}</span>
        <span>volume {{ readiness.volume }}</span>
        <span v-if="readiness.reasons?.length">reasons {{ readiness.reasons.join(', ') }}</span>
      </div>

      <div v-if="preview" class="result column">
        <strong>{{ t('application.delivery.preview') }}</strong>
        <div v-for="item in preview.resources" :key="`${item.kind}-${item.name}`">
          {{ item.kind }}/{{ item.name }} → {{ item.action }}
        </div>
        <span>retainVolumes={{ preview.retainVolumes }}, dataCleanup={{ preview.requiresDataCleanupAuthorization }}</span>
      </div>
    </div>

    <div class="panel">
      <h3>Plan preview</h3>
      <label class="field"><span>composition JSON</span><textarea v-model="composition" rows="6" placeholder='{"schemaVersion":"application-composition/v1",...}'></textarea></label>
      <button class="btn" :disabled="busy || !composition" @click="previewPlan">Preview plan</button>
      <pre v-if="planResult" class="json">{{ JSON.stringify(planResult, null, 2) }}</pre>
    </div>

    <div class="grid">
      <div class="panel">
        <h3>Deploy</h3>
        <label class="field"><span>image</span><input v-model="image" /></label>
        <button class="btn primary" :disabled="actionBusy || !deliveryId" @click="invoke('deploy')">Deploy</button>
      </div>

      <div class="panel">
        <h3>Upgrade / Rollback</h3>
        <label class="field"><span>source image</span><input v-model="sourceImage" /></label>
        <label class="field"><span>target image</span><input v-model="targetImage" /></label>
        <label class="field"><span>expected UID</span><input v-model="expectedUid" /></label>
        <label class="field"><span>backup ref</span><input v-model="backupRef" /></label>
        <div class="row">
          <button class="btn" :disabled="actionBusy || !deliveryId" @click="invoke('upgrade')">Upgrade</button>
          <button class="btn" :disabled="actionBusy || !deliveryId" @click="invoke('rollback')">Rollback</button>
        </div>
      </div>

      <div class="panel">
        <h3>Delete</h3>
        <label class="field"><span>expected UID</span><input v-model="expectedUid" /></label>
        <label class="check"><input v-model="approve" type="checkbox" /> approve high-risk Operation</label>
        <button class="btn danger" :disabled="actionBusy || !deliveryId" @click="invoke('delete')">Delete workload</button>
      </div>
    </div>

    <div v-if="operation" class="panel">
      <h3>{{ t('application.delivery.operation') }}</h3>
      <div class="result">
        <span>{{ operationId }}</span>
        <span :class="['badge', operation.status === 'succeeded' ? 'ok' : 'warn']">{{ operation.status }}</span>
        <span>{{ operation.type }}</span>
      </div>
      <div v-for="step in operation.steps || []" :key="step.name" class="step">
        {{ step.name }} → {{ step.status }} <span v-if="step.failure?.message">({{ step.failure.message }})</span>
      </div>
      <p v-if="operation.failure?.message" class="error">{{ operation.failure.message }}</p>
    </div>
  </section>
</template>

<style scoped>
.delivery-page { display: flex; flex-direction: column; gap: 14px; }
.page-head h2 { margin: 0 0 4px; font-size: 18px; color: var(--hnb-color-text-primary, #edeff5); }
.page-head p { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 13px; }
.panel { background: var(--hnb-color-bg-elevated, #151b26); border: 1px solid var(--hnb-color-border, #263244); border-radius: 8px; padding: 14px; }
.panel h3 { margin: 0 0 10px; font-size: 14px; color: var(--hnb-color-text-primary, #edeff5); }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 14px; }
.field { display: flex; flex-direction: column; gap: 5px; margin-bottom: 9px; font-size: 12px; color: var(--hnb-color-text-secondary, #9aa7b8); }
.field input, .field textarea { background: #0f141d; color: #e8edf5; border: 1px solid #2b3a4d; border-radius: 5px; padding: 7px 8px; font-family: ui-monospace, monospace; }
.field textarea { color: #e8edf5; border: 1px solid #2b3a4d; border-radius: 5px; padding: 7px 8px; resize: vertical; }
.json { margin-top: 10px; max-height: 220px; overflow: auto; background: #0b1017; border-radius: 5px; padding: 10px; color: #9fb3cc; font-size: 11px; }
.row { display: flex; gap: 8px; flex-wrap: wrap; }
.btn { background: #223047; color: #dce6f2; border: 1px solid #344963; border-radius: 5px; padding: 7px 12px; cursor: pointer; }
.btn.primary { background: #1f6feb; border-color: #1f6feb; color: white; }
.btn.danger { background: #8b2635; border-color: #b63a4a; color: white; }
.btn:disabled { opacity: .45; cursor: not-allowed; }
.result { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; margin-top: 10px; font-size: 12px; color: #9aa7b8; }
.result.column { flex-direction: column; align-items: flex-start; }
.badge { border-radius: 999px; padding: 2px 8px; font-weight: 600; }
.badge.ok { background: #123d26; color: #55d98b; }
.badge.warn { background: #3d2a12; color: #f0b35b; }
.check { display: flex; gap: 7px; align-items: center; font-size: 12px; color: #9aa7b8; margin: 6px 0 10px; }
.error { color: #ff7b8a; font-size: 12px; white-space: pre-wrap; }
.step { font-size: 12px; color: #9aa7b8; margin-top: 6px; }
</style>
