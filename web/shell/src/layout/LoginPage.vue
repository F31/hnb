<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { usePermissionStore } from '@/stores/permissionStore'

const router = useRouter()
const auth = useAuthStore()
const permission = usePermissionStore()

const username = ref('')
const password = ref('')
const error = ref('')
const submitting = ref(false)

async function handleLogin() {
  error.value = ''
  submitting.value = true
  try {
    await auth.login(username.value, password.value)
    // backend should provide permissions on /auth/login or via /v1/console/bootstrap
    if (auth.user?.permissions) {
      permission.setPermissions(auth.user.permissions)
    }
    // After login, route to /tenant-select for half-tenant selection flow
    router.push({ name: 'TenantSelect' })
  } catch (e: any) {
    error.value = e.message || '登录失败，请检查用户名和密码'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <h1><span class="logo-highlight">HNB</span> 云原生平台</h1>
      <p class="subtitle">HNB Cloud Native Application Platform</p>
      <form @submit.prevent="handleLogin">
        <div class="field">
          <label for="username">用户名</label>
          <input
            id="username"
            v-model="username"
            type="text"
            autocomplete="username"
            placeholder="请输入用户名"
            :disabled="submitting"
          />
        </div>
        <div class="field">
          <label for="password">密码</label>
          <input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            placeholder="请输入密码"
            :disabled="submitting"
          />
        </div>
        <p v-if="error" class="error">{{ error }}</p>
        <button type="submit" class="btn-login" :disabled="submitting">
          {{ submitting ? '登录中...' : '登 录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  height: 100vh;
  display: flex; align-items: center; justify-content: center;
  background: #0b0f14;
}
.login-card {
  width: 380px; padding: 40px;
  background: #171d25;
  border: 1px solid #293443;
  border-radius: 12px;
  text-align: center;
}
h1 { margin: 0 0 4px; font-size: 24px; color: #fff; }
.logo-highlight { color: #7188ff; }
.subtitle { color: #7a8a9a; font-size: 13px; margin-bottom: 28px; }
.field { text-align: left; margin-bottom: 16px; }
.field label {
  display: block; font-size: 13px; color: #b9c2d0; margin-bottom: 6px;
}
.field input {
  width: 100%; padding: 10px 12px;
  background: #1f2833;
  border: 1px solid #364052;
  border-radius: 6px;
  color: #fff;
  box-sizing: border-box;
}
.error { color: #ff8585; font-size: 13px; }
.btn-login {
  width: 100%; padding: 11px;
  background: #637bff; color: #fff;
  border: 0; border-radius: 8px;
  font-size: 15px; cursor: pointer; margin-top: 8px;
}
.btn-login:hover:not(:disabled) { background: #4f65e0; }
.btn-login:disabled { opacity: 0.6; cursor: not-allowed; }

@media (max-width: 480px) {
  .login-card { width: calc(100% - 32px); padding: 24px; }
}
</style>
