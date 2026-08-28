/**
 * 集群详情上下文注入：向集群详情控制台的注册组件提供当前 clusterId。
 * 由 ClusterDetailLayout / 页面组件 provide，注册组件通过 useClusterDetailId 消费。
 */
import { inject, provide, type InjectionKey } from 'vue'

export const CLUSTER_DETAIL_ID_KEY: InjectionKey<string> = Symbol('resource-cluster-detail-id')

export function provideClusterDetailId(clusterId: string): void {
  provide(CLUSTER_DETAIL_ID_KEY, clusterId)
}

export function useClusterDetailId(): string {
  const id = inject(CLUSTER_DETAIL_ID_KEY, null)
  if (!id) throw new Error('cluster detail id is not provided')
  return id
}

/** 从 window.location 或路由参数解析 clusterId（兼容深链接刷新） */
export function clusterIdFromRoutePath(): string {
  if (typeof window === 'undefined') return ''
  const match = window.location.pathname.match(/^\/resource\/clusters\/([^/]+)/)
  return match?.[1] ? decodeURIComponent(match[1]) : ''
}
