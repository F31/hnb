import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import { setAgentOnboardingApiClient } from '../api/agentOnboardingApi'
import AgentOnboardingGuide from '../components/AgentOnboardingGuide.vue'
import { createTestI18n, stubApiClient } from './testUtils'

const wrappers: VueWrapper[] = []

function guidePayload(overrides: Record<string, unknown> = {}) {
  const token = typeof overrides.token === 'string' ? overrides.token : 'jwt-onboarding-token'
  return {
    clusterId: 'target-1',
    tenantId: 'tenant-a',
    displayName: 'Prod Cluster',
    tunnelUrl: 'wss://hnb.example.com/tunnel',
    token: 'jwt-onboarding-token',
    tokenExpiry: '2026-08-05T12:00:00Z',
    namespace: 'hnb-agent-tenant-a',
    // 与真实清单一致：令牌出现在 Secret 的 stringData.agent-token 中
    manifest: `apiVersion: v1\nkind: Secret\nstringData:\n  agent-token: "${token}"\n`,
    installCommand: `kubectl apply -f - <<'HNB_AGENT_EOF'\napiVersion: v1\nHNB_AGENT_EOF`,
    ...overrides,
  }
}

describe('AgentOnboardingGuide', () => {
  let client: ApiClient & { post: ReturnType<typeof vi.fn> }

  beforeEach(() => {
    const post = vi.fn(async <T,>() => guidePayload() as T)
    client = stubApiClient({ post: post as unknown as ApiClient['post'] }) as unknown as ApiClient & {
      post: ReturnType<typeof vi.fn>
    }
    setAgentOnboardingApiClient(client)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn(async () => {}) },
      configurable: true,
    })
  })

  afterEach(() => {
    wrappers.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  function mountGuide(props: { clusterId: string; clusterName?: string } = { clusterId: 'target-1' }) {
    const wrapper = mount(AgentOnboardingGuide, {
      attachTo: document.body,
      props,
      global: { plugins: [createTestI18n()] },
    })
    wrappers.push(wrapper)
    return wrapper
  }

  it('初始为收起态：仅标题与“生成接入指引”按钮，未展开不请求', async () => {
    mountGuide()
    await flushPromises()
    expect(document.querySelector('.agent-onboarding-panel')).toBeNull()
    const btn = Array.from(document.querySelectorAll<HTMLElement>('.hnb-button')).find((b) =>
      b.textContent?.includes('生成接入指引'),
    )
    expect(btn).toBeTruthy()
    expect(client.post).not.toHaveBeenCalled()
  })

  it('展开后请求 BFF 并渲染安装命令与完整清单，发出 onboarded', async () => {
    const wrapper = mountGuide()
    await flushPromises()
    const generateBtn = Array.from(document.querySelectorAll<HTMLElement>('.hnb-button')).find((b) =>
      b.textContent?.includes('生成接入指引'),
    )
    generateBtn!.click()
    await flushPromises()

    expect(client.post).toHaveBeenCalledTimes(1)
    const [path] = client.post.mock.calls[0] as [string]
    expect(String(path)).toContain('/agent-onboarding')

    const panel = document.querySelector('.agent-onboarding-panel')
    expect(panel).toBeTruthy()
    expect(panel!.textContent).toContain('wss://hnb.example.com/tunnel')
    expect(panel!.textContent).toContain('hnb-agent-tenant-a')
    expect(document.querySelector('.agent-onboarding-command-body')?.textContent).toContain('kubectl apply')
    expect(document.querySelector('.agent-onboarding-manifest-body')?.textContent).toContain('kind: Secret')
    expect(wrapper.emitted('onboarded')).toBeTruthy()
  })

  it('复制安装命令写入剪贴板并短暂显示“已复制”', async () => {
    const wrapper = mountGuide()
    await flushPromises()
    Array.from(document.querySelectorAll<HTMLElement>('.hnb-button'))
      .find((b) => b.textContent?.includes('生成接入指引'))!
      .click()
    await flushPromises()

    const copyBtns = Array.from(document.querySelectorAll<HTMLElement>('.agent-onboarding-command-head .hnb-button'))
    copyBtns[0].click()
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledTimes(1)
    const written = vi.mocked(navigator.clipboard.writeText).mock.calls[0][0] as string
    expect(written).toContain("kubectl apply -f - <<'HNB_AGENT_EOF'")
    expect(copyBtns[0].textContent).toContain('已复制')
  })

  it('接口失败展示错误与重试按钮，重试可恢复', async () => {
    client.post.mockRejectedValueOnce(new Error('agent onboarding not configured'))
    const wrapper = mountGuide()
    await flushPromises()
    Array.from(document.querySelectorAll<HTMLElement>('.hnb-button'))
      .find((b) => b.textContent?.includes('生成接入指引'))!
      .click()
    await flushPromises()

    expect(document.querySelector('.agent-onboarding-error')?.textContent).toContain('agent onboarding not configured')
    expect(document.querySelector('.agent-onboarding-manifest-body')).toBeNull()

    const retryBtn = Array.from(document.querySelectorAll<HTMLElement>('.agent-onboarding-error .hnb-button')).find((b) =>
      b.textContent?.includes('重试'),
    )
    retryBtn!.click()
    await flushPromises()
    expect(client.post).toHaveBeenCalledTimes(2)
    expect(document.querySelector('.agent-onboarding-manifest-body')).toBeTruthy()
  })

  it('“重新生成”轮换令牌：再次请求且展示新令牌', async () => {
    client.post
      .mockResolvedValueOnce(guidePayload({ token: 'token-v1' }) as never)
      .mockResolvedValueOnce(guidePayload({ token: 'token-v2' }) as never)
    mountGuide()
    await flushPromises()
    Array.from(document.querySelectorAll<HTMLElement>('.hnb-button'))
      .find((b) => b.textContent?.includes('生成接入指引'))!
      .click()
    await flushPromises()
    expect(document.querySelector('.agent-onboarding-manifest-body')!.textContent).toContain('token-v1')

    const regenBtn = Array.from(document.querySelectorAll<HTMLElement>('.agent-onboarding-foot .hnb-button')).find((b) =>
      b.textContent?.includes('重新生成'),
    )
    regenBtn!.click()
    await flushPromises()
    expect(client.post).toHaveBeenCalledTimes(2)
    expect(document.querySelector('.agent-onboarding-manifest-body')!.textContent).toContain('token-v2')
    // 轮换后提醒旧命令失效
    expect(document.querySelector('.agent-onboarding-warn')?.textContent).toContain('重新生成')
  })
})
