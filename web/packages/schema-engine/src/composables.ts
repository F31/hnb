/**
 * Schema Engine 数据源 composable + InjectionKey。
 *
 * 在 PageRenderer 渲染时，runtime（clusterRuntime）通过
 * `useProvideDataSourceManager()` 注入 DataSourceManager，集群自定义组件
 * 通过 `useDataSourceManager()` 取回。组件再调用 `fetchPaginated(...)`
 * 即可走受信端点 + 自动 generation/缓存键隔离。
 */
import { inject, provide, type InjectionKey } from 'vue'
import type { DataSourceManager } from './DataSourceManager'
import type { DataQuery } from './DataSourceManager'
import type { PaginatedResult } from './types'

export const DATA_SOURCE_MANAGER_KEY: InjectionKey<DataSourceManager> =
  Symbol('schema-engine-data-source-manager')
export function provideDataSourceManager(ds: DataSourceManager): void {
  provide(DATA_SOURCE_MANAGER_KEY, ds)
}

export function useDataSourceManager(): DataSourceManager | null {
  return inject(DATA_SOURCE_MANAGER_KEY, null)
}

/**
 * 数据源消费工具：返回指定 dataSourceId 的 fetch helpers。
 * 传入 contextKey（来自 tenant/space）确保缓存键隔离。
 */
export function useDataSource(dataSourceId: string): {
  fetch: <T>(query?: DataQuery) => Promise<T>
  fetchPaginated: <T>(query?: DataQuery) => Promise<PaginatedResult<T>>
} {
  const ds = useDataSourceManager()
  if (!ds) throw new Error('DataSourceManager is not provided')
  if (!ds.has(dataSourceId)) {
    throw new Error(`Unknown dataSource "${dataSourceId}"`)
  }
  return {
    fetch: <T>(query: DataQuery = {}) => ds.fetch<T>(dataSourceId, query),
    fetchPaginated: <T>(query: DataQuery = {}) =>
      ds.fetchPaginated<T>(dataSourceId, query),
  }
}
