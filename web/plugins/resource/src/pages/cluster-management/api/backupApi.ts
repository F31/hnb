/**
 * 备份恢复 service adapter。
 * 页面只依赖 typed 函数；开发 fixture（含创建/删除等写操作的内存变更），生产空态。
 */
import type {
  BackupPolicy,
  BackupRepository,
  BackupTask,
  RestoreTask,
} from '../types/backup'
import {
  backupPoliciesFixture,
  backupRepositoriesFixture,
  backupTasksFixture,
  restoreTasksFixture,
} from './fixtures/backup'
import { pluginT } from './pluginI18n'

const USE_FIXTURES = import.meta.env.VITE_CLUSTER_DETAIL_USE_FIXTURES === 'true'

// ---------------------------------------------------------------------------
// 备份策略
// ---------------------------------------------------------------------------

export async function getBackupPolicies(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<BackupPolicy[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return backupPoliciesFixture
  return backupPoliciesFixture.filter((p) => p.name.toLowerCase().includes(kw))
}

export async function createBackupPolicy(_clusterId: string, payload: BackupPolicy): Promise<void> {
  if (USE_FIXTURES) {
    backupPoliciesFixture.push(payload)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}

export async function executeBackup(_clusterId: string, _policyName: string): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}

export async function updateBackupPolicy(
  _clusterId: string,
  _policyName: string,
  _payload: BackupPolicy,
): Promise<void> {
  if (USE_FIXTURES) return
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}

export async function deleteBackupPolicy(_clusterId: string, policyName: string): Promise<void> {
  if (USE_FIXTURES) {
    const idx = backupPoliciesFixture.findIndex((p) => p.name === policyName)
    if (idx >= 0) backupPoliciesFixture.splice(idx, 1)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}

// ---------------------------------------------------------------------------
// 备份任务
// ---------------------------------------------------------------------------

export async function getBackupTasks(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<BackupTask[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return backupTasksFixture
  return backupTasksFixture.filter((t) => t.backupFileName.toLowerCase().includes(kw))
}

// ---------------------------------------------------------------------------
// 恢复任务
// ---------------------------------------------------------------------------

export async function getRestoreTasks(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<RestoreTask[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return restoreTasksFixture
  return restoreTasksFixture.filter((t) => t.name.toLowerCase().includes(kw))
}

export async function createRestoreTask(_clusterId: string, payload: RestoreTask): Promise<void> {
  if (USE_FIXTURES) {
    restoreTasksFixture.push(payload)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}

// ---------------------------------------------------------------------------
// 备份存储仓库
// ---------------------------------------------------------------------------

export async function getBackupRepositories(
  _clusterId: string,
  params: { keyword?: string } = {},
): Promise<BackupRepository[]> {
  if (!USE_FIXTURES) return []
  const kw = params.keyword?.trim().toLowerCase() ?? ''
  if (!kw) return backupRepositoriesFixture
  return backupRepositoriesFixture.filter((r) => r.name.toLowerCase().includes(kw))
}

export async function createBackupRepository(
  _clusterId: string,
  payload: BackupRepository,
): Promise<void> {
  if (USE_FIXTURES) {
    backupRepositoriesFixture.push(payload)
    return
  }
  throw new Error(pluginT('resource.clusterMgmt.error.backupUnavailable'))
}
