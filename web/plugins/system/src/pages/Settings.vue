<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBPageShell, HNBButton, HNBFormField } from '@hnb/ui-kit'
import * as api from '../systemApi'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const success = ref('')
const notifyEmail = ref('')
const platformName = ref('')
const sessionTimeout = ref(3600)
const maintenanceMode = ref(false)

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.apiGet<Record<string, any>>('/api/v1/settings')
    platformName.value = data?.['platform.name'] || ''
    notifyEmail.value = data?.['notification.email'] || ''
    sessionTimeout.value = data?.['security.session_timeout'] || 3600
    maintenanceMode.value = data?.['platform.maintenance_mode'] || false
  } catch (e: any) {
    error.value = e?.message || t('system.settings.loadError')
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  saving.value = true
  error.value = ''
  success.value = ''
  try {
    await api.apiPut('/api/v1/settings', {
      'platform.name': platformName.value,
      'notification.email': notifyEmail.value,
      'security.session_timeout': sessionTimeout.value,
      'platform.maintenance_mode': maintenanceMode.value,
    })
    success.value = t('system.settings.saveSuccess')
  } catch (e: any) {
    error.value = e?.message || t('system.settings.saveError')
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>
<template>
  <HNBPageShell :title="t('system.settings.title')" :description="t('system.settings.desc')">
    <div class="tcc__cards">
      <section class="tcc__card">
        <div class="tcc__card-title-row">
          <span class="tcc__card-accent" />
          <h2 class="tcc__card-title">{{ t('system.settings.general') }}</h2>
        </div>
        <div v-if="loading" class="tcc__status">{{ t('system.common.loading') }}</div>
        <div v-else class="tcc__form">
          <HNBFormField :label="t('system.settings.platformName')" input-id="s-platform-name">
            <input id="s-platform-name" v-model="platformName" class="tcc__input" :placeholder="t('system.settings.platformNamePlaceholder')" />
          </HNBFormField>
          <HNBFormField :label="t('system.settings.notifyEmail')" input-id="s-notify-email">
            <input id="s-notify-email" v-model="notifyEmail" type="email" class="tcc__input" :placeholder="t('system.settings.notifyEmailPlaceholder')" />
          </HNBFormField>
          <HNBFormField :label="t('system.settings.sessionTimeout')" input-id="s-session-timeout">
            <input id="s-session-timeout" v-model.number="sessionTimeout" type="number" class="tcc__input" min="60" />
          </HNBFormField>
          <HNBFormField :label="t('system.settings.maintenanceMode')" input-id="s-maintenance-mode">
            <label class="tcc__toggle">
              <input id="s-maintenance-mode" v-model="maintenanceMode" type="checkbox" class="tcc__checkbox" />
              <span class="tcc__toggle-label">{{ maintenanceMode ? t('system.settings.maintenanceOn') : t('system.settings.maintenanceOff') }}</span>
            </label>
          </HNBFormField>
          <p v-if="error" class="tcc__error">{{ error }}</p>
          <p v-if="success" class="tcc__success">{{ success }}</p>
          <div class="tcc__actions">
            <HNBButton variant="primary" :loading="saving" @click="saveSettings">{{ t('system.settings.save') }}</HNBButton>
          </div>
        </div>
      </section>
    </div>
  </HNBPageShell>
</template>
<style scoped>
.tcc__cards { display:flex;flex-direction:column;gap:20px; }
.tcc__card { background:var(--hnb-color-bg-surface);border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-lg);padding:20px; }
.tcc__card-title-row { display:flex;align-items:center;gap:10px;margin-bottom:16px; }
.tcc__card-accent { width:3px;height:18px;background:var(--hnb-color-primary);border-radius:2px;flex-shrink:0; }
.tcc__card-title { margin:0;font-size:15px;font-weight:600;color:var(--hnb-color-text-primary); }
.tcc__form { display:flex;flex-direction:column;gap:14px;max-width:480px; }
.tcc__input { width:100%;height:34px;padding:0 12px;border:1px solid var(--hnb-color-border);border-radius:var(--hnb-radius-md);background:var(--hnb-color-bg);color:var(--hnb-color-text-primary);font-size:13px;box-sizing:border-box; }
.tcc__input:focus { outline:none;border-color:var(--hnb-color-primary); }
.tcc__toggle { display:flex;align-items:center;gap:8px;cursor:pointer; }
.tcc__checkbox { width:16px;height:16px;accent-color:var(--hnb-color-primary); }
.tcc__toggle-label { font-size:13px;color:var(--hnb-color-text-secondary); }
.tcc__actions { display:flex;gap:8px;margin-top:4px; }
.tcc__error { margin:0;padding:8px 12px;border-radius:var(--hnb-radius-md);background:rgba(240,68,56,0.12);color:var(--hnb-color-status-danger, #f04438);font-size:13px; }
.tcc__success { margin:0;padding:8px 12px;border-radius:var(--hnb-radius-md);background:rgba(0,200,83,0.12);color:var(--hnb-color-status-success, #12b76a);font-size:13px; }
.tcc__status { color:var(--hnb-color-text-tertiary);font-size:14px;padding:16px 0; }
</style>