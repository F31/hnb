import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'shell/src'),
      '@hnb/types': resolve(__dirname, 'packages/types/src'),
      '@hnb/ui-kit': resolve(__dirname, 'packages/ui-kit/src'),
      '@hnb/api-client': resolve(__dirname, 'packages/api-client/src'),
      '@hnb/schema-engine': resolve(__dirname, 'packages/schema-engine/src'),
    },
    conditions: ['development', 'browser', 'module', 'import', 'require'],
  },
  test: {
    environment: 'jsdom',
    include: [
      'shell/src/**/__tests__/**/*.test.ts',
      'packages/ui-kit/src/**/__tests__/**/*.test.ts',
      'packages/api-client/src/**/__tests__/**/*.test.ts',
      'packages/plugin-sdk/src/**/__tests__/**/*.test.ts',
      'packages/schema-engine/src/**/__tests__/**/*.test.ts',
      'plugins/*/src/**/__tests__/**/*.test.ts',
    ],
    coverage: {
      provider: 'v8',
      include: ['shell/src/**', 'packages/ui-kit/src/**'],
      exclude: ['**/__tests__/**', '**/*.d.ts'],
    },
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
})
