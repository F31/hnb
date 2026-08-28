import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { getRouterManager } from '@/core/router/RouterManager'
import { getEventBus } from '@/core/event-bus'
import { i18n } from '@/i18n'
import { useAuthStore } from '@/stores/authStore'

async function bootstrap() {
  const pinia = createPinia()
  const app = createApp(App)
  app.use(pinia)
  app.use(i18n)

  // Restore persisted authentication before the router resolves an initial
  // deep link; App performs freshness/bootstrap checks after mounting.
  useAuthStore(pinia).restoreSession()

  const eventBus = getEventBus()
  void eventBus

  const routerManager = getRouterManager([])
  app.use(routerManager.getRouter())

  // 全局错误捕获，防止单个组件崩溃影响整个页面
  app.config.errorHandler = (err, _instance, info) => {
    console.warn('[HNB] global error:', err, info)
  }

  app.mount('#app')
  console.log('[HNB Shell] mounted successfully')
}

bootstrap().catch((err) => {
  console.error('[HNB Shell] bootstrap failed:', err)
})
