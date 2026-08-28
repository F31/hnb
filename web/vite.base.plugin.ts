import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export function createPluginConfig(name: string, extraExternals: string[] = []) {
  const libName = `HNBPlugin${name.charAt(0).toUpperCase() + name.slice(1)}`
  return defineConfig({
    plugins: [vue()],
    define: {
      'process.env.NODE_ENV': JSON.stringify('production'),
    },
    build: {
      outDir: 'dist',
      lib: {
        entry: 'src/index.ts',
        name: libName,
        formats: ['es'],
        fileName: 'index',
      },
      rollupOptions: {
        external: ['vue', 'vue-i18n', 'vue-router', '@hnb/types', '@hnb/ui-kit', ...extraExternals],
        output: { globals: { vue: 'Vue' } },
      },
    },
  })
}
