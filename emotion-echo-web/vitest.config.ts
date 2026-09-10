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
      // Nuxt alias `~` / `@` / `#app` = <srcDir> = `<ROOT>/app`
      // （之前直接 alias 到 ROOT 让 `~/types/api` 解析失败——见 PR-A 测试）
      { find: /^~(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^@(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^#app(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^#imports(?=\/)/, replacement: path.join(ROOT, 'app') },
      // 兜底：~ / @ / #app 单独出现（不带 /）时 alias 到 app
      { find: '~', replacement: path.join(ROOT, 'app') },
      { find: '@', replacement: path.join(ROOT, 'app') },
      { find: '#app', replacement: path.join(ROOT, 'app') }
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
