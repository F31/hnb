<template>
  <div class="security-panel">
    <div class="metric-cards">
      <div class="metric-card"><span>{{ t('application.marketPage.security.totalScans') }}</span><strong>{{ totalScans }}</strong></div>
      <div class="metric-card"><span>{{ t('application.marketPage.security.pendingScans') }}</span><strong>{{ pendingScans }}</strong></div>
      <div class="metric-card"><span>{{ t('application.marketPage.security.criticalVulns') }}</span><strong>{{ criticalVulns }}</strong></div>
      <div class="metric-card"><span>{{ t('application.marketPage.security.lastDBSync') }}</span><strong>{{ lastDBSync || '-' }}</strong></div>
    </div>

    <div class="security-tabs">
      <button v-for="t in subTabs" :key="t.key" :class="{ active: activeSubTab === t.key }" @click="activeSubTab = t.key">{{ t.label }}</button>
    </div>

    <section v-if="activeSubTab === 'vulndb'" class="section-card">
      <div class="section-header">
        <h3 class="section-title">{{ t('application.marketPage.security.vulnDBRecords') }}</h3>
        <button class="primary-button vuln-db-update-button" type="button" @click="uploadDrawerOpen = true">
          {{ t('application.marketPage.security.vulnDBUpdate') }}
        </button>
      </div>

      <div class="vulndb-list">
        <div v-if="dbEntries.length" class="record-list">
          <div v-for="d in dbEntries" :key="d.id" class="record-card">
            <div class="record-field"><span class="record-label">{{ t('application.marketPage.security.updateVersion') }}</span><span class="record-value">{{ d.db_label || d.engine || '-' }}</span></div>
            <div class="record-field"><span class="record-label">{{ t('application.marketPage.security.updateUser') }}</span><span class="record-value">{{ d.tenant_id?.slice(0, 8) || '-' }}</span></div>
            <div class="record-field"><span class="record-label">{{ t('application.marketPage.common.createdAt') }}</span><span class="record-value">{{ d.created_at ? d.created_at.slice(0, 16) : '-' }}</span></div>
            <div class="record-field"><span class="record-label">{{ t('application.marketPage.common.updatedAt') }}</span><span class="record-value">{{ d.updated_at ? d.updated_at.slice(0, 16) : '-' }}</span></div>
          </div>
        </div>
        <div v-else class="empty-cell">{{ t('application.marketPage.security.noRecords') }}</div>
      </div>
    </section>

    <section v-if="activeSubTab === 'reports'" class="section-card">
      <div class="table-toolbar"><span></span></div>
      <table class="data-table">
        <thead><tr><th>{{ t('application.marketPage.security.artifactName') }}</th><th>{{ t('application.marketPage.security.cveId') }}</th><th>{{ t('application.marketPage.security.severity') }}</th><th>{{ t('application.marketPage.security.cvss3') }}</th><th>{{ t('application.marketPage.security.component') }}</th><th>{{ t('application.marketPage.security.currentVersion') }}</th><th>{{ t('application.marketPage.security.fixedVersion') }}</th><th>{{ t('application.marketPage.security.exempted') }}</th><th>{{ t('application.marketPage.security.affectedImages') }}</th></tr></thead>
        <tbody>
          <tr v-for="(f, i) in allFindings" :key="i">
            <td>{{ f._artifact_id?.slice(0, 12) || '-' }}</td>
            <td>{{ f.cve || '-' }}</td>
            <td><span class="severity-tag" :class="`severity-${(f.severity || '').toLowerCase()}`">{{ f.severity || '-' }}</span></td>
            <td>{{ f.cvss3 != null ? f.cvss3 : '-' }}</td>
            <td>{{ f.package || '-' }}</td>
            <td>{{ f.version || '-' }}</td>
            <td>{{ f.fixed_version || '-' }}</td>
            <td>{{ f.exempted ? '✓' : '-' }}</td>
            <td>{{ f.affected_images != null ? f.affected_images : '-' }}</td>
          </tr>
          <tr v-if="!allFindings.length"><td colspan="9" class="empty-cell">{{ t('application.marketPage.security.noReports') }}</td></tr>
        </tbody>
      </table>
    </section>

    <ApplicationDrawer
      v-model="uploadDrawerOpen"
      :title="t('application.marketPage.security.vulnDBUpdate')"
      :error="uploadError"
      :confirm-disabled="!selectedFile"
      :cancel-text="t('application.common.cancel')"
      :confirm-text="t('application.common.confirm')"
      @cancel="onCancel"
      @confirm="onConfirm"
    >
      <div class="drawer-form">
        <div class="field-group">
          <label class="field-label">{{ t('application.marketPage.security.downloadUrl') }}</label>
          <div class="field-row">
            <p class="download-url-text">{{ t('application.marketPage.security.downloadUrlDescription') }}</p>
          </div>
        </div>

        <div class="field-group">
          <label class="field-label">{{ t('application.marketPage.security.uploadArea') }}</label>
          <div class="upload-zone" @dragover.prevent @drop.prevent="onDrop">
            <div class="upload-icon">
              <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--hnb-color-primary, #5b8dff)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="17 8 12 3 7 8"/>
                <line x1="12" y1="3" x2="12" y2="15"/>
              </svg>
            </div>
            <p class="upload-hint">{{ t('application.marketPage.security.uploadHint') }}</p>
            <p class="upload-format">{{ t('application.marketPage.security.uploadFormat') }}</p>
            <input ref="fileInput" type="file" accept=".tgz" hidden @change="onFileSelected" />
            <button class="secondary-button" type="button" @click="fileInput?.click()">{{ t('application.marketPage.security.selectFile') }}</button>
          </div>
          <div v-if="selectedFile" class="file-info">
            <span class="file-name">{{ selectedFile.name }}</span>
            <span class="file-size">{{ (selectedFile.size / 1048576).toFixed(1) }} MiB</span>
            <button class="text-button" type="button" @click="clearSelectedFile">{{ t('application.common.cancel') }}</button>
          </div>
        </div>
      </div>
    </ApplicationDrawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { listSecurityReports, getDBStatus } from '../marketApi'
import ApplicationDrawer from '../components/ApplicationDrawer.vue'

const { t } = useI18n()

const dbEntries = ref<any[]>([])
const reports = ref<any[]>([])
const activeSubTab = ref('vulndb')
const uploadDrawerOpen = ref(false)
const uploadError = ref('')
const selectedFile = ref<File | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const subTabs = computed(() => [
  { key: 'vulndb', label: t('application.marketPage.security.vulnDBTab') },
  { key: 'reports', label: t('application.marketPage.security.scanReports') },
])

const allFindings = computed(() => {
  const list: any[] = []
  for (const r of reports.value) {
    const findings = Array.isArray(r.findings) ? r.findings : []
    for (const f of findings) {
      list.push({ _artifact_id: r.artifact_id, ...f })
    }
  }
  return list
})

const totalScans = computed(() => reports.value.length)
const pendingScans = computed(() => reports.value.filter((r) => r.state === 'queued' || r.state === 'running').length)
const criticalVulns = computed(() => {
  let total = 0
  for (const r of reports.value) {
    if (r.severity_summary?.critical) total += r.severity_summary.critical
  }
  return total
})
const lastDBSync = computed(() => {
  if (!dbEntries.value.length) return ''
  const latest = dbEntries.value.reduce((a, b) => {
    const at = a.last_sync_at || ''
    const bt = b.last_sync_at || ''
    return at > bt ? a : b
  })
  return latest.last_sync_at ? latest.last_sync_at.slice(0, 10) : ''
})

function onDrop(e: DragEvent) {
  const file = e.dataTransfer?.files?.[0]
  if (file) selectFile(file)
}

function onFileSelected(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (file) selectFile(file)
}

function selectFile(file: File) {
  if (!file.name.toLowerCase().endsWith('.tgz')) {
    clearSelectedFile()
    uploadError.value = t('application.marketPage.security.uploadFormat')
    return
  }
  selectedFile.value = file
  uploadError.value = ''
}

function clearSelectedFile() {
  selectedFile.value = null
  if (fileInput.value) fileInput.value.value = ''
}

function onConfirm() {
  // TODO: backend upload endpoint
  console.log('confirm', { file: selectedFile.value?.name })
}

function onCancel() {
  clearSelectedFile()
  uploadError.value = ''
}

async function fetchData() {
  reports.value = await listSecurityReports()
  try { dbEntries.value = await getDBStatus() } catch { dbEntries.value = [] }
}

onMounted(fetchData)
</script>

<style scoped>
.security-panel { display: flex; flex-direction: column; gap: 20px; }
.metric-cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; }
.metric-card { display: flex; flex-direction: column; gap: 6px; padding: 16px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 10px; background: var(--hnb-color-bg-surface, var(--hnb-color-bg-surface, #101425)); }
.metric-card span { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; }
.metric-card strong { color: var(--hnb-color-text-primary, #edeff5); font-size: 22px; font-weight: 600; }
.security-tabs { display: flex; gap: 4px; }
.security-tabs button { background: transparent; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; padding: 6px 16px; cursor: pointer; }
.security-tabs button.active { background: #1a5fb4; border-color: #1a5fb4; color: #fff; }
.section-card { display: flex; flex-direction: column; gap: 20px; }
.section-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.section-title { margin: 0; color: var(--hnb-color-text-primary, #edeff5); font-size: 15px; font-weight: 600; }
.drawer-form { display: flex; flex-direction: column; gap: 20px; }
.field-group { display: flex; flex-direction: column; gap: 8px; }
.field-label { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; }
.field-row { display: flex; gap: 8px; }
.download-url-text { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 13px; line-height: 1.5; word-break: break-all; }
.field-input:focus { border-color: var(--hnb-color-primary, #5b8dff); }
.upload-zone { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 14px; border: 2px dashed var(--hnb-color-border, #29344a); border-radius: 12px; background: var(--hnb-color-bg-void, #0b0f14); cursor: pointer; }
.upload-zone:hover { border-color: var(--hnb-color-primary, #5b8dff); }
.upload-icon { margin-bottom: 0; }
.upload-icon svg { width: 22px; height: 22px; }
.upload-hint { margin: 0; color: var(--hnb-color-text-secondary, #a9b2c2); font-size: 14px; }
.upload-format { margin: 0; color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.file-info { display: flex; align-items: center; gap: 12px; padding: 10px 14px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: var(--hnb-color-bg-surface, var(--hnb-color-bg-surface, #101425)); }
.file-name { color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.file-size { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 12px; }
.text-button { background: none; border: none; color: var(--hnb-color-primary, #5b8dff); cursor: pointer; font-size: 12px; padding: 0; }
.text-button:hover { color: var(--hnb-color-primary, #5b8dff); }
.primary-button, .secondary-button { padding: 10px 24px; border: 0; border-radius: 6px; cursor: pointer; font-size: 13px; }
.primary-button { background: var(--hnb-color-primary, #5b8dff); color: #fff; }
.primary-button:hover:not(:disabled) { background: var(--hnb-color-primary-hover, #7aa2ff); }
.primary-button:disabled { background: #293443; color: var(--hnb-color-text-tertiary, #6b7a8a); cursor: not-allowed; }
.vuln-db-update-button { flex: 0 0 auto; }
.secondary-button { background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); color: var(--hnb-color-text-primary, #edeff5); border: 1px solid var(--hnb-color-border, #29344a); }
.secondary-button:hover { background: #293443; }
.data-table { width: 100%; border-collapse: collapse; }
.data-table th, .data-table td { padding: 10px 14px; text-align: left; border-bottom: 1px solid var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.data-table th { color: var(--hnb-color-text-tertiary, #6b7a8a); font-weight: 500; text-transform: uppercase; font-size: 11px; letter-spacing: 0.06em; }
.empty-cell { color: var(--hnb-color-text-tertiary, #6b7a8a); text-align: center; padding: 40px 14px; }
.table-toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.vulndb-list { display: flex; flex-direction: column; gap: 12px; }
.record-list { display: flex; flex-direction: column; gap: 8px; }
.record-card { display: flex; flex-wrap: wrap; gap: 16px 32px; padding: 12px 16px; border: 1px solid var(--hnb-color-border, #29344a); border-radius: 8px; background: var(--hnb-color-bg-surface, var(--hnb-color-bg-surface, #101425)); }
.record-field { display: flex; flex-direction: column; gap: 2px; min-width: 120px; }
.record-label { color: var(--hnb-color-text-tertiary, #6b7a8a); font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; }
.record-value { color: var(--hnb-color-text-primary, #edeff5); font-size: 13px; }
.status-pill { display: inline-block; padding: 2px 10px; border-radius: 10px; font-size: 11px; font-weight: 500; text-transform: uppercase; }
.status-pill.status-active, .status-pill.status-up-to-date { background: #143d2b; color: var(--hnb-color-status-success, var(--hnb-color-status-success, #12b76a)); }
.status-pill.status-updating, .status-pill.status-running { background: #1a3d5c; color: var(--hnb-color-status-info, #5bb8f5); }
.status-pill.status-error, .status-pill.status-failed { background: #4a1a1a; color: var(--hnb-color-status-danger, #f04438); }
.status-pill.status-queued { background: #2a2a1a; color: var(--hnb-color-status-warning, #f79009); }
.status-pill.status-disabled { background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); color: var(--hnb-color-text-tertiary, #6b7a8a); }
.severity-tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
.severity-tag.severity-critical { background: #4a1a1a; color: var(--hnb-color-status-danger, #f04438); }
.severity-tag.severity-high { background: #4a2a1a; color: var(--hnb-color-status-warning, #f79009); }
.severity-tag.severity-medium { background: #2a3a1a; color: #d4d84a; }
.severity-tag.severity-low { background: #1a2a3a; color: #6bb5f5; }
.severity-tag.severity-unknown { background: var(--hnb-color-bg-elevated, var(--hnb-color-bg-elevated, #171d31)); color: var(--hnb-color-text-tertiary, #6b7a8a); }
</style>
