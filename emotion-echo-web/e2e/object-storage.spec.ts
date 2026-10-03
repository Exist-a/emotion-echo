/**
 * E2E-27 回归钉（测试点 #15 / #19 / #20 的自动化形态）：
 * 对象存储 URL 契约（网关相对化 F-116/M1）与头像浏览器渲染。
 *
 * 运行方式：
 *   pnpm playwright test e2e/object-storage.spec.ts --project=chromium
 *   （webServer 由 playwright.config 自动起 pnpm dev；或 BASE_URL=http://localhost:3000 指定已在跑的）
 *
 * 前置：dev 栈运行中（BFF/APISIX/MinIO），演示账号 echo/echo123。
 *
 * 背景（E2E-26 F-184 教训沿用）：IAB 截图存在渲染帧与 DOM 不同步的失真，
 * [V] 视觉证据由本 spec 的独立 Chromium 渲染栈产出。
 */
import { test, expect, request } from '@playwright/test'
import * as path from 'node:path'
import * as fs from 'node:fs'
import { fileURLToPath } from 'node:url'

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }
const SHOTS_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../docs/e2e-roadmap/stages/e2e-27-object-storage-minio/screenshots',
)

const TINY_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
)

async function loginApi(): Promise<{ api: ReturnType<typeof request.newContext>; token: string }> {
  const api = await request.newContext({ baseURL: API_BASE })
  const login = await api.post('/api/v1/auth/login', { data: DEMO, timeout: 15000 })
  expect(login.status(), 'login must succeed').toBe(200)
  const token = (await login.json()).data.accessToken as string
  expect(token).toBeTruthy()
  return { api, token }
}

test.describe('E2E-27 对象存储回归钉', () => {
  test.setTimeout(180_000)

  // ============ #15：F-116 修复的 API 契约（RED 曾断言旧绝对地址必红） ============
  test('API: 上传返回网关相对 url + 反代端点 GET/HEAD 200 + 404/400 负向', async () => {
    const { api, token } = await loginApi()
    const auth = { Authorization: `Bearer ${token}` }

    // 上传 → 相对 url（不得下发 PublicBaseURL 绝对地址）
    const up = await api.post('/api/v1/uploads/image', {
      headers: auth,
      multipart: { file: { name: 'e2e27-pin.png', mimeType: 'image/png', buffer: TINY_PNG } },
      timeout: 30000,
    })
    expect(up.status(), 'upload must 200').toBe(200)
    const url = (await up.json()).data.url as string
    expect(url, 'F-116/M1: url 必须是网关相对路径').toMatch(/^\/api\/v1\/uploads\/file\//)
    expect(url, '不得含存储宿主绝对地址').not.toContain('localhost:9000')

    // 反代端点：GET 200（真实 Content-Type）+ HEAD 200（gin 不自动转发 HEAD 的回归）
    const get = await api.get(url, { headers: auth, timeout: 15000 })
    expect(get.status(), 'GET 反代必须 200').toBe(200)
    expect(get.headers()['content-type'], '回读真实 Content-Type').toContain('image/png')

    const head = await api.head(url, { headers: auth, timeout: 15000 })
    expect(head.status(), 'HEAD 必须与 GET 同语义 200（seed 方法表 + gin 双注册回归）').toBe(200)

    // 负向：缺失对象 404（非路由级空 404——断言 JSON code 字段）
    const miss = await api.get('/api/v1/uploads/file/nonexistent-e2e27.png', {
      headers: auth,
      timeout: 15000,
    })
    expect(miss.status(), '缺失对象必须 404（ADR 决策 3）').toBe(404)
    expect(await miss.text(), '必须是 handler JSON 而非 gin 空 404').toContain('"code"')

    // 负向：含 .. 的 key 400（不触达 storage）
    const trav = await api.get('/api/v1/uploads/file/..vhidden', { headers: auth, timeout: 15000 })
    expect(trav.status(), 'path traversal 必须 400').toBe(400)

    await api.dispose()
  })

  // ============ #19 [V]：浏览器发起者视角头像渲染 ============
  test('#19 [V] 头像渲染：相对 URL 解析 + naturalWidth>0 + 截图', async ({ page }) => {
    fs.mkdirSync(SHOTS_DIR, { recursive: true })
    const login = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
    expect(login.ok()).toBe(true)
    const token = (await login.json()).data.accessToken as string
    await page.context().addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])

    // 前置：先经 API 换上**合法**头像（dev 存量对象可能是历史测试垃圾字节，
    // naturalWidth 断言对损坏图片恒 0——与 URL 契约无关的假失败）
    const avUp = await page.request.post(`${API_BASE}/api/v1/user/avatar`, {
      headers: { Authorization: `Bearer ${token}` },
      multipart: { avatar: { name: 'e2e27-valid.png', mimeType: 'image/png', buffer: TINY_PNG } },
      timeout: 30000,
    })
    expect(avUp.status(), '前置头像上传必须 200').toBe(200)
    const avatarUrl = (await avUp.json()).data.avatar as string
    expect(avatarUrl).toMatch(/^\/api\/v1\/user\/avatar\/image\//)

    // networkidle 而非固定 sleep：nuxt dev 冷编译页面 chunk 时 onMounted 的
    // fetchUserInfo 会晚于 4s（诊断实测：固定 4~6s 抓到默认头像，networkidle 后正常）
    await page.goto('/chat/user', { waitUntil: 'networkidle', timeout: 60000 })
    await expect
      .poll(
        async () => (await page.locator('img.avatar').first().getAttribute('src')) ?? '',
        { timeout: 20000, message: '头像 src 必须被 fetchUserInfo 更新为网关地址' },
      )
      .toContain('localhost:19080')

    const img = page.locator('img.avatar').first()
    await expect(img, '我的空间头像 <img> 必须可见').toBeVisible({ timeout: 10000 })

    // 解析结果必须是网关绝对地址（resolveObjectUrl 生效——裸 /api/ 打 :3000 无代理会 404）
    const src = (await img.getAttribute('src')) ?? ''
    expect(src, 'src 必须经 resolveObjectUrl 解析为网关地址').toContain('localhost:19080')
    expect(src, 'src 必须指向反代端点').toContain('/api/v1/user/avatar/image/')

    // 发起者视角可达的最终判据：图片真实解码（naturalWidth>0 而非 URL 字符串相等）
    const natural = await img.evaluate((el) => (el as HTMLImageElement).naturalWidth)
    expect(natural, '头像必须真实加载解码（naturalWidth>0）——URL 字符串对但加载失败不算过').toBeGreaterThan(0)

    const shot = path.join(SHOTS_DIR, '27-19-avatar-render.png')
    await page.screenshot({ path: shot, fullPage: false })
    expect(fs.existsSync(shot), '截图必须落盘').toBe(true)

    // ============ #19b：UI 上传 → 预览 → 刷新后仍渲染 ============
    // 头像 input 在「修改资料」弹框内（v-if dialogFormVisible 门控）——先开弹框
    await page.getByRole('button', { name: '修改资料' }).click()
    await page.locator('.ms-dialog [role="dialog"], [role="dialog"]').first().waitFor({ timeout: 8000 })
    const input = page.locator('input.avatar-input')
    await input.setInputFiles({
      name: 'e2e27-ui.png',
      mimeType: 'image/png',
      buffer: TINY_PNG,
    })
    // 上传成功：预览图（avatarPreviewSrc）换成本轮对象且可解码
    await page.waitForTimeout(3500)
    const preview = page.locator('img.avatar').nth(1)
    await expect(preview).toBeVisible()
    const previewSrc = (await preview.getAttribute('src')) ?? ''
    expect(previewSrc, '预览必须解析到网关（blob 本地预览或相对→网关均可，绝不裸 /api/）').not.toMatch(/^\/api\//)
    const previewNatural = await preview.evaluate((el) => (el as HTMLImageElement).naturalWidth)
    expect(previewNatural, '预览图必须真实解码').toBeGreaterThan(0)

    // 刷新：store 回读 profile（相对形态）→ resolveObjectUrl → 仍渲染
    await page.reload({ waitUntil: 'networkidle', timeout: 60000 }).catch(() => {})
    await expect
      .poll(
        async () => (await page.locator('img.avatar').first().getAttribute('src')) ?? '',
        { timeout: 20000, message: '刷新后 src 必须被重新解析为网关地址' },
      )
      .toContain('localhost:19080')
    const img2 = page.locator('img.avatar').first()
    const natural2 = await img2.evaluate((el) => (el as HTMLImageElement).naturalWidth)
    expect(natural2, '刷新后头像仍真实解码').toBeGreaterThan(0)

    const shot2 = path.join(SHOTS_DIR, '27-20-avatar-upload-refresh.png')
    await page.screenshot({ path: shot2, fullPage: false })
    expect(fs.existsSync(shot2), '刷新后截图必须落盘').toBe(true)
  })
})
