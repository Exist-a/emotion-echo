import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

const ROOT = path.resolve(__dirname)

// vitest 走 vite resolver;Nuxt 别名 `~` = ROOT,#app 也是 ROOT。
// 必须在 plugins[] 里也加 vue(),确保 .vue 文件经过编译,内部 import 才走 vite resolver。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: [
      { find: '~', replacement: ROOT },
      { find: '@', replacement: ROOT },
      { find: '#app', replacement: ROOT }
    ]
  },
  test: {
    environment: 'happy-dom',
    include: ['app/**/*.{test,spec}.ts'],
    setupFiles: [path.join(ROOT, 'tests-setup.ts')],
    globals: true,
    css: false
  }
})
