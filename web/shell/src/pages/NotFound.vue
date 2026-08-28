<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
</script>

<template>
  <div class="not-found">
    <h1>404</h1>
    <p>您访问的页面不存在</p>
    <button class="btn-home" @click="router.push({ name: 'Dashboard' })">
      返回首页
    </button>
  </div>
</template>

<style scoped>
.not-found {
  height: 100vh;
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
  background: #0b0f14; color: #fff;
}
h1 { font-size: 72px; margin: 0 0 8px; color: #7188ff; }
p { color: #7a8a9a; font-size: 16px; margin: 0 0 24px; }
.btn-home {
  padding: 12px 24px;
  background: #637bff; color: #fff;
  border: 0; border-radius: 6px;
  cursor: pointer; font-size: 14px;
}
.btn-home:hover { background: #4f65e0; }
</style>
