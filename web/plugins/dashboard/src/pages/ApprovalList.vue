<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const pendingCount = ref(0)
const loading = ref(true)
const router = useRouter()

onMounted(async () => {
  try {
    const res = await fetch('/api/v1/operations?status=pending_approval&pageSize=1')
    const json = await res.json()
    pendingCount.value = json?.pagination?.total ?? 0
  } catch {
    pendingCount.value = 0
  } finally {
    loading.value = false
  }
})

function goToApprovals() {
  router.push('/system/approvals')
}
</script>
<template>
  <div class="dash-approvals">
    <div class="dash-approvals__header">
      <h1>{{ $t('dashboard.approvals.title') }}</h1>
      <span v-if="!loading" class="dash-approvals__count" :class="{ 'has-pending': pendingCount > 0 }">{{ pendingCount }}</span>
    </div>
    <p v-if="loading" class="dash-approvals__loading">{{ $t('dashboard.common.loading') }}</p>
    <p v-else-if="pendingCount === 0" class="dash-approvals__empty">{{ $t('dashboard.approvals.empty') }}</p>
    <p v-else class="dash-approvals__pending">{{ $t('dashboard.approvals.pending', { count: pendingCount }) }}</p>
    <button v-if="pendingCount > 0" class="dash-approvals__btn" @click="goToApprovals">{{ $t('dashboard.approvals.viewAll') }}</button>
  </div>
</template>
<style scoped>
.dash-approvals { padding:20px; }
.dash-approvals__header { display:flex;align-items:center;gap:10px; }
.dash-approvals__header h1 { margin:0;font-size:18px; }
.dash-approvals__count { display:inline-flex;align-items:center;justify-content:center;min-width:24px;height:24px;padding:0 6px;border-radius:99px;background:var(--hnb-color-border);font-size:12px;font-weight:600;color:var(--hnb-color-text-secondary); }
.dash-approvals__count.has-pending { background:rgba(240,68,56,0.15);color:#ffb4ad; }
.dash-approvals__loading { color:var(--hnb-color-text-tertiary); }
.dash-approvals__empty { color:var(--hnb-color-text-tertiary); }
.dash-approvals__pending { color:var(--hnb-color-text-primary); }
.dash-approvals__btn { margin-top:8px;padding:6px 16px;border:1px solid var(--hnb-color-primary);border-radius:var(--hnb-radius-md);background:transparent;color:var(--hnb-color-primary);cursor:pointer;font-size:13px; }
</style>