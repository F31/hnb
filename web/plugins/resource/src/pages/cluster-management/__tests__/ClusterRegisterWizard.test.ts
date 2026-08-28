import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiClient } from '@hnb/types'
import { setClusterApiClient, setClusterContextStore } from '../api/clusterApi'
import { setOperationApiClient } from '../api/operationApi'
import ClusterRegisterWizard from '../components/ClusterRegisterWizard.vue'
import { createTestI18n, stubApiClient, stubContextStore } from './testUtils'

const wrappers: VueWrapper[] = []

describe('ClusterRegisterWizard (9.4/9.5/9.6)', () => {
  let apiClient: ApiClient & { post: ReturnType<typeof vi.fn>; get: ReturnType<typeof vi.fn> }

  beforeEach(() => {
    const postMock = vi.fn(async <T,>() => ({
      intentId: 'intent-1',
      status: 'accepted',
      semanticDigest: 'd',
      intent: {},
      operationId: 'op-1',
      createdAt: '2026-08-01T00:00:00Z',
    }) as T)
    const getMock = vi.fn(async <T,>(_url: string) => ({
      data: {
        operationId: 'op-1',
        intentId: 'intent-1',
        type: 'create-cluster',
        status: 'in_progress',
        targetId: 'target-1',
        targetKind: 'KubernetesTarget',
        progress: { completedSteps: 1, totalSteps: 3, percent: 33 },
        correlationId: 'c',
        createdAt: '2026-08-01T00:00:00Z',
        updatedAt: '2026-08-01T00:00:00Z',
        executionPlanId: 'plan-1',
        steps: [
          { stepId: 's1', name: 'validate', status: 'succeeded', attempt: 1 },
          { stepId: 's2', name: 'provision', status: 'in_progress', attempt: 1 },
          { stepId: 's3', name: 'observe', status: 'pending', attempt: 0 },
        ],
        allowedActions: [],
        links: { operation: '/resource/operations/op-1' },
      },
    }) as T)
    apiClient = stubApiClient({
      post: postMock as unknown as ApiClient['post'],
      get: getMock as unknown as ApiClient['get'],
    }) as unknown as ApiClient & { post: ReturnType<typeof vi.fn>; get: ReturnType<typeof vi.fn> }
    setClusterApiClient(apiClient)
    setOperationApiClient(apiClient)
    setClusterContextStore(stubContextStore())
  })

  afterEach(() => {
    vi.restoreAllMocks()
    wrappers.splice(0).forEach(w => w.unmount())
    document.body.innerHTML = ''
  })

  function query(sel: string): HTMLElement {
    const el = document.querySelector<HTMLElement>(sel)
    if (!el) throw new Error(`Element not found: ${sel} — body: ${document.body.innerHTML.substring(0, 500)}`)
    return el
  }

  function queryAll(sel: string): NodeListOf<HTMLElement> {
    return document.querySelectorAll<HTMLElement>(sel)
  }

  async function mountToConfirm() {
    const wrapper = mount(ClusterRegisterWizard, {
      attachTo: document.body,
      props: { modelValue: true },
      global: { plugins: [createTestI18n()] },
    })
    wrappers.push(wrapper)
    await flushPromises()
    const input = query('.form-grid input') as HTMLInputElement
    input.value = 'cluster-a'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    // stepOneValid 要求凭据非空（kubeconfig / cloudcore-client），凭据在第一步行填写
    const kubeInput = query('.kubeconfig-input') as HTMLTextAreaElement
    kubeInput.value = 'apiVersion: v1\nkind: Config\nclusters: []\nusers: []\n'
    kubeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()
    return wrapper
  }

  it('提交经 RuntimeIntent 走共享鉴权客户端，202 不表示为成功', async () => {
    const wrapper = await mountToConfirm()
    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()

    // 首次 POST 为敏感凭据注册，意图只引用 SecretReference，不携带明文
    const [secretPath] = apiClient.post.mock.calls[0] as [string, unknown]
    expect(String(secretPath)).toContain('/api/v1/secrets:register')
    const intentCall = apiClient.post.mock.calls.find(([p]) => String(p).includes('/api/v1/runtime-intents'))
    expect(intentCall).toBeTruthy()
    const [path, payload] = intentCall! as [string, { kind: string; spec?: { credentialSecretRef?: unknown; kubeconfig?: unknown } }]
    expect(String(path)).toContain('/api/v1/runtime-intents')
    expect(payload.kind).toBe('CreateKubernetesTarget')
    expect(payload.spec?.credentialSecretRef).toBeTruthy()
    expect(payload.spec?.kubeconfig).toBeUndefined()
    const submitted = wrapper.emitted('submitted')
    expect(submitted).toBeTruthy()
    expect(submitted![0][0]).toMatchObject({ status: 'accepted', operationId: 'op-1' })
  })

  it('submitting 期间按钮禁用（防重双击）', async () => {
    let resolveSubmit: (v: unknown) => void = () => {}
    apiClient.post.mockImplementationOnce(
      () => new Promise((resolve) => { resolveSubmit = resolve }),
    )
    const wrapper = await mountToConfirm()
    const submitBtn = query('.cluster-register-drawer__footer-right .hnb-button--primary') as HTMLButtonElement
    submitBtn.click()
    await flushPromises()

    expect(submitBtn.disabled).toBe(true)

    resolveSubmit({
      intentId: 'intent-1',
      status: 'accepted',
      semanticDigest: 'd',
      intent: {},
      operationId: 'op-1',
      createdAt: '2026-08-01T00:00:00Z',
    })
    await flushPromises()
  })

  it('提交失败后恢复可重试并展示错误，不丢失表单', async () => {
    const secretReg = { provider: 'kms', scope: 'tenant:default', name: 'cluster-a-credential', version: '1' }
    apiClient.post
      .mockRejectedValueOnce(new Error('upstream unavailable'))
      .mockResolvedValueOnce(secretReg)
      .mockResolvedValueOnce({
        intentId: 'intent-2',
        status: 'accepted',
        semanticDigest: 'd2',
        intent: {},
        operationId: 'op-2',
        createdAt: '2026-08-01T00:00:00Z',
      })
    const wrapper = await mountToConfirm()

    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()
    expect(document.querySelector('.cluster-register-submit-error')?.textContent).toContain('upstream unavailable')

    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()
    const submitted = wrapper.emitted('submitted')
    expect(submitted).toBeTruthy()
    expect(submitted![0][0]).toMatchObject({ operationId: 'op-2' })
    // 第一次提交（凭据注册）失败 + 第二次提交（凭据注册 + 意图）
    expect(apiClient.post).toHaveBeenCalledTimes(3)
  })

  it('导入 Edge 走 ImportRuntimeTarget 且凭据仅 SecretReference 引用', async () => {
    const wrapper = mount(ClusterRegisterWizard, {
      attachTo: document.body,
      props: { modelValue: true },
      global: { plugins: [createTestI18n()] },
    })
    wrappers.push(wrapper)
    await flushPromises()

    const input = query('.form-grid input') as HTMLInputElement
    input.value = 'edge-cluster'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    const importButton = Array.from(queryAll('.source-toggle button')).find(b => b.textContent?.includes('纳管已有集群'))
    importButton!.click()
    await flushPromises()

    const kindSelect = query('.form-grid select') as HTMLSelectElement
    kindSelect.value = 'edge'
    kindSelect.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()

    const cloudcoreInput = query('input[placeholder*="wss://"]') as HTMLInputElement
    cloudcoreInput.value = 'wss://cloudcore:10002'
    cloudcoreInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    const nodeGroupInput = query('input[placeholder*="edge-group"]') as HTMLInputElement
    nodeGroupInput.value = 'group-1'
    nodeGroupInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    // 凭据（cloudcore-client）在第一步行填写
    const kubeInput = query('.kubeconfig-input') as HTMLTextAreaElement
    kubeInput.value = 'endpoint: wss://cloudcore:10002\ntoken: test-token\n'
    kubeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()

    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()
    const [secretPath, secretPayload] = apiClient.post.mock.calls[0] as [string, { purpose?: string }]
    expect(String(secretPath)).toContain('/api/v1/secrets:register')
    expect(secretPayload.purpose).toBe('cloudcore-client')
    const intentCall = apiClient.post.mock.calls.find(([p]) => String(p).includes('/api/v1/runtime-intents'))
    expect(intentCall).toBeTruthy()
    const [path, payload] = intentCall! as [string, { kind: string; spec?: { secretReferences?: unknown[] } }]
    expect(String(path)).toContain('/api/v1/runtime-intents')
    expect(payload.kind).toBe('ImportRuntimeTarget')
    expect(payload.spec?.secretReferences ?? []).toHaveLength(0)
  })

  const VALID_KUBECONFIG = `apiVersion: v1
kind: Config
current-context: prod-ctx
clusters:
  - name: prod-cluster
    cluster:
      server: https://10.0.0.10:6443
contexts:
  - name: prod-ctx
    context:
      cluster: prod-cluster
      user: admin
users:
  - name: admin
    user:
      token: secret-token
`

  it('提交前预检：展示将接入的目标（集群名 + API Server），且不泄露凭据明文', async () => {
    const wrapper = mount(ClusterRegisterWizard, {
      attachTo: document.body,
      props: { modelValue: true },
      global: { plugins: [createTestI18n()] },
    })
    wrappers.push(wrapper)
    await flushPromises()

    const kubeInput = query('.kubeconfig-input') as HTMLTextAreaElement
    kubeInput.value = VALID_KUBECONFIG
    kubeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    const value = query('.preflight-value')
    const url = query('.preflight-url')
    expect(value.textContent).toContain('prod-cluster')
    expect(url.textContent).toContain('https://10.0.0.10:6443')
    // 不得把 token 明文渲染到预检面板
    expect(document.querySelector('.preflight-panel')?.textContent).not.toContain('secret-token')
  })

  it('提交后进入内联 Operation 进度视图（闭环），展示操作 ID 并可前往跟踪', async () => {
    const wrapper = await mountToConfirm()
    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()

    // 提交成功即切换为内联进度视图（即使 Operation 尚未返回轮询数据）
    const progress = document.querySelector('.cluster-register-step-progress')
    expect(progress).toBeTruthy()
    const opId = document.querySelector('.operation-id')
    expect(opId?.textContent).toContain('op-1')
    // 提供“前往跟踪”入口，形成提交 → 跟踪闭环
    const track = Array.from(document.querySelectorAll('.cluster-register-drawer__footer-right .hnb-button--secondary'))
      .find(b => b.textContent?.includes('跟踪') || b.textContent?.includes('Track'))
    expect(track).toBeTruthy()
  })

  it('导入 Kubernetes 集群且 Operation 成功后，展示 cluster-agent 接入指引（闭环最终环节）', async () => {
    // 轮询返回成功终态 + targetId（导入操作落库的 RuntimeTarget ID）
    apiClient.get.mockImplementation(async <T,>(_url: string) => ({
      data: {
        operationId: 'op-1',
        intentId: 'intent-1',
        type: 'import-cluster',
        status: 'succeeded',
        targetId: 'target-1',
        targetKind: 'KubernetesTarget',
        progress: { completedSteps: 2, totalSteps: 2, percent: 100 },
        correlationId: 'c',
        createdAt: '2026-08-01T00:00:00Z',
        updatedAt: '2026-08-01T00:00:10Z',
        completedAt: '2026-08-01T00:00:10Z',
        executionPlanId: 'plan-1',
        steps: [{ stepId: 's1', name: 'observe', status: 'succeeded', attempt: 1 }],
        allowedActions: [],
        links: { operation: '/resource/operations/op-1' },
      },
    }) as T)

    const wrapper = mount(ClusterRegisterWizard, {
      attachTo: document.body,
      props: { modelValue: true },
      global: { plugins: [createTestI18n()] },
    })
    wrappers.push(wrapper)
    await flushPromises()

    const input = query('.form-grid input') as HTMLInputElement
    input.value = 'imported-k8s'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    const importButton = Array.from(queryAll('.source-toggle button')).find(b => b.textContent?.includes('纳管已有集群'))
    importButton!.click()
    await flushPromises()

    const kubeInput = query('.kubeconfig-input') as HTMLTextAreaElement
    kubeInput.value = VALID_KUBECONFIG
    kubeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()

    // 下一步 → 提交
    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()
    query('.cluster-register-drawer__footer-right .hnb-button--primary').click()
    await flushPromises()

    // 共享轮询器首轮 2s（±20%）：等待其拉取到 succeeded 终态
    await new Promise((r) => setTimeout(r, 2600))

    expect(document.querySelector('.cluster-register-step-progress')).toBeTruthy()
    const block = document.querySelector('.agent-onboarding-wizard-block')
    expect(block).toBeTruthy()
    // 指引默认收起：不立即请求 onboarding 端点（仅凭据注册 + 意图 2 次 POST）
    expect(apiClient.post).toHaveBeenCalledTimes(2)
  })
})