/**
 * 集群状态字典（V2.5 §11.3）。
 *
 * 语义色由服务端字典治理，前端只引用本映射，不得自定义状态色。
 * 文案 Key 为 `resource.clusterMgmt.status.<CODE>`，默认 zh-CN。
 */
import type { StatusSemantic } from '@hnb/ui-kit'
import type { ClusterStatus } from '../types/cluster'

export const CLUSTER_STATUS_DICTIONARY: Record<
  ClusterStatus,
  { semantic: StatusSemantic; terminal: boolean }
> = {
	UNKNOWN: { semantic: 'default', terminal: false },
	REGISTERING: { semantic: 'processing', terminal: false },
	PROVISIONING: { semantic: 'processing', terminal: false },
	UPGRADING: { semantic: 'processing', terminal: false },
  RUNNING: { semantic: 'success', terminal: false },
  DEGRADED: { semantic: 'warning', terminal: false },
  STALE: { semantic: 'warning', terminal: false },
	FAILED: { semantic: 'error', terminal: false },
  DELETING: { semantic: 'processing', terminal: false },
  TERMINATED: { semantic: 'default', terminal: true },
}

export function clusterStatusMeta(status: ClusterStatus): { semantic: StatusSemantic; terminal: boolean } {
  return CLUSTER_STATUS_DICTIONARY[status] ?? { semantic: 'default', terminal: false }
}

/** 集群状态可执行写操作判定（STALE 允许提交，由服务端 STALE challenge 风险确认保护；终止态不可操作） */
export function canMutate(status: ClusterStatus): boolean {
  return status === 'RUNNING' || status === 'DEGRADED' || status === 'STALE'
}
