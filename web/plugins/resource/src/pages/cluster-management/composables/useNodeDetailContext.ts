/**
 * 节点详情上下文注入：向节点详情页签组件提供 nodeId 与节点名称。
 * NodeDetailPage（父）provide nodeId；NodeDetailLayout（子）读取 nodeId 并
 * provide nodeName；页签组件（slot 后代）消费两者。
 */
import { inject, provide, type InjectionKey } from 'vue'

export const NODE_DETAIL_ID_KEY: InjectionKey<string> = Symbol('resource-node-detail-id')
export const NODE_DETAIL_NAME_KEY: InjectionKey<string> = Symbol('resource-node-detail-name')

export function provideNodeDetailId(nodeId: string): void {
  provide(NODE_DETAIL_ID_KEY, nodeId)
}

export function useNodeDetailId(): string {
  const id = inject(NODE_DETAIL_ID_KEY, null)
  if (!id) throw new Error('node detail id is not provided')
  return id
}

export function provideNodeDetailName(name: string): void {
  provide(NODE_DETAIL_NAME_KEY, name)
}

export function useNodeDetailName(): string {
  return inject(NODE_DETAIL_NAME_KEY, '') ?? ''
}
