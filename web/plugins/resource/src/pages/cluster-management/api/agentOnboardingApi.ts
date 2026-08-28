/**
 * cluster-agent 接入指引数据访问层（Resource 插件）。
 *
 * 闭环说明：导入/纳管 Kubernetes 集群后，"接入集群" 的最终环节是把
 * cluster-agent 部署到目标集群（对齐 KubeSphere 成员集群 Agent 下发与
 * Rancher 集群注册命令）。浏览器无法直连目标集群，因此由 apiserver 在
 * 校验调用者对目标有权后签发绑定 (tenant, cluster) 的 agent-tunnel 令牌，
 * 并渲染可 kubectl apply 的部署清单；本模块仅消费该 BFF 端点的 JSON 结果，
 * 令牌与清单在 UI 中以“复制到剪贴板”方式交给管理员在目标集群执行。
 *
 * 安全约束：
 *  - 全程携带平台认证上下文头（统一走 @hnb/api-client）；
 *  - 不在此模块持久化令牌/清单，页面卸载即丢弃；
 *  - 端点本身按 ResourceCluster/ActionRead 鉴权，跨租户一律 404。
 */

import type { ApiClient } from '@hnb/types'

const AGENT_ONBOARDING_PATH = '/api/v1/resources/clusters'

export interface AgentOnboardingResponse {
  clusterId: string
  tenantId: string
  displayName: string
  tunnelUrl: string
  token: string
  tokenExpiry: string
  namespace: string
  manifest: string
  installCommand: string
}

let apiClient: ApiClient | null = null

export function setAgentOnboardingApiClient(client: ApiClient): void {
  apiClient = client
}

function client(): ApiClient {
  if (!apiClient) throw new Error('agent onboarding api client is not initialized')
  return apiClient
}

/** 请求 cluster-agent 接入指引（BFF 服务端签发令牌并渲染清单） */
export async function getAgentOnboarding(clusterId: string): Promise<AgentOnboardingResponse> {
  return client().post<AgentOnboardingResponse>(
    `${AGENT_ONBOARDING_PATH}/${encodeURIComponent(clusterId)}/agent-onboarding`,
  )
}
