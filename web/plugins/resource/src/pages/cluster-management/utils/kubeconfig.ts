/**
 * kubeconfig 预检解析（纯前端，只读）。
 *
 * KubeSphere / Rancher 在提交接入前会解析 kubeconfig，向用户确认“将接入哪个
 * 集群 / 哪个 API server”，降低接错目标的风险。这里复用 `yaml` 包做一次性解析，
 * 仅提取展示所需的非敏感信息：
 *  - current-context 与所选 cluster 的 server URL；
 *  - 断言它是合法的 kubeconfig 结构（clusters/users/contexts 存在），
 *    但不校验网络连通性或服务端可达性（那属于服务端预检，本前端不直连目标）。
 *
 * 安全约束：绝不提取或返回 client-certificate-data / client-key-data / token /
 * certificate-authority-data 等敏感字段。解析失败或字段缺失时返回空摘要，调用方
 * 以“未解析到目标信息”兜底，不能把解析异常当成凭据校验通过。
 */

import { parseDocument } from 'yaml'

interface RawKubeconfig {
  'current-context'?: unknown
  clusters?: Array<{
    name?: unknown
    cluster?: {
      server?: unknown
      'certificate-authority-data'?: unknown
      'insecure-skip-tls-verify'?: unknown
    }
  }>
  contexts?: Array<{
    name?: unknown
    context?: {
      cluster?: unknown
      user?: unknown
    }
  }>
  users?: Array<{ name?: unknown; user?: unknown }>
}

export interface KubeClusterInfo {
  name: string
  server: string
}

export interface KubeContextInfo {
  name: string
  cluster: string
}

export interface KubeSummary {
  /** 是否解析出可展示的目标信息（至少一个 cluster 且存在 server） */
  recognizable: boolean
  /** 是否具备一个有效 kubeconfig 的最小结构（clusters/contexts/users） */
  structurallyValid: boolean
  currentContext: string
  clusters: KubeClusterInfo[]
  /** current-context 命中的 cluster；无 current-context 时取第一个 */
  targetCluster?: KubeClusterInfo
  errors: string[]
}

function asString(v: unknown): string {
  return typeof v === 'string' ? v.trim() : ''
}

/**
 * 解析 kubeconfig 文本为非敏感摘要。
 * @param text kubeconfig 原文（yaml）
 */
export function parseKubeSummary(text: string): KubeSummary {
  const empty: KubeSummary = {
    recognizable: false,
    structurallyValid: false,
    currentContext: '',
    clusters: [],
    errors: [],
  }
  if (!text.trim()) return empty

  let doc: unknown
  try {
    const parsed = parseDocument(text)
    if (parsed.errors.length > 0) {
      // 结构化解析失败：不当作“可识别”，返回错误摘要（不展开敏感内容）。
      const first = parsed.errors[0]
      return { ...empty, errors: [first?.message ?? 'yaml-parse-error'] }
    }
    doc = parsed.toJS()
  } catch (err) {
    return { ...empty, errors: [err instanceof Error ? err.message : String(err)] }
  }

  if (!doc || typeof doc !== 'object') {
    return { ...empty, errors: ['not-an-object'] }
  }

  const raw = doc as RawKubeconfig
  const errors: string[] = []
  let structurallyValid = true

  const clusters = Array.isArray(raw.clusters) ? raw.clusters : []
  const contexts = Array.isArray(raw.contexts) ? raw.contexts : []
  const users = Array.isArray(raw.users) ? raw.users : []
  if (!clusters.length) { structurallyValid = false; errors.push('missing-clusters') }
  if (!contexts.length) { structurallyValid = false; errors.push('missing-contexts') }
  if (!users.length) { structurallyValid = false; errors.push('missing-users') }

  const clusterInfos: KubeClusterInfo[] = clusters
    .map((c) => ({
      name: asString(c?.name),
      server: asString(c?.cluster?.server),
    }))
    .filter((c) => c.name && c.server)

  // 上下文里可能引用未在 clusters 段声明的名字，但入站通常齐全；此处只反映已声明对象。
  const contextInfos: KubeContextInfo[] = contexts
    .map((c) => ({ name: asString(c?.name), cluster: asString(c?.context?.cluster) }))
    .filter((c) => c.name && c.cluster)

  const currentContext = asString(raw['current-context'])
  let targetCluster: KubeClusterInfo | undefined
  if (currentContext && contextInfos.length) {
    const ctx = contextInfos.find((c) => c.name === currentContext)
    const clusterName = ctx?.cluster
    if (clusterName) {
      targetCluster = clusterInfos.find((c) => c.name === clusterName)
    }
  }
  if (!targetCluster) {
    targetCluster = clusterInfos[0]
  }

  return {
    recognizable: clusterInfos.length > 0 && !!targetCluster,
    structurallyValid,
    currentContext,
    clusters: clusterInfos,
    targetCluster,
    errors,
  }
}
