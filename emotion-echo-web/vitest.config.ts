import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

const ROOT = path.resolve(__dirname)

// vitest 走 vite resolver;Nuxt 别名 `~` = ROOT,#app 也是 ROOT。
// 必须在 plugins[] 里也加 vue(),确保 .vue 文件经过编译,内部 import 才走 vite resolver。
// E2E-12：Nuxt 在编译期把 `import.meta.client` / `import.meta.server` 替换为字面量；
// vitest 不经 Nuxt 编译，二者恒为 `undefined` ⇒ 带 `if (!import.meta.client) return`
// 守卫的客户端逻辑（如 store.applyTheme）在测试里被**静默跳过**、根本测不到。
//
// 注意：Vite 的 `define` 对 `import.meta.*` 不生效（test 实测仍为 undefined），
// 必须在 transform 阶段做等价替换。happy-dom 本身就是客户端语义，故固定为
// client=true / server=false。
function nuxtImportMetaShim() {
  return {
    name: 'vitest-nuxt-import-meta-shim',
    enforce: 'pre' as const,
    transform(code: string, id: string) {
      if (id.includes('node_modules')) return null
      if (!code.includes('import.meta.client') && !code.includes('import.meta.server')) return null
      return {
        code: code
          .replace(/import\.meta\.client/g, 'true')
          .replace(/import\.meta\.server/g, 'false'),
        map: null,
      }
    }
  }
}

export default defineConfig({
  plugins: [nuxtImportMetaShim(), vue()],
  resolve: {
    alias: [
      // Nuxt alias `~` / `@` / `#app` = <srcDir> = `<ROOT>/app`
      // （之前直接 alias 到 ROOT 让 `~/types/api` 解析失败——见 PR-A 测试）
      { find: /^~(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^@(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^#app(?=\/)/, replacement: path.join(ROOT, 'app') },
      { find: /^#imports(?=\/)/, replacement: path.join(ROOT, 'app') },
      // 兜底：~ / @ 单独出现（不带 /）时 alias 到 app
      { find: '~', replacement: path.join(ROOT, 'app') },
      { find: '@', replacement: path.join(ROOT, 'app') },
      // E2E-F-39: #app 精确匹配走 mock 模块 (提供 useCookie / useRuntimeConfig 等 Nuxt auto-import)
      { find: '#app', replacement: path.join(ROOT, 'tests-app-mock.ts') }
    ]
  },
  test: {
    environment: 'happy-dom',
    include: ['app/**/*.{test,spec}.ts'],
    setupFiles: [path.join(ROOT, 'tests-setup.ts')],
    globals: true,
    css: false,
    // R-01: 增加 testTimeout 解决并发模式下 happy-dom 初始化慢导致的超时
    // 单独运行全绿，并发时 happy-dom 环境初始化竞争导致 5s 超时
    testTimeout: 15000,
    hookTimeout: 15000
  }
})
