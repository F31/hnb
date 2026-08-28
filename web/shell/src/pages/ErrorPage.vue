<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { useNavigationStore } from '@/stores/navigationStore'
import { usePluginStore } from '@/stores/pluginStore'
import { storeToRefs } from 'pinia'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const navStore = useNavigationStore()
const pluginStore = usePluginStore()

const errorType = computed(() => {
  switch (route.name) {
    case 'PluginError':
      return 'plugin'
    case 'PluginUnavailable':
      return 'unavailable'
    case 'LicenseRequired':
      return 'license'
    case 'Forbidden':
      return 'forbidden'
    case 'NotFound':
      return 'notfound'
    default:
      return 'unknown'
  }
})

const titles: Record<string, string> = {
  plugin: '插件加载失败',
  unavailable: '插件不可用',
  license: '许可证受限',
  forbidden: '权限不足',
  notfound: '页面未找到',
  unknown: '系统错误',
}

const messages: Record<string, string> = {
  plugin: '必要的插件未能成功加载，请联系系统管理员',
  unavailable: '您访问的功能所需的插件未启用或未安装',
  license: '此功能需要有效的许可证授权',
  forbidden: '您没有访问此资源的权限',
  notfound: '请求的页面不存在',
  unknown: '发生了一个意外错误，请稍后重试',
}

function backHome() {
  router.push({ name: 'Dashboard' })
}

void authStore
void navStore
void pluginStore
</script>

<template>
  <div class="error-page">
    <h1 class="error-icon" :class="errorType">⚠️</h1>
    <h2 class="error-title">{{ titles[errorType] || '未知错误' }}</h2>
    <p class="error-message">
      {{ messages[errorType] || '抱歉，发生了一些问题' }}
    </p>
    <button class="btn-home" @click="backHome">返回首页</button>
  </div>
</template>

<style scoped>
.error-page {
  padding: 80px 24px;
  text-align: center;
  background: #0b0f14;
  height: 100vh;
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
}
.error-icon {
  font-size: 64px;
  color: #ff8585;
  margin: 0 0 16px;
}
.error-title {
  color: #fff;
  font-size: 24px;
  margin: 0 0 12px;
}
.error-message {
  color: #b9c2d0;
  font-size: 15px;
  margin: 0 0 32px;
  max-width: 480px;
}
.btn-home {
  padding: 12px 28px;
  background: #637bff;
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}
.btn-home:hover { background: #4f65e0; }
</style>
