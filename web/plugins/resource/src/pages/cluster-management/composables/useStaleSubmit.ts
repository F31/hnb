/**
 * STALE 风险确认提交流程（KERNEL-019 非预选确认）。
 *
 * 提交 RuntimeIntent 若收到 409 STALE_CONFIRMATION_REQUIRED，展示风险确认弹窗
 * （StaleChallengeDialog），用户确认后携带 riskConfirmation 重提；取消则中止。
 * 列表页（升级/删除）与接入向导（创建/导入）共用，保证确认语义一致。
 */
import { ref } from 'vue'
import * as api from '../api/clusterApi'
import type { RuntimeIntentEnvelope, RuntimeIntentRecord, StaleChallenge } from '../types/cluster'

export function useStaleSubmit() {
  const staleChallenge = ref<StaleChallenge | null>(null)
  const staleActionLabel = ref('')
  let staleResolver: ((confirmed: boolean) => void) | null = null

  function requestStaleConfirm(challenge: StaleChallenge, actionLabel: string): Promise<boolean> {
    staleChallenge.value = challenge
    staleActionLabel.value = actionLabel
    return new Promise((resolve) => {
      staleResolver = resolve
    })
  }

  function resolveStaleConfirm(confirmed: boolean): void {
    const resolver = staleResolver
    staleResolver = null
    staleChallenge.value = null
    resolver?.(confirmed)
  }

  /** 提交意图；收到 STALE challenge 时弹出风险确认，确认后携带 riskConfirmation 重提 */
  async function submit(
    envelope: RuntimeIntentEnvelope,
    actionLabel: string,
  ): Promise<RuntimeIntentRecord | 'cancelled'> {
    try {
      return await api.submitRuntimeIntent(envelope)
    } catch (err) {
      const challenge = api.staleChallengeFromError(err)
      if (!challenge) throw err
      const confirmed = await requestStaleConfirm(challenge, actionLabel)
      if (!confirmed) return 'cancelled'
      return api.submitRuntimeIntent(envelope, {
        riskConfirmation: { acknowledged: true, confirmation: challenge.confirmation },
      })
    }
  }

  return { staleChallenge, staleActionLabel, submit, resolveStaleConfirm }
}
