/**
 * 备份恢复 fixture（备份策略 / 备份任务 / 恢复任务 / 备份存储仓库）。
 * 仅构建时显式设置 VITE_CLUSTER_DETAIL_USE_FIXTURES=true 时启用。
 */
import type {
  BackupPolicy,
  BackupRepository,
  BackupTask,
  RestoreTask,
} from '../../types/backup'

export const backupPoliciesFixture: BackupPolicy[] = [
  { name: 'policy-prod-daily', cluster: 'graphify', status: 'enabled', backupMethod: '全量', nextBackupAt: '2026-08-08 02:00:00', createdAt: '2026-08-01 10:00:00' },
  { name: 'policy-vm-incremental', cluster: 'graphify', status: 'enabled', backupMethod: '增量', nextBackupAt: '2026-08-08 03:00:00', createdAt: '2026-08-02 11:30:00' },
  { name: 'policy-archive', cluster: 'graphify', status: 'disabled', backupMethod: '差异', nextBackupAt: '--', createdAt: '2026-07-20 09:00:00' },
]

export const backupTasksFixture: BackupTask[] = [
  { backupFileName: 'graphify-20260807-020000', sourceCluster: 'graphify', backupPolicy: 'policy-prod-daily', startTime: '2026-08-07 02:00:00', endTime: '2026-08-07 02:45:12', status: 'success', progress: 100, execType: 'scheduled' },
  { backupFileName: 'graphify-vm-20260807-030000', sourceCluster: 'graphify', backupPolicy: 'policy-vm-incremental', startTime: '2026-08-07 03:00:00', endTime: '', status: 'running', progress: 64, execType: 'scheduled' },
  { backupFileName: 'graphify-manual-20260806', sourceCluster: 'graphify', backupPolicy: 'policy-prod-daily', startTime: '2026-08-06 15:00:00', endTime: '2026-08-06 15:30:01', status: 'success', progress: 100, execType: 'manual' },
  { backupFileName: 'graphify-20260805-020000', sourceCluster: 'graphify', backupPolicy: 'policy-prod-daily', startTime: '2026-08-05 02:00:00', endTime: '2026-08-05 02:50:33', status: 'failed', progress: 78, execType: 'scheduled' },
]

export const restoreTasksFixture: RestoreTask[] = [
  { name: 'restore-20260806-prod', targetCluster: 'graphify', targetNamespace: 'default', restoredFile: 'graphify-20260806-manual', status: 'success', progress: 100, description: '恢复生产命名空间', createdAt: '2026-08-06 16:00:00' },
  { name: 'restore-20260807-dr', targetCluster: 'graphify', targetNamespace: 'dr', restoredFile: 'graphify-20260807-020000', status: 'running', progress: 42, description: '灾备演练恢复', createdAt: '2026-08-07 10:00:00' },
  { name: 'restore-20260805-vm', targetCluster: 'graphify', targetNamespace: 'vm-973fd1ef', restoredFile: 'graphify-vm-20260805', status: 'failed', progress: 30, description: '虚拟机数据恢复', createdAt: '2026-08-05 20:00:00' },
]

export const backupRepositoriesFixture: BackupRepository[] = [
  { name: 's3-backup-primary', cluster: 'graphify', accessUrl: 's3://backup.hnb.local', region: 'cn-east-1', bucket: 'hnb-backup', availability: 'available', forcePathStyle: true },
  { name: 'minio-dr', cluster: 'graphify', accessUrl: 'minio://dr.hnb.local:9000', region: 'cn-west-1', bucket: 'hnb-dr', availability: 'unavailable', forcePathStyle: true },
]
