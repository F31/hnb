/**
 * 备份恢复领域类型（备份策略 / 备份任务 / 恢复任务 / 备份存储仓库）。
 * 后端端点暂缺，由 service adapter + fixture 提供（生产空态）。
 */

/** 备份策略 */
export interface BackupPolicy {
  name: string
  cluster: string
  status: 'enabled' | 'disabled' | 'unknown'
  backupMethod: string
  nextBackupAt: string
  createdAt: string
}

/** 备份任务 */
export interface BackupTask {
  backupFileName: string
  sourceCluster: string
  backupPolicy: string
  startTime: string
  endTime: string
  status: 'running' | 'success' | 'failed' | 'pending'
  progress: number
  execType: 'scheduled' | 'manual'
}

/** 恢复任务 */
export interface RestoreTask {
  name: string
  targetCluster: string
  targetNamespace: string
  restoredFile: string
  status: 'running' | 'success' | 'failed' | 'pending'
  progress: number
  description?: string
  createdAt: string
}

/** 备份存储仓库 */
export interface BackupRepository {
  name: string
  cluster: string
  accessUrl: string
  region: string
  bucket: string
  availability: 'available' | 'unavailable'
  forcePathStyle: boolean
}
