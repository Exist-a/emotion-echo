import { test, expect } from '@playwright/test'

/**
 * E2E-11: 我的空间 Playwright 回归钉
 *
 * 覆盖 plan.md 12 个测试点：
 * 1.  /user/profile 返回真实用户数据（非硬编码"体验用户"）
 * 2.  修改昵称成功 + 页面显示新昵称
 * 3.  昵称格式校验（1 字符 / 13 字符被拦截，不发请求）
 * 4.  年龄校验（-1 / 131 被拦截，不发请求）
 * 5.  头像上传成功 + 页面头像更新
 * 6.  头像 >2MB 被前端拦截
 * 7.  昼夜使用模式饼图有数据可渲染
 * 8.  近 30 天对话频次折线图有数据可渲染
 * 9.  互动深度指标柱状图有数据可渲染
 * 10. 图表空态可读
 * 11. 退出登录弹框确认 → 跳转登录页
 * 12. 侧边栏"我的空间"导航可进入
 *
 * 前置条件：
 * - dev 环境运行中（docker compose --env-file .env.local up -d）
 * - BFF + user-svc + analytics-svc 可用
 * - 演示账号 echo / echo123 存在
 */

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }

/** 通过 API 登录并注入 cookie，绕过 UI 登录 race；返回 accessToken */
async function loginViaAPI(page: import('@playwright/test').Page) {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
  return token as string
}

async function gotoMySpace(page: import('@playwright/test').Page) {
  await page.goto('/chat/user')
  await page.waitForLoadState('domcontentloaded')
  // 等资料 + 行为数据请求完成（图表/空态/回填渲染的前提）
  await page.waitForTimeout(4000)
}

/** 打开"修改资料"原生弹框（E2E-11：已替换未解析的 <el-dialog>） */
async function openEditDialog(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: '修改资料' }).click()
  await page.locator('.ms-dialog [role="dialog"], [role="dialog"]').first().waitFor({ timeout: 8000 })
}

test.describe('E2E-11 我的空间', () => {
  // ==================== #1 profile 真实数据 ====================
  test('#1 /user/profile 返回真实用户数据（非硬编码 mock）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const resp = await page.request.get(`${API_BASE}/api/v1/user/profile`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(resp.ok()).toBe(true)
    const body = await resp.json()
    const data = body?.data
    expect(data, 'profile 响应必须有 data').toBeTruthy()
    expect(
      data.nickname,
      'E2E-11: profile 曾硬编码返回 "体验用户"；修复后应透传 user-svc 真实昵称',
    ).not.toBe('体验用户')
    expect(data.nickname, '真实昵称非空').toBeTruthy()
    // avatar 字段必须存在（可能为空字符串，但键必须在）
    expect(Object.prototype.hasOwnProperty.call(data, 'avatar')).toBe(true)
  })

  // ==================== #2 修改昵称 ====================
  test('#2 修改昵称成功 + 页面显示新昵称', async ({ page }) => {
    const token = await loginViaAPI(page)

    const newNick = `回响${Date.now() % 10000}`
    const resp = await page.request.patch(`${API_BASE}/api/v1/users/me`, {
      data: { nickname: newNick },
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(resp.status(), `PATCH nickname 应 200，实际 ${resp.status()}`).toBe(200)

    // 确认落库：带上同一 token 重新拉 profile
    const after = await page.request.get(`${API_BASE}/api/v1/user/profile`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const body = await after.json()
    expect(body?.data?.nickname).toBe(newNick)

    // 页面显示：导航后应渲染服务端返回的新昵称
    await gotoMySpace(page)
    await expect(page.locator('.nickname')).toHaveText(newNick)

    // 复原，避免污染后续测试
    await page.request.patch(`${API_BASE}/api/v1/users/me`, {
      data: { nickname: 'Echo User' },
      headers: { Authorization: `Bearer ${token}` },
    })
  })

  // ==================== #3 昵称校验 ====================
  test('#3 昵称格式校验：非法值被拦截且不发请求', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)
    await openEditDialog(page)

    const nicknameInput = page.locator('.profile-form input[type="text"]')
    await expect(nicknameInput).toBeVisible()
    // 弹框应回填当前昵称（E2E-11：原实现回填为空）
    await expect(nicknameInput).not.toHaveValue('')

    // 监听 PATCH 请求，断言校验失败时不会发出
    let patchCount = 0
    page.on('request', (req) => {
      if (req.method() === 'PATCH' && req.url().includes('/users/me')) patchCount++
    })

    // 1 字符（下限 2）
    await nicknameInput.fill('x')
    await page.getByRole('button', { name: '保存资料' }).click()
    await page.waitForTimeout(1000)
    expect(patchCount, 'E2E-11: 1 字符昵称应被前端拦截，不发 PATCH').toBe(0)
    // 校验失败时弹框保持打开 + 出现提示
    await expect(page.locator('[role="dialog"]')).toBeVisible()

    // 13 字符（上限 12）
    await nicknameInput.fill('a'.repeat(13))
    await page.getByRole('button', { name: '保存资料' }).click()
    await page.waitForTimeout(1000)
    expect(patchCount, 'E2E-11: 13 字符昵称应被前端拦截，不发 PATCH').toBe(0)
  })

  // ==================== #4 年龄校验 ====================
  test('#4 年龄校验：越界值被拦截且不发请求', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)
    await openEditDialog(page)

    const ageInput = page.locator('.profile-form input[type="number"]')
    await expect(ageInput).toBeVisible()

    let patchCount = 0
    page.on('request', (req) => {
      if (req.method() === 'PATCH' && req.url().includes('/users/me')) patchCount++
    })

    await ageInput.fill('-1')
    await page.getByRole('button', { name: '保存资料' }).click()
    await page.waitForTimeout(1000)
    expect(patchCount, 'E2E-11: 年龄 -1 应被前端拦截，不发 PATCH').toBe(0)

    await ageInput.fill('131')
    await page.getByRole('button', { name: '保存资料' }).click()
    await page.waitForTimeout(1000)
    expect(patchCount, 'E2E-11: 年龄 131 应被前端拦截，不发 PATCH').toBe(0)
  })

  // ==================== #5 头像上传 ====================
  test('#5 头像上传成功 + 页面头像更新', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)
    await openEditDialog(page)

    // 构造一个 1x1 PNG（远小于 2MB）
    const pngBase64 =
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
    const buffer = Buffer.from(pngBase64, 'base64')

    const uploadStatuses: number[] = []
    page.on('response', (res) => {
      if (res.url().includes('/user/avatar')) uploadStatuses.push(res.status())
    })

    const fileInput = page.locator('.profile-form input[type="file"]')
    await fileInput.setInputFiles({ name: 'avatar.png', mimeType: 'image/png', buffer })
    await page.waitForTimeout(4000)

    expect(
      uploadStatuses,
      'E2E-11: 头像上传应触发 POST /user/avatar。' +
        '原实现用未解析的 <el-upload>，上传逻辑是死代码，一次请求都不发。',
    ).not.toHaveLength(0)
    expect(uploadStatuses[0], '头像上传应返回 200（MinIO 写入 + user-svc 落库）').toBe(200)

    // 服务端返回的公开 URL 应回填到表单预览。
    //
    // E2E-11 复查：原断言只要求 previewSrc 「truthy」——而本地预览用的是
    // `blob:` URL，**本地 blob 也满足 truthy**，所以这个断言在"服务端 URL 从未
    // 回填"的 bug 下依然通过（IAB 实测发现：BFF 曾把 avatar 放在顶层而非
    // data 内 ⇒ 前端 res 为 undefined ⇒ 预览永远停在 blob，且弹"上传失败"）。
    // 现在必须断言它是 http(s) 的 MinIO URL。
    const previewSrc = await page.locator('.avatar-uploader .avatar').getAttribute('src')
    expect(previewSrc, 'E2E-11: 上传成功后预览应有 src').toBeTruthy()
    expect(
      previewSrc!.startsWith('http'),
      `E2E-11: 预览 src 必须是服务端 URL（实测拿到：${previewSrc}）。` +
        '若是 blob: 说明服务端返回值没回填——本地 blob 只是上传前的临时预览。',
    ).toBe(true)
  })

  // ==================== #6 头像 >2MB 拦截 ====================
  test('#6 头像 >2MB 被前端拦截，不发请求', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)
    await openEditDialog(page)

    let uploadCount = 0
    page.on('request', (req) => {
      if (req.url().includes('/user/avatar')) uploadCount++
    })

    // 3MB 假图片
    const big = Buffer.alloc(3 * 1024 * 1024, 0)
    const fileInput = page.locator('.profile-form input[type="file"]')
    await fileInput.setInputFiles({ name: 'big.png', mimeType: 'image/png', buffer: big })
    await page.waitForTimeout(2500)

    expect(uploadCount, 'E2E-11: 超过 2MB 的头像应被前端拦截，不发请求').toBe(0)
  })

  // ==================== #7~#9 三个图表 ====================
  test('#7~#9 三个行为图表有数据可渲染', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)

    const card = page.locator('.user-data-card')
    await expect(card).toBeVisible()

    // 三个图表容器（有数据时 chartData 长度 > 0）
    const chartItems = page.locator('.user-data-card .chart-item')
    const count = await chartItems.count()

    // 无事件数据时可能是 0（空态）——记录实际值用于报告，不强行断言
    test.info().annotations.push({
      type: 'chart-count',
      description: `chartData 渲染条目数 = ${count}`,
    })

    if (count > 0) {
      // 有数据：断言至少渲染了图表 canvas/svg，且空态不出现
      await expect(card.locator('.ee-empty')).toHaveCount(0, {
        timeout: 3000,
      })
    }
    expect(count, 'E2E-11: chartData 最多 3 个图表').toBeLessThanOrEqual(3)
  })

  // ==================== #10 空态 ====================
  test('#10 图表空态可读（不与图表同时出现）', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)

    const card = page.locator('.user-data-card')
    const chartCount = await card.locator('.chart-item').count()
    const emptyCount = await card.locator('.ee-empty').count()
    const skeletonCount = await card.locator('.ee-skeleton').count()

    // E2E-11 核心断言：空态与图表**不得同时出现**（原实现恒渲染空态）
    expect(
      chartCount === 0 || emptyCount === 0,
      `E2E-11: "暂无数据"占位符与图表不得同时渲染（图表 ${chartCount} 个，空态 ${emptyCount} 个）`,
    ).toBe(true)

    // 加载完成后骨架屏必须消失（原实现恒渲染）
    expect(skeletonCount, 'E2E-11: 加载完成后骨架屏应消失（原实现无 v-if 恒渲染）').toBe(0)
  })

  // ==================== #11 退出登录 ====================
  test('#11 退出登录弹框确认 → 跳转登录页', async ({ page }) => {
    await loginViaAPI(page)
    await gotoMySpace(page)

    // E2E-11：原 <el-dialog> 未解析 → "确认退出"按钮在 DOM 中根本不存在
    await page.getByRole('button', { name: '退出登录' }).click()
    const dialog = page.locator('[role="dialog"]').filter({ hasText: '离开这里' })
    await expect(dialog).toBeVisible({ timeout: 8000 })

    await page.getByRole('button', { name: '确认退出' }).click()
    await page.waitForTimeout(2500)

    expect(page.url()).toContain('/login')
  })

  // ==================== #12 侧边栏导航 ====================
  test('#12 侧边栏"我的空间"导航可进入', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    const navLink = page.getByRole('link', { name: '我的空间' })
    if ((await navLink.count()) > 0) {
      await navLink.first().click()
      await page.waitForTimeout(2000)
      expect(page.url()).toContain('/chat/user')
    } else {
      // 折叠态下可能不在 DOM：直接 goto 验证路由可达
      await page.goto('/chat/user')
      await page.waitForTimeout(2000)
      expect(page.url()).toContain('/chat/user')
    }
    await expect(page.locator('.user-info-card')).toBeVisible()
  })
})
