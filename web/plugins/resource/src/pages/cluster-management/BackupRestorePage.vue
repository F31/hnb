<script setup lang="ts">
/**
 * BackupRestorePage — 资源 > 备份恢复。
 * 四个页签：备份策略管理 / 备份任务管理 / 恢复任务管理 / 备份存储仓库。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BackupPoliciesTab from './components/BackupPoliciesTab.vue'
import BackupTasksTab from './components/BackupTasksTab.vue'
import RestoreTasksTab from './components/RestoreTasksTab.vue'
import BackupRepositoriesTab from './components/BackupRepositoriesTab.vue'

const { t } = useI18n()
const activeTab = ref('policies')

const tabs = [
  { key: 'policies', label: 'backupPolicies' },
  { key: 'tasks', label: 'backupTasks' },
  { key: 'restores', label: 'restoreTasks' },
  { key: 'repositories', label: 'repositories' },
]
</script>

<template>
  <div class="backup-restore-page">
    <header class="page-header">
      <h2 class="page-title">{{ t('resource.clusterMgmt.backup.title') }}</h2>
    </header>

    <nav class="backup-tabs" role="tablist" :aria-label="t('resource.clusterMgmt.backup.title')">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="backup-tab"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        {{ t(`resource.clusterMgmt.backup.tab.${tab.label}`) }}
      </button>
    </nav>

    <BackupPoliciesTab v-if="activeTab === 'policies'" />
    <BackupTasksTab v-else-if="activeTab === 'tasks'" />
    <RestoreTasksTab v-else-if="activeTab === 'restores'" />
    <BackupRepositoriesTab v-else />
  </div>
</template>

<style scoped>
.backup-restore-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: var(--hnb-color-bg-surface, #fff);
  border: 1px solid var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-md, 6px);
  padding: 18px 20px;
}
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: 18px; font-weight: 600; color: var(--hnb-color-text-primary, #12172a); }
.backup-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--hnb-color-border, #e2e7ef);
  flex-wrap: wrap;
}
.backup-tab {
  position: relative;
  padding: 8px 16px;
  border: 0;
  background: transparent;
  color: var(--hnb-color-text-secondary, #5b6675);
  font-size: 14px;
  cursor: pointer;
}
.backup-tab:hover { color: var(--hnb-color-primary, #2f6fed); }
.backup-tab.active { color: var(--hnb-color-primary, #2f6fed); font-weight: 600; }
.backup-tab.active::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: -1px;
  height: 2px;
  background: var(--hnb-color-primary, #2f6fed);
}
</style>
