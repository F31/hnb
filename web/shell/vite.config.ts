import { defineConfig, type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { copyFileSync, existsSync, mkdirSync } from 'fs'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

/**
 * 把 vue / vue-i18n / vue-router / @hnb/ui-kit 拷入 public/vendor，
 * 供 index.html 的 import map 引用。Shell 与插件 Bundle 将 vue 系列
 * 标记为 external，通过 import map 共享同一运行时实例，避免双实例
 * 导致的响应式、依赖注入与 locale 状态断裂。
 */
function vendorPlugin(): Plugin {
  const files = [
    'vue/dist/vue.esm-browser.prod.js',
    'vue-i18n/dist/vue-i18n.esm-browser.prod.js',
    'vue-router/dist/vue-router.esm-browser.prod.js',
  ]
  const destDir = resolve(__dirname, 'public/vendor')
  const copy = () => {
    mkdirSync(destDir, { recursive: true })
    for (const f of files) {
      copyFileSync(resolve(__dirname, 'node_modules', f), resolve(destDir, f.split('/').pop()!))
    }
    const uiKitDist = resolve(__dirname, '../packages/ui-kit/dist/index.js')
    const uiKitCss = resolve(__dirname, '../packages/ui-kit/dist/index.css')
    if (existsSync(uiKitDist)) {
      copyFileSync(uiKitDist, resolve(destDir, 'ui-kit.esm.js'))
    }
    if (existsSync(uiKitCss)) {
      copyFileSync(uiKitCss, resolve(destDir, 'ui-kit.css'))
    }
  }
  return {
    name: 'hnb-vendor',
    buildStart: copy,
    configureServer: copy,
  }
}

/**
 * dev 模式双 Vue 实例根治。
 *
 * 生产：Shell 与插件 Bundle 都把 vue 标记为 external，经 import map
 * 共享 /vendor 的同一份 vue（单实例）。
 * dev：Shell 源码由 vite 提供 vue（预构建依赖）；若插件仍经原生
 * import('/modules/<name>/index.js') 加载，其裸导入 'vue' 会被浏览器
 * import map 解析到 /vendor 的另一份拷贝 —— 双实例，inject/reactive 断裂。
 *
 * 该中间件把 /modules/<name>/index.js 重写到插件源码入口并交给 vite
 * 转换管线：裸导入统一改写为 vite 的 vue，插件与 Shell 在 dev 下同样
 * 共享单实例；同时免去了 dev 前手动构建插件 Bundle 的步骤。
 */
function devModulesPlugin(): Plugin {
  return {
    name: 'hnb-dev-modules',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, _res, next) => {
        const match = req.url?.match(/^\/modules\/([\w-]+)\/index\.js(?:\?.*)?$/)
        if (match) {
          const entry = resolve(__dirname, '../plugins', match[1], 'src/index.ts')
          if (existsSync(entry)) {
            req.url = `/@fs${entry}`
          }
        }
        next()
      })
    },
  }
}

export default defineConfig({
  plugins: [
    vue(),
    vendorPlugin(),
    devModulesPlugin(),
    AutoImport({
      imports: [
        'vue',
        'vue-router',
        'pinia',
        { 'naive-ui': ['useDialog', 'useMessage', 'useNotification', 'useLoadingBar'] },
      ],
      dts: resolve(__dirname, 'src/auto-imports.d.ts'),
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: resolve(__dirname, 'src/components.d.ts'),
    }),
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@hnb/types': resolve(__dirname, '../packages/types/src'),
      '@hnb/ui-kit': resolve(__dirname, '../packages/ui-kit/src'),
      '@hnb/api-client': resolve(__dirname, '../packages/api-client/src'),
      '@hnb/schema-engine': resolve(__dirname, '../packages/schema-engine/src'),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        shell: resolve(__dirname, 'index.html'),
      },
      // vue / vue-i18n / vue-router 经 import map 从 /vendor 加载，与插件 Bundle 共享同一实例
      external: ['vue', 'vue-i18n', 'vue-router'],
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8001',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://localhost:8001',
        changeOrigin: true,
      },
    },
  },
})