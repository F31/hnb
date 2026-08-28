<script setup lang="ts">
/**
 * cluster-agent 接入指引（闭环最终环节）。
 *
 * 对齐 KubeSphere 成员集群 Agent 下发与 Rancher 集群注册：导入 Kubernetes
 * 集群后，管理员需要把 cluster-agent 部署到目标集群，agent 才能经隧道回连
 * 平台、上报观测，从而把集群推进到 RUNNING/已连接。浏览器无法直连目标集群，
 * 因此本组件调用 BFF 端点（服务端校权后签发绑定令牌并渲染清单），把
 * kubectl 安装命令与完整清单呈现给管理员复制、在目标集群执行。
 *
 * 安全约束：
 *  - 令牌/清单通过本插件 API 层获取，页面卸载即丢弃，不持久化；
 *  - 令牌含明文承载，渲染用 <pre>/textarea，不写入任何日志；
 *  - “重新生成”会轮换令牌，UI 明确提示旧令牌随之失效。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { HNBButton } from '@hnb/ui-kit'
import { getAgentOnboarding } from '../api/agentOnboardingApi'
import type { AgentOnboardingResponse } from '../api/agentOnboardingApi'

const props = defineProps<{
  clusterId: string
  clusterName?: string
}>()

const emit = defineEmits<{
  onboarded: []
}>()

const { t } = useI18n()

const expanded = ref(false)
const loading = ref(false)
const error = ref('')
const guide = ref<AgentOnboardingResponse | null>(null)
const copied = ref<'install' | 'manifest' | null>(null)

async function load(): Promise<void> {
  error.value = ''
  loading.value = true
  copied.value = null
  try {
    guide.value = await getAgentOnboarding(props.clusterId)
    emit('onboarded')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    guide.value = null
  } finally {
    loading.value = false
  }
}

function toggle(): void {
  if (expanded.value) {
    expanded.value = false
    return
  }
  expanded.value = true
  if (!guide.value) void load()
}

async function regenerate(): Promise<void> {
  await load()
}

function formatExpiry(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

async function copyToClipboard(text: string, kind: 'install' | 'manifest'): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      // 旧浏览器回退：临时 textarea + execCommand
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    copied.value = kind
    window.setTimeout(() => {
      if (copied.value === kind) copied.value = null
    }, 2000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}
</script>

<template>
  <section class="agent-onboarding-guide" aria-label="cluster agent 接入指引">
    <div class="agent-onboarding-head">
      <div class="agent-onboarding-title">
        <h4>{{ t('resource.clusterMgmt.agentOnboarding.title') }}</h4>
        <p v-if="!expanded" class="agent-onboarding-desc">
          {{ t('resource.clusterMgmt.agentOnboarding.descShort') }}
        </p>
      </div>
      <HNBButton
        variant="primary"
        size="small"
        :loading="loading"
        @click="toggle"
      >
        {{ expanded
          ? t('resource.clusterMgmt.agentOnboarding.collapse')
          : (guide ? t('resource.clusterMgmt.agentOnboarding.view') : t('resource.clusterMgmt.agentOnboarding.generate'))
        }}
      </HNBButton>
    </div>

    <div v-if="expanded" class="agent-onboarding-panel">
      <div v-if="error" class="agent-onboarding-error" role="alert">
        {{ error }}
        <HNBButton variant="ghost" size="small" @click="load">{{ t('resource.clusterMgmt.agentOnboarding.retry') }}</HNBButton>
      </div>

      <template v-else-if="guide">
        <ol class="agent-onboarding-steps">
          <li>{{ t('resource.clusterMgmt.agentOnboarding.stepInstall') }}</li>
          <li>{{ t('resource.clusterMgmt.agentOnboarding.stepWait') }}</li>
          <li>{{ t('resource.clusterMgmt.agentOnboarding.stepVerify') }}</li>
        </ol>

        <dl class="agent-onboarding-meta">
          <div>
            <dt>{{ t('resource.clusterMgmt.agentOnboarding.cluster') }}</dt>
            <dd>{{ props.clusterName || guide.displayName || guide.clusterId }}</dd>
          </div>
          <div>
            <dt>{{ t('resource.clusterMgmt.agentOnboarding.tunnelUrl') }}</dt>
            <dd>{{ guide.tunnelUrl }}</dd>
          </div>
          <div>
            <dt>{{ t('resource.clusterMgmt.agentOnboarding.namespace') }}</dt>
            <dd>{{ guide.namespace }}</dd>
          </div>
          <div>
            <dt>{{ t('resource.clusterMgmt.agentOnboarding.tokenExpiry') }}</dt>
            <dd>{{ formatExpiry(guide.tokenExpiry) }}</dd>
          </div>
        </dl>

        <div class="agent-onboarding-command">
          <div class="agent-onboarding-command-head">
            <span>{{ t('resource.clusterMgmt.agentOnboarding.installCommand') }}</span>
            <HNBButton variant="secondary" size="small" @click="copyToClipboard(guide.installCommand, 'install')">
              {{ copied === 'install' ? t('resource.clusterMgmt.agentOnboarding.copied') : t('resource.clusterMgmt.agentOnboarding.copy') }}
            </HNBButton>
          </div>
          <pre class="agent-onboarding-command-body">{{ guide.installCommand }}</pre>
        </div>

        <div class="agent-onboarding-manifest">
          <div class="agent-onboarding-manifest-head">
            <span>{{ t('resource.clusterMgmt.agentOnboarding.manifestTitle') }}</span>
            <HNBButton variant="secondary" size="small" @click="copyToClipboard(guide.manifest, 'manifest')">
              {{ copied === 'manifest' ? t('resource.clusterMgmt.agentOnboarding.copied') : t('resource.clusterMgmt.agentOnboarding.copy') }}
            </HNBButton>
          </div>
          <pre class="agent-onboarding-manifest-body">{{ guide.manifest }}</pre>
        </div>

        <div class="agent-onboarding-foot">
          <span class="agent-onboarding-warn">{{ t('resource.clusterMgmt.agentOnboarding.rotateWarn') }}</span>
          <HNBButton variant="ghost" size="small" :loading="loading" @click="regenerate">
            {{ t('resource.clusterMgmt.agentOnboarding.regenerate') }}
          </HNBButton>
        </div>
      </template>

      <div v-else class="agent-onboarding-waiting" role="status">
        {{ loading ? t('resource.clusterMgmt.agentOnboarding.generating') : '' }}
      </div>
    </div>
  </section>
</template>

<style scoped>
.agent-onboarding-guide {
  border: 1px solid var(--hnb-color-border, #d8dee6);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--hnb-color-bg-subtle, #fafbfc);
}
.agent-onboarding-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.agent-onboarding-title h4 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}
.agent-onboarding-desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--hnb-color-text-secondary, #5b6472);
}
.agent-onboarding-panel {
  margin-top: 12px;
}
.agent-onboarding-steps {
  margin: 0 0 12px;
  padding-left: 18px;
  font-size: 13px;
  display: grid;
  gap: 4px;
}
.agent-onboarding-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 20px;
  margin: 0 0 12px;
}
.agent-onboarding-meta > div {
  display: flex;
  gap: 8px;
  font-size: 13px;
}
.agent-onboarding-meta dt {
  color: var(--hnb-color-text-secondary, #5b6472);
  white-space: nowrap;
}
.agent-onboarding-meta dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.agent-onboarding-command,
.agent-onboarding-manifest {
  margin-bottom: 12px;
}
.agent-onboarding-command-head,
.agent-onboarding-manifest-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 6px;
}
.agent-onboarding-command-body,
.agent-onboarding-manifest-body {
  margin: 0;
  padding: 10px 12px;
  background: var(--hnb-color-bg-code, #0f172a);
  color: var(--hnb-color-text-on-code, #e2e8f0);
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  overflow: auto;
  max-height: 320px;
  white-space: pre;
}
.agent-onboarding-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.agent-onboarding-warn {
  font-size: 12px;
  color: var(--hnb-color-text-secondary, #5b6472);
}
.agent-onboarding-error {
  color: var(--hnb-color-danger, #d64545);
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 10px;
}
.agent-onboarding-waiting {
  font-size: 13px;
  color: var(--hnb-color-text-secondary, #5b6472);
}
</style>
