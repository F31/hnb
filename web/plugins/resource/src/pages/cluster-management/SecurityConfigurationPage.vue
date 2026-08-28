<script setup lang="ts">
/**
 * SecurityConfigurationPage — 安全配置中心（OpenSpec security-configuration）。
 * 独立全局路由 `/security/configuration`；两个页签：漏洞库设置（两步纵向步骤：
 * 下载漏洞库 → 上传漏洞库 .tgz，仅接受 .tgz）、漏洞扫描设置（空态壳层）。
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { HNBButton, HNBPageState } from '@hnb/ui-kit'
import {
  getVulnerabilityDbStatus,
  uploadVulnerabilityDatabase,
  type VulnerabilityDbStatus,
} from './api/p4Api'
import SectionHeader from './components/SectionHeader.vue'
import VulnerabilityUploadZone from './components/VulnerabilityUploadZone.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const activeTab = computed(() => (/\/vulnerability-scan$/.test(route.path) ? 'scan' : 'database'))

const dbStatus = ref<VulnerabilityDbStatus | null>(null)
const selectedFile = ref<File | null>(null)
const uploading = ref(false)
const progress = ref(0)
const uploadError = ref('')
const uploadSuccess = ref(false)

const tabs = computed(() => [
  { key: 'database', label: t('resource.clusterMgmt.security.tab.database') },
  { key: 'scan', label: t('resource.clusterMgmt.security.tab.scan') },
])

function switchTab(key: string): void {
  router.push(`/security/configuration/${key === 'scan' ? 'vulnerability-scan' : 'vulnerability-database'}`)
}

function onSelected(file: File): void {
  selectedFile.value = file
  uploadError.value = ''
  uploadSuccess.value = false
  progress.value = 0
}

function cancelUpload(): void {
  selectedFile.value = null
  uploadError.value = ''
  uploadSuccess.value = false
  progress.value = 0
}

async function confirmUpload(): Promise<void> {
  if (!selectedFile.value || uploading.value) return
  if (!selectedFile.value.name.toLowerCase().endsWith('.tgz')) {
    uploadError.value = t('resource.clusterMgmt.security.vulnUpload.extError')
    return
  }
  uploading.value = true
  uploadError.value = ''
  uploadSuccess.value = false
  try {
    await uploadVulnerabilityDatabase(selectedFile.value, (p) => {
      progress.value = p
    })
    uploadSuccess.value = true
    selectedFile.value = null
  } catch (err) {
    uploadError.value = err instanceof Error ? err.message : String(err)
  } finally {
    uploading.value = false
  }
}

onMounted(async () => {
  try {
    dbStatus.value = await getVulnerabilityDbStatus()
  } catch {
    dbStatus.value = null
  }
})
</script>

<template>
  <div class="security-config">
    <header class="security-header">
      <div>
        <h2 class="security-title">{{ t('resource.clusterMgmt.security.title') }}</h2>
        <p v-if="dbStatus" class="security-status">
          {{ t('resource.clusterMgmt.security.dbStatus') }}：{{ dbStatus.label }}
        </p>
      </div>
      <a class="help-link" href="https://docs.hnb.example.io/security" target="_blank" rel="noopener noreferrer">
        {{ t('resource.clusterMgmt.security.help') }}
      </a>
    </header>

    <nav class="security-tabs" :aria-label="t('resource.clusterMgmt.aria.securityTabs')">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="security-tab"
        :class="{ active: activeTab === tab.key }"
        @click="switchTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <!-- 漏洞库设置 -->
    <template v-if="activeTab === 'database'">
      <ol class="vuln-steps">
        <li class="vuln-step">
          <div class="vuln-step__head">
            <span class="vuln-step__index">1</span>
            <SectionHeader :title="t('resource.clusterMgmt.security.step.download')" />
          </div>
          <p class="vuln-step__desc">
            {{ t('resource.clusterMgmt.security.step.downloadDesc') }}
            <a href="https://github.com/aquasecurity/trivy-db/releases" target="_blank" rel="noopener noreferrer">
              Trivy DB releases
            </a>
            {{ t('resource.clusterMgmt.security.step.downloadFile') }}
          </p>
        </li>
        <li class="vuln-step">
          <div class="vuln-step__head">
            <span class="vuln-step__index">2</span>
            <SectionHeader :title="t('resource.clusterMgmt.security.step.upload')" />
          </div>
          <VulnerabilityUploadZone
            :uploading="uploading"
            :progress="progress"
            :error="uploadError"
            @selected="onSelected"
          />
          <div class="vuln-actions">
            <HNBButton variant="secondary" :disabled="uploading" @click="cancelUpload">
              {{ t('resource.clusterMgmt.common.cancel') }}
            </HNBButton>
            <HNBButton :loading="uploading" :disabled="!selectedFile" @click="confirmUpload">
              {{ t('resource.clusterMgmt.security.confirm') }}
            </HNBButton>
          </div>
          <p v-if="uploadSuccess" class="upload-success" role="status">
            {{ t('resource.clusterMgmt.security.uploadSuccess') }}
          </p>
        </li>
      </ol>
    </template>

    <!-- 漏洞扫描设置 -->
    <template v-else>
      <HNBPageState
        state="empty"
        :title="t('resource.clusterMgmt.placeholder.notEnabled')"
        :description="t('resource.clusterMgmt.security.scanNotEnabled')"
      />
    </template>
  </div>
</template>

<style scoped>
.security-config {
  display: flex;
  flex-direction: column;
  gap: 16px;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 20px;
}
.security-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.security-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.security-status { margin: 4px 0 0; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); }
.help-link { color: var(--hnb-color-primary, #2f6fed); font-size: 13px; text-decoration: none; }
.security-tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--hnb-color-border, #e2e7ef); }
.security-tab {
  position: relative;
  padding: 8px 16px;
  border: 0;
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
  cursor: pointer;
}
.security-tab:hover { color: var(--hnb-color-primary, #2f6fed); }
.security-tab.active { color: var(--hnb-color-primary, #2f6fed); font-weight: 600; }
.security-tab.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
.vuln-steps { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 24px; }
.vuln-step { display: flex; flex-direction: column; gap: 10px; }
.vuln-step__head { display: flex; align-items: center; gap: 10px; }
.vuln-step__index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--hnb-color-primary, #2f6fed);
  color: #fff;
  font-size: 12px;
  font-weight: 600;
}
.vuln-step__desc { margin: 0; font-size: 13px; color: var(--hnb-color-text-secondary, #5b6675); line-height: 1.6; }
.vuln-step__desc a { color: var(--hnb-color-primary, #2f6fed); }
.vuln-actions { display: flex; gap: 10px; }
.upload-success { color: var(--hnb-color-status-success, #12b76a); font-size: 13px; margin: 0; }
</style>
