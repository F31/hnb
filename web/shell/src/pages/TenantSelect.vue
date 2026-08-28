<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { normalizeWorkspace, useContextStore } from '@/stores/contextStore'
import { switchTenantAtomic } from '@/core/context'
import { fetchWorkspaces } from '@/core/api/workspaces'

const router = useRouter()
const authStore = useAuthStore()
const contextStore = useContextStore()

const workspaces = ref<any[]>([])
const currentSpace = ref<string | null>(null)
const loading = ref(false)
const error = ref('')

onMounted(async () => {
  if (!authStore.isAuthenticated) {
    router.replace({ name: 'Login' })
    return
  }
  loading.value = true
  try {
    workspaces.value = (await fetchWorkspaces()).map(normalizeWorkspace)
    if (workspaces.value.length === 0) {
      error.value = '当前用户未关联任何工作空间，请联系管理员'
      return
    }
    // Auto-select first workspace
    const first = workspaces.value[0]
    currentSpace.value = first.id
    await enterWorkspace(first.id)
  } catch (err: any) {
    error.value = err.message || '加载工作空间失败'
  } finally {
    loading.value = false
  }
})

async function enterWorkspace(spaceId: string) {
  // Atomically switch tenant: full cleanup (plugins/routes/nav/permission)
  // via switchTenantAtomic, then bind the selected space.
  const ws = workspaces.value.find((w) => w.id === spaceId)
  const tenantId = ws?.tenantId
  if (!tenantId) {
    error.value = '工作空间缺少 tenantId 信息'
    return
  }
  await switchTenantAtomic(tenantId)
  contextStore.setSpace(spaceId, contextStore.switchGeneration)
  router.push({ name: 'Dashboard' })
}

function handleSpaceChange() {
  if (currentSpace.value) {
    enterWorkspace(currentSpace.value)
  }
}
</script>

<template>
  <div class="tenant-page">
    <div class="selector-card">
      <h2><span class="logo-highlight">HNB</span> 工作空间选择</h2>
      <p class="subtitle">请选择要操作的工作空间</p>

      <p v-if="loading" class="hint">正在加载...</p>
      <p v-else-if="error" class="error">{{ error }}</p>

      <form v-else @submit.prevent="handleSpaceChange">
        <div class="field">
          <label for="space-select">工作空间</label>
          <select id="space-select" v-model="currentSpace">
            <option v-for="ws in workspaces" :key="ws.id" :value="ws.id">
              {{ ws.displayName || ws.name }}
            </option>
          </select>
        </div>
        <button type="submit" class="btn-enter" :disabled="!currentSpace">
          进入控制台
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.tenant-page {
  height: 100vh;
  display: flex; align-items: center; justify-content: center;
  background: #0b0f14;
}
.selector-card {
  width: 380px; padding: 32px;
  background: #171d25;
  border: 1px solid #293443;
  border-radius: 12px;
  text-align: center;
}
h2 { margin: 0 0 8px; font-size: 22px; color: #fff; }
.logo-highlight { color: #7188ff; }
.subtitle { color: #7a8a9a; font-size: 13px; margin-bottom: 24px; }
.field { text-align: left; margin-bottom: 20px; }
.field label {
  display: block; font-size: 13px;
  color: #b9c2d0; margin-bottom: 8px;
}
.field select {
  width: 100%; padding: 10px 12px;
  background: #1f2833;
  border: 1px solid #364052;
  border-radius: 6px;
  color: #fff; font-size: 14px;
  box-sizing: border-box;
}
.btn-enter {
  width: 100%; padding: 12px;
  background: #637bff; color: #fff;
  border: 0; border-radius: 8px;
  font-size: 15px; cursor: pointer;
  transition: background 0.2s;
}
.btn-enter:hover:not(:disabled) { background: #4f65e0; }
.btn-enter:disabled { opacity: 0.5; cursor: not-allowed; }
.hint, .error {
  color: #b9c2d0; font-size: 14px; margin: 16px 0;
}
.error { color: #ff8585; }
</style>
