import { test, expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

/**
 * E2E-17 数字人 + TTS 回归钉（plan §4 #14 / §6 step 5）
 *
 * 覆盖测试点：
 *   #3  BFF /api/v1/tts/phonemes 返回契约（audio + phonemes + duration）
 *   #10 假随机轮播已删（字面契约在 vitest；此处验 chat 页数字人区域 + 语音按钮仍存在）
 *   #13/#14 段间 gap：单测钉微任务级衔接（queue.test）；此处验端到端 TTS 链路可达
 *   #17 端到端：发消息 → AI 回复 → 数字人区域可见（[V] 截图）
 *   #18 双 project 全绿（chromium + mobile）
 *
 * 前置：dev 栈健康（RUNBOOK §2.1），xtts 容器 healthy（A0），BFF v0.1.27+（F-127）
 *
 * Mobile #3 BLOCKED 修复（2026-09-23 STATUS.md §四.2）：
 *   `page.request.post` 在 mobile viewport 下持续 502（chromium mobile API
 *   client 与 chromium client 行为差异；端点本身 curl 200 + 18.7s warm 通）。
 *   改用 `page.evaluate(() => fetch(...))` —— 走浏览器页面上下文网络栈，
 *   绕开 Playwright API client 差异。warm-up 也走 fetch（cold path 90s
 *   timeout 不够 cold 路径 100s+，先发一个 1 字 warm-up 让模型加载）。
 */

const API_BASE = 'http://localhost:19080'
const DEMO = { username: 'echo', password: 'echo123' }
const SCREENSHOT_DIR = join(process.cwd(), 'screenshots')
mkdirSync(SCREENSHOT_DIR, { recursive: true })

async function loginViaAPI(page: import('@playwright/test').Page): Promise<string> {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login must return accessToken').toBeTruthy()
  await page.context().addCookies([
    { name: 'access_token', value: token, url: 'http://localhost:3000' },
  ])
  return token
}

async function waitForHydration(page: import('@playwright/test').Page) {
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}

/**
 * 通过浏览器页面上下文发起 phonemes 请求。
 *
 * 用 `page.evaluate(fetch)` 而非 `page.request.post`：
 * - page.request 走 Playwright API client（chromium mobile viewport 下持续 502）
 * - page.evaluate(fetch) 走 Chromium 浏览器网络栈（与前端真实调用一致）
 *
 * 前置：cookie 已注入 access_token（受 cookie 域限制只走 localhost:3000）；
 * fetch 直接打 APISIX 网关 :19080 携带 Bearer（不经 cookie 路径），故显式注入。
 */
async function fetchPhonemesViaBrowser(
  page: import('@playwright/test').Page,
  token: string,
  payload: { text: string; language: string; speed: number },
): Promise<{ status: number; body: any }> {
  return await page.evaluate(
    async ({ url, token, payload }) => {
      try {
        const r = await fetch(url, {
          method: 'POST',
          mode: 'cors',
          credentials: 'omit',
          headers: {
            Authorization: `Bearer ${token}`,
            'Content-Type': 'application/json; charset=utf-8',
          },
          body: JSON.stringify(payload),
        })
        const body = await r.json().catch(() => null)
        return { status: r.status, body }
      } catch (e: any) {
        // 把异常贴到 DOM 以便 Playwright 报错时拿得到（页面 console 不外传）
        const div = document.createElement('div')
        div.id = '__fetch_error__'
        div.textContent = `${e?.name ?? 'unknown'}: ${e?.message ?? String(e)}`
        document.body.appendChild(div)
        return { status: 0, body: { error: String(e?.message ?? e), type: e?.name ?? 'unknown' } }
      }
    },
    { url: `${API_BASE}/api/v1/tts/phonemes`, token, payload },
  )
}

test.describe('E2E-17 数字人 + TTS', () => {
  test('#3 BFF /tts/phonemes 返回 audio+phonemes+duration 契约', async ({ page }) => {
    // XTTS CPU 推理：cold path 100s+ / warm path 16-30s。BFF→XTTS 90s timeout
    // （E2E-F-127 yaml/config.go 漂移修复）+ APISIX upstream 180s timeout。
    // XTTS 单实例串行推理：full spec 跑时其他测试可能占用 worker，导致本次请求
    // 被串行延后触发 client.Timeout。test 级 600s 给 6 次 retry × 60s 间隔留余量。
    test.setTimeout(600_000)
    const token = await loginViaAPI(page)

    // page.evaluate(fetch) 必须发生在 http(s) 文档上下文中（about:blank 下
    // 跨域 fetch 触发 chromium same-origin policy → "TypeError: Failed to fetch"）。
    // 先 goto 把浏览器放进 web 域；用 /api/v1/health 同源 HEAD 也行但 /login 是 Nuxt
    // 路由直接 hydration，最简单。
    await page.goto('/login')
    await waitForHydration(page)

    // XTTS 单实例串行推理：full spec 跑时其他测试可能占用 worker，导致本次请求
    // 被串行延后触发 client.Timeout（cold path 100s+ 撞 90s BFF timeout）。
    // 重试 6 次 + 60s 间隔（让 XTTS worker 队列彻底清空；warm path 实测 14-30s，
    // 60s 间隔给 cold path 余量）。test 级 600s 给 6 次 retry 留余量。
    let envelope: any
    let lastStatus = -1
    for (let attempt = 1; attempt <= 6; attempt++) {
      const r = await fetchPhonemesViaBrowser(page, token, {
        text: '你好',
        language: 'zh-cn',
        speed: 1.0,
      })
      lastStatus = r.status
      envelope = r.body
      console.log(`[f17] phonemes attempt ${attempt}/6 status=${lastStatus} body=${JSON.stringify(r.body).slice(0, 200)}`)
      if (lastStatus === 200) break
      if (attempt < 6) await page.waitForTimeout(60000) // 60s 间隔让 XTTS worker 队列清空
    }
    expect(lastStatus, `6 次重试后仍未 200（XTTS 持续不可达）`).toBe(200)

    expect(envelope.code, `BFF envelope code: ${envelope.message}`).toBe(0)

    const data = envelope.data
    expect(data, 'data 必须存在').toBeTruthy()
    expect(data.audio, 'audio base64 必须非空').toBeTruthy()
    expect(data.sample_rate).toBe(24000)
    expect(data.text).toBe('你好')
    expect(data.duration, 'duration 必须 > 0').toBeGreaterThan(0)

    // phoneme 数组 = 输入字符数（per-char 等分，仓 server.py 口径）
    expect(Array.isArray(data.phonemes)).toBe(true)
    expect(data.phonemes.length, '2 字符输入应产 2 phonemes').toBe(2)
    const [p0, p1] = data.phonemes
    expect(p0.char).toBe('你')
    expect(p0.start).toBe(0)
    expect(p1.char).toBe('好')

    // per-char 等分验证：last.start + last.duration == duration
    const sum = p1.start + p1.duration
    expect(
      Math.abs(sum - data.duration),
      `per-char 等分: ${p1.start}+${p1.duration} ≈ ${data.duration}`,
    ).toBeLessThan(0.01)

    // audio 是合法 base64 WAV（RIFF 头）
    const audioBytes = Buffer.from(data.audio, 'base64')
    expect(audioBytes.length, 'audio 解码后必须有字节').toBeGreaterThan(1000)
    expect(audioBytes.subarray(0, 4).toString('ascii')).toBe('RIFF')
    expect(audioBytes.subarray(8, 12).toString('ascii')).toBe('WAVE')
  })

  test('#10/#17 数字人区域在聊天页可见 + TTS 链路端到端', async ({ page }) => {
    // AI 回复（LLM 流式）+ 500ms debounce + phonemes 请求断言 60s —— test 级须 > 之
    test.setTimeout(180_000)
    const token = await loginViaAPI(page)

    // 监听 phonemes 请求（验证前端 TTS 链路真的会调它）
    // E2E-F-131（2026-09-24）：F-127/F-132/F-133 修复链路经验 ——
    // 仅断言"发起 ≥1 次请求"会让 502/504 也算 PASS（CI 绿但用户无声音）。
    // 改为同时断言响应 status ∈ {200, 504} 的具体分布（去弱断言）；
    // 若 Nacos 注册竞态导致 504，spec 会显式标 FAIL，便于收口。
    const phonemesRequests: { url: string; status: number; body: string }[] = []
    page.on('response', async (resp) => {
      const req = resp.request()
      if (req.method() === 'POST' && resp.url().includes('/api/v1/tts/phonemes')) {
        let body = ''
        try {
          body = (await resp.text()).slice(0, 200)
        } catch {
          body = ''
        }
        phonemesRequests.push({ url: resp.url(), status: resp.status(), body })
      }
    })

    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    // 注意：数字人 wrapper（.digital-human-wrapper）只在 [id].vue（会话详情页）渲染，
    // new.vue 仅引用 TTS composable 不渲染组件 —— 故断言放在发消息跳转之后。
    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('请用一句话介绍你自己')

    // #10 假动画已删的运行时旁证：发送按钮存在且可点
    //    —— startRandomLipAnimation 的删除由 vitest 字面契约钉死（phoneme.test.ts #10）
    // 等 enabled 再 click（Vue :disabled="!message.trim()" reactive 时序；
    // force:true 在 disabled 按钮上可能不触发 submit —— chromium 首拍实测）
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeVisible({ timeout: 5_000 })
    await expect(sendBtn).toBeEnabled({ timeout: 5_000 })
    await sendBtn.click()

    // 新建会话 → 自动跳转 /chat/conversation/:id（数字人所在页）
    await expect(page).toHaveURL(/\/chat\/conversation\/\d+/, { timeout: 15_000 })
    await waitForHydration(page)

    // #17: 数字人 wrapper 在详情页可见
    // 注意：页面有 2 个 .digital-human-wrapper（[id].vue 外层容器 + DigitalHuman 组件
    // 自身 #digital-human-wrapper）→ strict mode 需 .first()
    const digitalHuman = page.locator('.digital-human-wrapper').first()
    await expect(digitalHuman, '数字人 wrapper 必须可见（[id].vue 渲染）').toBeVisible({
      timeout: 10_000,
    })

    // AI 回复气泡出现
    const aiBubble = page.locator('.dialog-ai .bubble-ai').first()
    await expect(aiBubble).toBeVisible({ timeout: 15_000 })
    await expect(async () => {
      const text = (await aiBubble.textContent())?.trim() ?? ''
      expect(text.length).toBeGreaterThan(0)
    }).toPass({ timeout: 20_000 })

    // [V] 视觉证据：数字人 + 对话同屏
    await page.screenshot({
      path: `${SCREENSHOT_DIR}/e2e-17-digital-human-chat-${test.info().project.name}.png`,
      fullPage: true,
    })

    // TTS 链路：flushTTS 在流完成后 500ms debounce 触发 phonemes 请求
    // （useTTSManager debounce 保留 —— F-129 决策；端点冷推理可达 25s）
    // E2E-F-131：必须 status=200 才算"成功请求"——修前 3 次全是 90012/90017/90044ms 撞底 502/504。
    // F-132 核数修复后 8 核下 40 字 19.9s + 端到端 200 27.6s 实测见 [账本 F-132]。
    // F-138 yaml 超时 180s 容纳 80 字 49 字。期望 ≥1 次 200。
    await expect(async () => {
      expect(
        phonemesRequests.length,
        'AI 回复完成后 500ms debounce 必须发起 /tts/phonemes 请求',
      ).toBeGreaterThanOrEqual(1)
    }).toPass({ timeout: 60_000 })

    // 严格化断言：≥1 次 status=200；若全是 502/504 则标 FAIL（F-131 防退化）
    const okOnes = phonemesRequests.filter((r) => r.status === 200)
    expect(
      okOnes.length,
      `TTS phonemes 至少一次 status=200；当前 ${phonemesRequests.length} 次记录 = ` +
        JSON.stringify(phonemesRequests.map((r) => ({ s: r.status, b: r.body.slice(0, 60) }))) +
        '（E2E-F-131 防弱断言退化）',
    ).toBeGreaterThanOrEqual(1)
  })

  test('#18 数字人 wrapper 在 mobile project 也可见', async ({ page }) => {
    test.setTimeout(120_000)
    await loginViaAPI(page)
    // mobile 同样走「发消息 → 跳转详情页」路径断言数字人
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('移动端数字人可见性验证')
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeEnabled({ timeout: 5_000 })
    await sendBtn.click()
    await expect(page).toHaveURL(/\/chat\/conversation\/\d+/, { timeout: 15_000 })
    await waitForHydration(page)

    await expect(page.locator('.digital-human-wrapper').first()).toBeVisible({ timeout: 10_000 })

    // [V] mobile 视觉证据
    await page.screenshot({
      path: `${SCREENSHOT_DIR}/e2e-17-digital-human-mobile-${test.info().project.name}.png`,
      fullPage: true,
    })
  })
})