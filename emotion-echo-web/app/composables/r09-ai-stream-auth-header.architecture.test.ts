import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'

// Sprint 111 · R-09 architecture regression (static-source)
//
// Sprint 110 浏览器实测发现 (2026-09-17 09:05:14, after rebuild + 演示账号登录):
//   POST /api/v1/ai/stream Authorization header = "" (空字符串)
//   → APISIX jwt-auth 401 "JWT token invalid"
//   → 浏览器 AI 回复气泡 = "请求失败: JWT token invalid"
//
// 真根因: cookie access_token 是 HttpOnly (Set-Cookie: ...; HttpOnly),
//         浏览器 JS 读不到 (document.cookie = ""),
//         useCookie('access_token').value 永远返回空,
//         直接 useCookie 拼 Authorization 头永远空.
//
// 修法 (centralized helper): app/lib/clientAccessToken.ts getClientAccessToken()
//   1. CSR 优先 userStore.getAccessToken (Pinia ref, 浏览器 JS 可见)
//   2. fallback useCookie (SSR / store 未初始化场景)
//   3. server 端直接 cookie
//
// 本测试钉住: ai-stream / TTS 调用代码禁止直接 useCookie('access_token').value,
// 必须通过 getClientAccessToken() helper, 保证 Authorization header 不为空.

const handlerSrc = readFileSync('./app/composables/useAIStreamHandler.ts', 'utf8')
// CL-04 Phase 5：`useAIStream.ts` 已作为死路径删除（本项目唯一生产客户端是 useAIStreamHandler），
// 原断言随之移除；回流防护见下方「死路径不得回流」用例。
const ttsSrc = readFileSync('./app/composables/useTTSPlayer.ts', 'utf8')
const helperSrc = readFileSync('./app/lib/clientAccessToken.ts', 'utf8')

describe('Sprint 111 · R-09 architecture: ai-stream/TTS Authorization header 不能为空', () => {
  it('useAIStreamHandler MUST call getClientAccessToken() (HttpOnly cookie 浏览器读不到)', () => {
    // 直接 useCookie('access_token').value 在浏览器永远空 (HttpOnly 限制)
    const usesGetClientAccessToken = /getClientAccessToken\s*\(/.test(handlerSrc)
    expect(
      usesGetClientAccessToken,
      'useAIStreamHandler 必须通过 getClientAccessToken() 读 token, ' +
        "不能直接 useCookie('access_token').value (Sprint 111 浏览器实测: " +
        'Authorization 空 → APISIX jwt-auth 401).',
    ).toBe(true)
  })

  it('useTTSPlayer MUST call getClientAccessToken() (同 R-09 根因)', () => {
    const usesGetClientAccessToken = /getClientAccessToken\s*\(/.test(ttsSrc)
    expect(
      usesGetClientAccessToken,
      'useTTSPlayer 必须通过 getClientAccessToken() 读 token. ' +
        'Sprint 111 浏览器实测确认 R-09 在此文件同根因.',
    ).toBe(true)
  })

  it('clientAccessToken helper 必须存在并导出 getClientAccessToken', () => {
    const exportsHelper = /export\s+function\s+getClientAccessToken\s*\(/.test(helperSrc)
    expect(
      exportsHelper,
      'app/lib/clientAccessToken.ts 必须导出 getClientAccessToken. ' +
        'R-09 修复核心 helper, 所有 ai-stream/TTS 统一通过它读 token.',
    ).toBe(true)
  })

  it('clientAccessToken helper CSR 分支优先 userStore (浏览器 JS 可见)', () => {
    // Sprint 111 · R-09 真根因: HttpOnly cookie 读不到, 必须 userStore 优先
    const hasCsrBranch = /import\.meta\.client/.test(helperSrc)
    const readsStore = /useUserStore|getAccessToken/.test(helperSrc)
    expect(
      hasCsrBranch && readsStore,
      'helper 必须 (1) 区分 CSR / SSR, (2) CSR 优先 userStore.getAccessToken. ' +
        '否则继续读不到 token, 401 复发.',
    ).toBe(true)
  })

  it('clientAccessToken helper SSR / fallback 走 useCookie', () => {
    const callsUseCookie = /useCookie\s*\(/.test(helperSrc)
    expect(
      callsUseCookie,
      'helper 必须保留 useCookie fallback (SSR 渲染 / store 未初始化场景).',
    ).toBe(true)
  })

  it("R-09 修复前 (回归保护): 两个调用点禁止再单独 useCookie('access_token').value 拼 Authorization", () => {
    // 反向钉死: 不准再用 useCookie('access_token').value 作为 Authorization header 唯一来源
    const badPatterns = [
      { name: 'useAIStreamHandler', src: handlerSrc },
      { name: 'useTTSPlayer', src: ttsSrc },
    ]
    for (const { name, src } of badPatterns) {
      // 匹配 useCookie('access_token').value 直接拼 Authorization (无 store fallback)
      const directCookieAsAuth =
        /Authorization.*useCookie\s*\(\s*['"]access_token['"]\s*\)\.value/s.test(src)
      expect(
        !directCookieAsAuth,
        `${name} 不能直接用 useCookie('access_token').value 拼 Authorization (Sprint 111 浏览器实测 401 根因). ` +
          '必须改用 getClientAccessToken().',
      ).toBe(true)
    }
  })

  // CL-04 Phase 5（T-12）：死路径清零后的**回流防护**。
  // 这两条死路径（另一套 OpenAI 兼容 SSE 客户端 / 硬编码 RCT prompt）都容易被当成"备用实现"
  // 留着 —— 留着就有人接错线；**删除 + 钉死"不得回流"**才算清零。
  it('死路径不得回流: useAIStream.ts / usePrompt.ts / promptType.ts 必须不存在', () => {
    expect(
      existsSync('./app/composables/useAIStream.ts'),
      'useAIStream.ts 已删（本项目唯一生产客户端是 useAIStreamHandler）；不得回流',
    ).toBe(false)
    expect(
      existsSync('./app/composables/usePrompt.ts'),
      'usePrompt.ts（含 buildRCTPrompt）已删；不得回流',
    ).toBe(false)
    expect(
      existsSync('./app/types/prompt/promptType.ts'),
      '仅服务于已删死路径的类型文件一并清除；不得回流',
    ).toBe(false)
  })

  it('死路径不得回流: 生产链路不得再出现 buildRCTPrompt 引用', () => {
    const productionFiles = [
      './app/composables/useAIStreamHandler.ts',
      './app/composables/useConversationSender.ts',
    ]
    for (const p of productionFiles) {
      const src = readFileSync(p, 'utf8')
      expect(/buildRCTPrompt/.test(src), `${p} 不得引用 buildRCTPrompt（已删）`).toBe(false)
    }
  })
})
