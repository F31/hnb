<script setup lang="ts">
/**
 * STALE 风险确认弹窗（KERNEL-019）。
 *
 * 仅呈现服务端 STALE challenge 提供的四维状态与策略结果，前端不猜测；
 * 需勾选风险知悉后才可确认。由 useStaleSubmit 驱动（challenge 非空即展示）。
 */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { StaleChallenge } from '../types/cluster'

const props = defineProps<{
  challenge: StaleChallenge | null
  actionLabel: string
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const { t } = useI18n()

const acknowledged = ref(false)
watch(
  () => props.challenge,
  () => {
    acknowledged.value = false
  },
)
</script>

<template>
  <div v-if="challenge" class="stale-modal-mask" role="dialog" aria-modal="true">
    <div class="stale-modal-card">
      <header class="stale-modal-header">
        <h2>{{ t('resource.clusterMgmt.staleChallenge.title') }}</h2>
      </header>
      <div class="stale-modal-body">
        <p>{{ t('resource.clusterMgmt.staleChallenge.desc') }}</p>
        <dl class="stale-intent-info">
          <dt>{{ t('resource.clusterMgmt.staleChallenge.lastKnownStateAt') }}</dt>
          <dd>{{ challenge.lastKnownStateAt || '--' }}</dd>
          <dt>{{ t('resource.clusterMgmt.staleChallenge.lifecycle') }}</dt>
          <dd>{{ challenge.lifecycleState || '--' }}</dd>
          <dt>{{ t('resource.clusterMgmt.staleChallenge.health') }}</dt>
          <dd>{{ challenge.healthState || '--' }}</dd>
          <dt>{{ t('resource.clusterMgmt.staleChallenge.connectivity') }}</dt>
          <dd>{{ challenge.connectivityState || '--' }}</dd>
          <dt>{{ t('resource.clusterMgmt.staleChallenge.action') }}</dt>
          <dd>{{ challenge.action || actionLabel }}</dd>
          <dt>{{ t('resource.clusterMgmt.staleChallenge.policy.' + challenge.policyOutcome) }}</dt>
          <dd>{{ t('resource.clusterMgmt.staleChallenge.policyDesc.' + challenge.policyOutcome) }}</dd>
        </dl>
        <p class="stale-impact">{{ t('resource.clusterMgmt.staleChallenge.impact') }}</p>
        <label class="stale-ack">
          <input v-model="acknowledged" type="checkbox" />
          <span>{{ t('resource.clusterMgmt.staleChallenge.acknowledge') }}</span>
        </label>
      </div>
      <footer class="stale-modal-footer">
        <button class="stale-btn secondary" type="button" @click="emit('cancel')">
          {{ t('resource.clusterMgmt.staleChallenge.cancel') }}
        </button>
        <button
          class="stale-btn primary"
          type="button"
          :disabled="!acknowledged"
          @click="emit('confirm')"
        >
          {{ t('resource.clusterMgmt.staleChallenge.confirm') }}
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.stale-modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 48px 16px;
  z-index: 1300;
}
.stale-modal-card {
  width: 100%;
  max-width: 480px;
  background: var(--hnb-color-bg-surface);
  border: 1px solid var(--hnb-color-border);
  border-radius: var(--hnb-radius-lg);
  box-shadow: var(--hnb-shadow-4);
  display: flex;
  flex-direction: column;
  gap: var(--hnb-space-md);
  padding: var(--hnb-space-lg);
}
.stale-modal-header h2 { margin: 0; font-size: var(--hnb-font-size-title); }
.stale-modal-body { display: flex; flex-direction: column; gap: var(--hnb-space-sm); }
.stale-modal-body p { margin: 0; color: var(--hnb-color-text-secondary); }
.stale-modal-footer { display: flex; justify-content: flex-end; gap: var(--hnb-space-sm); }
.stale-intent-info {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: var(--hnb-space-xs) var(--hnb-space-md);
  margin: 0;
}
.stale-intent-info dt { color: var(--hnb-color-text-secondary); font-size: var(--hnb-font-size-caption); }
.stale-intent-info dd { margin: 0; word-break: break-all; }
.stale-impact { margin: 0; color: var(--hnb-color-text-tertiary); font-size: var(--hnb-font-size-caption); }
.stale-ack {
  display: flex;
  align-items: center;
  gap: var(--hnb-space-sm);
  font-size: var(--hnb-font-size-body);
  color: var(--hnb-color-text-secondary);
}
.stale-ack input { width: 16px; height: 16px; accent-color: var(--hnb-color-primary); }
.stale-btn {
  padding: 8px 18px;
  border-radius: var(--hnb-radius-md);
  cursor: pointer;
  font-size: var(--hnb-font-size-body);
}
.stale-btn.primary {
  border: 0;
  background: var(--hnb-color-primary);
  color: #fff;
}
.stale-btn.primary:disabled { opacity: 0.55; cursor: not-allowed; }
.stale-btn.secondary {
  border: 1px solid var(--hnb-color-border);
  background: var(--hnb-color-bg-surface);
  color: var(--hnb-color-text-primary);
}
.stale-btn.secondary:disabled { opacity: 0.55; cursor: not-allowed; }
</style>
