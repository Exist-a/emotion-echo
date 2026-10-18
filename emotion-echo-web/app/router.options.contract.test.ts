// app/router.options.contract.test.ts
//
// CL-06 · Phase 8 运行态验收抓出的**架构陷阱**回归钉：
//
// 现象：`app/pages/chat/agents.vue` 存在、也被编进了 chunk，但访问 `/chat/agents` 返回
// Nuxt 的 404（`Page not found: /chat/agents`）。生产构建与本地 `pnpm build` 均可复现。
//
// 根因：本仓 `app/router.options.ts` 用**手写路由表覆盖**了 Nuxt 自动生成的路由 ——
// `routes: (_routes) => [ ... ]` **忽略**了 `_routes` 参数，于是"文件系统即路由"这一
// Nuxt 约定在本仓**不成立**：
//   · 新增 `app/pages/**` 下的页面文件**不会**自动产生路由（静默 404）；
//   · 该文件里也**不会**报错，类型检查/单测全绿 —— 只有真人点到那个 URL 才知道。
//
// 本测试把这条隐式约定变成**显式门禁**：pages 下每个页面文件都必须在 router.options.ts
// 里被注册。若某页确实要长期不可达，必须写进下面的 `INTENTIONALLY_UNROUTED` 并附理由。
import { describe, it, expect } from 'vitest'
import fg from 'fast-glob'
import { readFileSync } from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(__dirname, '..')
const ROUTER_OPTIONS = path.join(ROOT, 'app', 'router.options.ts')

/**
 * 有意不注册的页面。**新增须附理由**（否则本测试会失败）。
 *
 * ⚠️ 唯一合法理由是"该页确实不需要路由"；"以后再说"不算 ——
 * 未注册 = 访问 404，而这一点在类型检查与其它单测里完全不可见。
 */
const INTENTIONALLY_UNROUTED: string[] = [
  // 根页：由 router.options.ts 顶部的 `{ name: 'root', path: '/', redirect: { name: 'chat' } }`
  // 承担（重定向而非渲染该页），故不需要 import 它。
  'pages/index.vue',
  // Lane O（端侧 WebLLM）的演示页：**不属于任何导航入口**，当前也确实不可达。
  // 不是 CL-06 引入的问题、也不属本阶段范围 —— 已登账 **CL-F-46**（连带记录本仓
  // "手写路由表与文件树漂移"的系统性风险）。此处显式豁免，便于将来有人要接它时一眼可见。
  'pages/demo/local-llm.vue',
]

describe('router.options.ts · 手写路由表必须覆盖 pages 下每个页面', () => {
  it('每个 app/pages/**/*.vue 都在 router.options.ts 里被 import', async () => {
    const files = await fg(['app/pages/**/*.vue'], { cwd: ROOT, absolute: false })
    expect(files.length).toBeGreaterThan(0)

    const src = readFileSync(ROUTER_OPTIONS, 'utf8')
    const missing = files
      .map((f) => f.replace(/^app\//, ''))
      .filter((rel) => !INTENTIONALLY_UNROUTED.includes(rel))
      // router.options.ts 里的写法是 `import('~/pages/chat/agents.vue')`；`~` 即 app/，
      // 故比对 `~/` + pages 下的相对路径（早期版本误写成 `~/pages/${rel}` ⇒ 恒不匹配、假红，
      // 这层"守卫自身写错"的坑由反向自检发现：注册好路由后它仍红）。
      .filter((rel) => !src.includes(`~/${rel.replace(/\\/g, '/')}`))

    expect(
      missing,
      '这些页面文件没有出现在 app/router.options.ts 的手写路由表里 ⇒ 访问会 404（Nuxt 的' +
        '"文件系统即路由"被本文件覆盖，不会自动生效）。请为它们补一条路由；' +
        '若确为有意不可达，写进本测试的 INTENTIONALLY_UNROUTED 并说明原因。',
    ).toEqual([])
  })

  it('角色管理页（CL-06）已注册为 /chat 的子路由（本次缺陷的定点回归钉）', () => {
    const src = readFileSync(ROUTER_OPTIONS, 'utf8')
    expect(src).toContain(`~/pages/chat/agents.vue`)
    // 必须是 /chat 的**子**路由（继承 nav 布局与 requiresAuth），而不是顶层路由
    expect(src).toMatch(/name:\s*'chat-agents'/)
    expect(src).toMatch(/path:\s*'agents'/)
  })
})
