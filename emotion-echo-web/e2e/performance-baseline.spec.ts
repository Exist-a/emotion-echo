import { test, expect } from '@playwright/test'
import { mkdirSync, writeFileSync, appendFileSync } from 'node:fs'
import { join } from 'node:path'

/**
 * E2E-28 性能与延迟基线 回归钉（plan §4 #9 / #12 / #13 / #17 / #18 / #19）
 *
 * 覆盖：
 *   #9  [V] 浏览器流式视觉：首 token 计时 + 渐进增长采样（vs 整段缓冲）
 *   #12 段间 gap：两段音频 ended→playing 间隙 <1000ms（F-129/F-134 链）
 *   #13 端到端分解：点发送 → TTS 首请求 → SSE 完成（F-134 生效证据：
 *       TTS 首请求应早于 SSE 完成 —— 段到标点即发）
 *   #17/#18 [V] Grafana p95 面板渲染 + 截图
 *   #19 回归钉：浏览器 TTFT < 30s（宽松上限，拦"坏了"不拦"变慢"）
 *
 * 测量点声明（与 plan #4/#13 判读一致）：本 spec 计时为**浏览器端**
 * （点击时刻 → 首 token 渲染），含 前端调度+网关+BFF+LLM+回传+渲染，
 * 与服务端埋点（F-193 测点差）不可直接互比。
 *
 * 前置：dev 栈健康（RUNBOOK §2.1）、本地 :3000 dev server 或 webServer
 * 自动起、xtts healthy（#12/#13）。headless 自动播放：本文件显式声明
 * --autoplay-policy=no-user-gesture-required（#12 需真实播放）。
 */

// 音频自动播放（headless Chrome 默认拦 play()；#12 需真实播放推进）
test.use({
  launchOptions: {
    args: ['--autoplay-policy=no-user-gesture-required'],
  },
})

const API_BASE = 'http://localhost:19080'
const GRAFANA = 'http://localhost:13000'
const DEMO = { username: 'echo', password: 'echo123' }
const STAGE_DIR = join(
  process.cwd(), '..', 'docs', 'e2e-roadmap', 'stages', 'e2e-28-performance-baseline',
)
const SHOT_DIR = join(STAGE_DIR, 'screenshots')
const BASELINE_DIR = join(STAGE_DIR, 'baseline')
mkdirSync(SHOT_DIR, { recursive: true })
mkdirSync(BASELINE_DIR, { recursive: true })

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

/** 只读观测探针 v2：
 *  实测坑（#12 首轮 0/0 真因，探针缺陷非产品缺陷）：useTTSPlayer 用
 *  `new Audio(url)`（useTTSPlayer.ts:241）**脱离 DOM** ⇒ document 捕获
 *  监听器永远收不到它的 playing/ended。改为**构造器登记 + 状态轮询**：
 *  - 包装 window.Audio 登记实例进 __audioEls（透传参数、不改行为）
 *  - 轮询读 paused/currentTime/ended（属性读取零副作用），
 *    浏览器侧打 firstEnded / firstPlayingAfter 时间戳（performance.now）
 *  同时保留 document 捕获监听（若未来改 DOM 内 audio 也能抓）。 */
const EVENT_PROBE = `
window.__ttsEvents = [];
window.__audioEls = [];
;((orig) => {
  window.Audio = function (...args) {
    const el = new orig(...args);
    const idx = window.__audioEls.length;
    window.__audioEls.push(el);
    // 直挂实例事件：毫秒级时间戳（轮询粒度 100ms 在 1000ms 阈值边界
    // 不足以判 1032ms vs 932ms —— 二轮提精度）
    el.addEventListener('playing', () => {
      window.__ttsEvents.push({ type: 'playing', i: idx, t: performance.now() });
    });
    el.addEventListener('ended', () => {
      window.__ttsEvents.push({ type: 'ended', i: idx, t: performance.now() });
    });
    return el;
  };
})(window.Audio);
document.addEventListener('playing', (e) => {
  if (e.target && e.target.tagName === 'AUDIO') {
    window.__ttsEvents.push({ type: 'playing', t: performance.now() });
  }
}, true);
document.addEventListener('ended', (e) => {
  if (e.target && e.target.tagName === 'AUDIO') {
    window.__ttsEvents.push({ type: 'ended', t: performance.now() });
  }
}, true);
true`

const AUDIO_SNAPSHOT = `(() => {
  const els = (window).__audioEls || [];
  const now = performance.now();
  const st = els.map((a) => ({
    ct: +(a.currentTime || 0).toFixed(2),
    dur: +(a.duration || 0).toFixed(2),
    paused: !!a.paused,
    ended: !!a.ended,
  }));
  window.__marks = window.__marks || { firstEnded: -1, nextPlaying: -1 };
  if (window.__marks.firstEnded < 0 && st.some((s) => s.ended)) {
    window.__marks.firstEnded = now;
  }
  if (window.__marks.firstEnded >= 0 && window.__marks.nextPlaying < 0
      && st.some((s) => !s.paused && s.ct > 0 && !s.ended)) {
    window.__marks.nextPlaying = now;
  }
  return {
    created: st.length,
    ended: st.filter((s) => s.ended).length,
    playing: st.filter((s) => !s.paused && s.ct > 0).length,
    marks: window.__marks,
    states: st,
  };
})()`

test.describe('E2E-28 性能与延迟基线 回归钉', () => {
  test('#19/#9 浏览器 TTFT<30s + 渐进流式渲染 + [V] 截图', async ({ page }) => {
    test.setTimeout(180_000)
    await loginViaAPI(page)
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })

    const t0 = Date.now()
    await textarea.fill('请写一首关于春天的五言绝句，并逐句解释每一句的含义，越详细越好。')
    // fill → 等 Vue 反应式把按钮从 disabled 翻启用（立即点会静默不发，实测坑）
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeEnabled({ timeout: 5_000 })
    await sendBtn.click({ force: true })

    const aiBubble = page.locator('.dialog-ai .bubble-ai').first()
    await expect(aiBubble).toBeVisible({ timeout: 15_000 })

    // 首 token（浏览器端 TTFT = 点击 → 首个非空渲染）
    await expect
      .poll(async () => (await aiBubble.textContent())?.trim().length ?? 0, {
        timeout: 30_000,
        message: '首 token 应在 30s 内渲染（#19 回归上限）',
      })
      .toBeGreaterThan(0)
    const ttftBrowser = Date.now() - t0

    // 渐进增长采样（#9）：每 200ms 采长度，直到 3s 无变化（完成）或 120s 上限
    const growth: number[] = []
    let last = -1
    let stable = 0
    const deadline = Date.now() + 120_000
    while (Date.now() < deadline && stable < 15) {
      const len = (await aiBubble.textContent())?.trim().length ?? 0
      if (len !== last) {
        growth.push(len)
        last = len
        stable = 0
      } else {
        stable += 1
      }
      await page.waitForTimeout(200)
    }

    await page.screenshot({
      path: join(SHOT_DIR, '28-09-streaming-bubble.png'),
      fullPage: false,
    })

    const artifact = { ttftBrowserMs: ttftBrowser, growthSteps: growth.length, growth }
    appendFileSync(join(BASELINE_DIR, 'browser_stream_timing.json'), JSON.stringify(artifact) + '\n')
    console.log('[#9/#19]', JSON.stringify(artifact))

    // #19：回归上限（拦"坏了"）
    expect(ttftBrowser).toBeLessThan(30_000)
    // #9：渐进而非一次性 —— ≥3 个增长步（整段缓冲会是 1~2 步跳变）
    expect(growth.length, `渐进增长步数不足: ${JSON.stringify(growth)}`).toBeGreaterThanOrEqual(3)
  })

  test('#13 端到端分解：TTS 首请求早于 SSE 完成（F-134 生效证据）', async ({ page }) => {
    test.setTimeout(240_000)
    await loginViaAPI(page)
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    let tTtsFirst = -1
    let tSseFinish = -1
    const t0 = Date.now()
    page.on('request', (req) => {
      if (tTtsFirst < 0 && req.url().includes('/api/v1/tts/')) {
        tTtsFirst = Date.now() - t0
      }
    })
    page.on('requestfinished', (req) => {
      if (tSseFinish < 0 && req.url().includes('/api/v1/ai/stream')) {
        tSseFinish = Date.now() - t0
      }
    })

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('请分三句介绍北京的三个著名景点，每句以句号结尾。')
    // fill → 等 Vue 反应式把按钮从 disabled 翻启用（立即点会静默不发，实测坑）
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeEnabled({ timeout: 5_000 })
    await sendBtn.click({ force: true })

    // 等 TTS 首请求出现（F-134 段到标点即发；同时段 SSE 可能仍在流）
    await expect
      .poll(() => tTtsFirst, { timeout: 120_000, message: 'TTS 首请求应出现（F-134 切段触发）' })
      .toBeGreaterThan(0)
    // SSE 完成（流尾）
    await expect
      .poll(() => tSseFinish, { timeout: 180_000, message: 'SSE 流应完成' })
      .toBeGreaterThan(0)

    const artifact = { t0: 0, ttsFirstMs: tTtsFirst, sseFinishMs: tSseFinish }
    appendFileSync(join(BASELINE_DIR, 'e2e_decomposition.json'), JSON.stringify(artifact) + '\n')
    console.log('[#13]', JSON.stringify(artifact))

    // F-134 生效：多句 prompt 下首段在流完成前就发出（切段即 TTS）
    expect(
      tTtsFirst,
      `TTS 首请求(${tTtsFirst}ms) 应早于 SSE 完成(${tSseFinish}ms) —— F-134 切段未生效或回复单段`,
    ).toBeLessThan(tSseFinish)
  })

  test('#12 段间 gap < 1000ms（两段音频衔接）', async ({ page }) => {
    test.setTimeout(300_000)
    await page.addInitScript(EVENT_PROBE)
    await loginViaAPI(page)
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('请用三句完整的话介绍上海：第一句讲外滩，第二句讲浦东，第三句讲小吃。每句以句号结尾。')
    // fill → 等 Vue 反应式把按钮从 disabled 翻启用（立即点会静默不发，实测坑）
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeEnabled({ timeout: 5_000 })
    await sendBtn.click({ force: true })

    // 等到 ≥1 段 ended 且**不同实例**的下一段已 playing（实例直挂事件，
    // 毫秒级精度；TTS 段级排队热态每段 ~10-30s ⇒ 总时长可能 60s+）
    let ev: any[] = []
    await expect
      .poll(
        async () => {
          ev = await page.evaluate(() => (window as any).__ttsEvents ?? [])
          const ended1 = ev.find((e: any) => e.type === 'ended')
          const nextPlay = ended1 && ev.find(
            (e: any) => e.type === 'playing' && e.t > ended1.t && e.i !== ended1.i,
          )
          return ended1 && nextPlay ? 'ready' : `${ev.filter((e: any) => e.type === 'ended').length}/${ev.filter((e: any) => e.type === 'playing').length}`
        },
        { timeout: 240_000, message: '应有 ≥1 段 ended 且下一段（不同实例）playing' },
      )
      .toBe('ready')

    const ended1 = ev.find((e: any) => e.type === 'ended')
    const nextPlay = ev.find((e: any) => e.type === 'playing' && e.t > ended1.t && e.i !== ended1.i)
    const gap = nextPlay.t - ended1.t

    const snap = await page.evaluate(AUDIO_SNAPSHOT)
    appendFileSync(
      join(BASELINE_DIR, 'segment_gap.json'),
      JSON.stringify({ gapMs: Math.round(gap), events: ev, snap }) + '\n',
    )
    console.log('[#12] gap =', Math.round(gap), 'ms events=', JSON.stringify(ev))

    await page.screenshot({ path: join(SHOT_DIR, '28-12-segment-gap.png'), fullPage: false })
    expect(gap, `段间 gap ${Math.round(gap)}ms ≥ 1000ms（实例事件毫秒级）`).toBeLessThan(1000)
  })

  test('#17/#18 [V] Grafana p95 延迟面板渲染 + 截图', async ({ page }) => {
    test.setTimeout(120_000)
    await page.goto(`${GRAFANA}/login`)
    await page.locator('input[name="user"]').fill('admin')
    await page.locator('input[name="password"]').fill('admin')
    await page.locator('button[type="submit"]').first().click()
    await page.waitForURL('**/graph', { timeout: 30_000 }).catch(() => {})

    // 找 overview 仪表盘（E2E-22 D1 面板：HTTP p95 Latency）
    // 用 page.evaluate(fetch) 而非 page.request：宿主端口对 Playwright
    // APIRequestContext 间歇 ECONNRESET（F-192 同族，curl 200 对照），
    // E2E-17 mobile 502 先例同解法。fetch 走浏览器栈 + 页面已登录同源 cookie。
    const search = await page.evaluate(async () => {
      const r = await fetch('/api/search?query=emotion', { credentials: 'include' })
      return { ok: r.ok, body: await r.json().catch(() => null) }
    })
    expect(search.ok, 'Grafana search API 应可达').toBe(true)
    const hits = search.body as any[]
    // search 返回含 dash-folder（首轮 hits[0] 是文件夹 → /d/<folder-uid>
    // = Dashboard not found，面板永不渲染 —— 必须过滤 type）
    const dash = (hits as any[]).find((h) => h.type === 'dash-db')
    expect(dash, `应有 dash-db 类型仪表盘: ${JSON.stringify(hits.map((h: any) => h.type))}`).toBeTruthy()
    const uid = dash!.uid
    await page.goto(`${GRAFANA}/d/${uid}?orgId=1&from=now-6h&to=now`)
    await page.waitForLoadState('networkidle', { timeout: 30_000 }).catch(() => {})
    // 面板渲染完成信号：等 panel 容器或标题出现（首载 bundle 慢，裸 sleep 不可靠）
    await page
      .locator('[data-testid="dashboard-panel"], .panel-container, .react-grid-item')
      .first()
      .waitFor({ timeout: 30_000 })
      .catch(() => {})

    const bodyText = (await page.locator('body').textContent()) ?? ''
    expect(bodyText, '面板页应含 p95/延迟面板标题').toMatch(/p95|延迟|Latency/i)

    await page.screenshot({
      path: join(SHOT_DIR, '28-17-grafana-p95-panel.png'),
      fullPage: false,
    })
  })
})
