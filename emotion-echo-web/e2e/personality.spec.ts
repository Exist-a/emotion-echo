import { test, expect } from '@playwright/test'

/**
 * E2E-14: 人格量表与 AI 提示词定制 Playwright 回归钉
 *
 * 覆盖 plan.md 中浏览器/HTTP 可观测的测试点：
 * #1  人格量表种子数据已就位（category='personality'）
 * #2  人格量表种子数据结构正确（30 题，每题 5 选项）
 * #4  人格量表提交成功（返回五维度分数）
 * #5  结果弹窗展示维度分数（雷达图 + 维度明细），不显示"等级"
 * #6  /question 页按 category 分 tab
 * #7  切换 tab 后列表正确筛选
 * #8  user 页人格维度雷达图正确渲染
 * #11 人格量表结果持久化（factorScores 落库可读回）
 * #12 未完成人格量表时 AI 仍正常回复（无画像降级路径）
 *
 * 注：#9「AI 请求 system prompt 含人格维度」在 Go 单测层验证
 * （ai_stream_personality_test.go 用 fake streamer 捕获 messages）——
 * system prompt 由 BFF 服务端组装，浏览器网络面板看不到，无法在此断言。
 * #10「AI 回复体现实个性化」属主观判定，由执行记录人工裁定。
 *
 * 前置条件：
 * - dev 环境运行中（docker compose --env-file .env.local up -d）
 * - 种子数据 deploy/db/06-seed-surveys.sql 已执行（含 BIG5）
 */

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }
/** 阶段证据目录（相对 emotion-echo-web/，与 docs/e2e-roadmap/stages/<stage>/screenshots 对齐） */
const SHOTS = '../docs/e2e-roadmap/stages/e2e-14-personality-ai-prompt/screenshots'
/** 截图文件名带 project 名：chromium 与 mobile 两次运行否则会互相覆盖 */
const shot = (name: string) => `${SHOTS}/${name}-${test.info().project.name}.png`

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` }
}

async function loginViaAPI(page: import('@playwright/test').Page): Promise<string> {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
  return token as string
}

/** 从列表 API 找人格量表的 id */
async function findPersonalitySurveyId(
  page: import('@playwright/test').Page,
  token: string,
): Promise<number> {
  const resp = await page.request.get(`${API_BASE}/api/v1/surveys`, {
    headers: authHeaders(token),
  })
  expect(resp.ok()).toBe(true)
  const body = await resp.json()
  const items: any[] = body?.data?.items ?? []
  const personality = items.find((it) => it.category === 'personality')
  expect(personality, '种子数据必须含 category=personality 的量表').toBeTruthy()
  return personality.id
}

/** 全选第一项（每题第一个选项）并提交，返回提交响应体 */
async function answerAllAndSubmit(page: import('@playwright/test').Page) {
  const questions = page.locator('.question-block')
  const count = await questions.count()
  for (let i = 0; i < count; i++) {
    await questions.nth(i).locator('.radio-option').first().click()
  }
  const submitBtn = page.locator('button', { hasText: '提交这份答卷' })
  await expect(submitBtn).toBeEnabled({ timeout: 5000 })
  const submitPromise = page.waitForResponse(
    (resp) =>
      resp.url().includes('/api/v1/surveys/') &&
      resp.url().includes('/submit') &&
      resp.request().method() === 'POST',
  )
  await submitBtn.click()
  const submitResp = await submitPromise
  expect(submitResp.ok(), '人格量表提交必须 200').toBe(true)
  return await submitResp.json()
}

test.describe('E2E-14 人格量表与 AI 提示词定制', () => {
  // ==================== #1 种子数据 ====================
  test('#1 列表 API 含 category=personality 的量表', async ({ page }) => {
    const token = await loginViaAPI(page)
    const resp = await page.request.get(`${API_BASE}/api/v1/surveys`, {
      headers: authHeaders(token),
    })
    expect(resp.ok()).toBe(true)
    const body = await resp.json()
    const items: any[] = body?.data?.items ?? []
    const personality = items.filter((it) => it.category === 'personality')
    expect(personality.length, '至少一个 category=personality 的量表').toBeGreaterThanOrEqual(1)
    expect(personality[0].code, '人格量表 code 应为 BIG5').toBe('BIG5')
  })

  // ==================== #2 种子数据结构 ====================
  test('#2 人格量表详情含 30 题且每题 5 选项', async ({ page }) => {
    const token = await loginViaAPI(page)
    const id = await findPersonalitySurveyId(page, token)
    const resp = await page.request.get(`${API_BASE}/api/v1/surveys/${id}`, {
      headers: authHeaders(token),
    })
    expect(resp.ok()).toBe(true)
    const body = await resp.json()
    const questions: any[] = body?.data?.questions ?? []
    expect(questions.length, 'BIG5 应为 30 题').toBe(30)
    for (const q of questions) {
      expect(q.id).toMatch(/^q\d+$/)
      expect(q.type).toBe('radio')
      expect(q.options.length, 'Likert 5 点计分').toBe(5)
    }
  })

  // ==================== #4 提交成功 + 维度分数 ====================
  test('#4 人格量表提交成功并返回五维度分数', async ({ page }) => {
    const token = await loginViaAPI(page)
    const id = await findPersonalitySurveyId(page, token)

    // 全选"中立"（第 3 个选项 = 3 分）→ 每维度应得 18 分
    const answers: Record<string, number> = {}
    for (let i = 1; i <= 30; i++) answers[`q${i}`] = 3

    const resp = await page.request.post(`${API_BASE}/api/v1/surveys/${id}/submit`, {
      headers: authHeaders(token),
      data: { answers },
    })
    expect(resp.ok()).toBe(true)
    const body = await resp.json()
    const data = body?.data ?? body
    expect(data.riskLevel, '人格量表用 dimension_profile 标记').toBe('dimension_profile')
    expect(data.factorScores, '响应必须带 factorScores').toBeTruthy()
    expect(Object.keys(data.factorScores).sort()).toEqual([
      'agreeableness',
      'conscientiousness',
      'extraversion',
      'neuroticism',
      'openness',
    ])
    // 全中立 → 每维度 6 题 × 3 分 = 18
    for (const key of Object.keys(data.factorScores)) {
      expect(data.factorScores[key], `${key} 全中立应为 18`).toBe(18)
    }
  })

  // ==================== #6 tab 分组 ====================
  test('#6 /question 页有两个分类 tab', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/question')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    const tabs = page.locator('.tab-btn')
    await expect(tabs.first()).toBeVisible({ timeout: 10000 })
    expect(await tabs.count()).toBe(2)
    await expect(page.locator('.tab-btn', { hasText: '症状筛查' })).toBeVisible()
    await expect(page.locator('.tab-btn', { hasText: '人格画像' })).toBeVisible()

    // 人格画像 tab 展开 + 截图取证
    await page.locator('.tab-btn', { hasText: '人格画像' }).click()
    await page.waitForTimeout(400)
    await page.screenshot({
      path: shot('01-question-tabs-personality-1280x720'),
      clip: { x: 0, y: 0, width: 1280, height: 720 },
    })
  })

  // ==================== #3 卡片内容完整（描述非空白）====================
  // E2E-14 实测发现：列表 API 的 description 恒为空 —— proto `SurveyItem`
  // (agent.proto:67-75) 无该字段，列表走 gRPC 时被静默丢弃 → 卡片描述行空白。
  // E2E-13 的 #4 曾断言「标题、描述、题数」通过，但未实际校验描述内容（弱断言）。
  // 修复后此用例钉住"描述必须渲染出文本"。
  test('#3 量表卡片渲染描述文本（非空白行）', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/question')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    await page.locator('.tab-btn', { hasText: '人格画像' }).click()
    await page.waitForTimeout(400)

    const card = page.locator('.assessment-card').first()
    await expect(card).toBeVisible({ timeout: 10000 })
    const desc = card.locator('.card-desc')
    await expect(desc).toBeVisible()
    const text = (await desc.textContent())?.trim() ?? ''
    expect(text.length, '卡片描述不得为空（gRPC 路径曾丢弃 description）').toBeGreaterThan(4)
  })

  // ==================== #7 tab 筛选 ====================
  test('#7 切换 tab 后列表按类别筛选', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/question')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 症状筛查 tab：不应出现人格量表
    await page.locator('.tab-btn', { hasText: '症状筛查' }).click()
    await page.waitForTimeout(400)
    const symptomCards = page.locator('.assessment-card')
    const symptomCount = await symptomCards.count()
    expect(symptomCount).toBeGreaterThanOrEqual(1)
    for (let i = 0; i < symptomCount; i++) {
      await expect(symptomCards.nth(i).locator('.badge')).not.toHaveText('人格')
    }

    // 人格画像 tab：只应出现人格量表
    await page.locator('.tab-btn', { hasText: '人格画像' }).click()
    await page.waitForTimeout(400)
    const personalityCards = page.locator('.assessment-card')
    expect(await personalityCards.count()).toBeGreaterThanOrEqual(1)
    for (let i = 0; i < (await personalityCards.count()); i++) {
      await expect(personalityCards.nth(i).locator('.badge')).toHaveText('人格')
    }
  })

  // ==================== #5 结果弹窗展示维度分数 ====================
  test('#5 人格量表结果弹窗展示雷达图 + 五维度明细，不显示等级', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/question')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 切到人格画像 tab 并进入答题
    await page.locator('.tab-btn', { hasText: '人格画像' }).click()
    await page.waitForTimeout(400)
    await page
      .locator('.assessment-card')
      .first()
      .locator('button', { hasText: '开始答题' })
      .click()
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    const questions = page.locator('.question-block')
    expect(await questions.count(), '人格量表应为 30 题').toBe(30)

    await answerAllAndSubmit(page)

    const resultDialog = page.locator('.modal-card', { hasText: '你的人格画像' })
    await expect(resultDialog).toBeVisible({ timeout: 10000 })

    // 五维度明细齐全
    const dimensionRows = resultDialog.locator('.dimension-row')
    await expect(dimensionRows).toHaveCount(5)
    for (const label of ['开放性', '尽责性', '外向性', '宜人性', '神经质']) {
      await expect(resultDialog).toContainText(label)
    }
    // 每行展示分数 + 高/中/低档位
    await expect(resultDialog.locator('.dimension-level').first()).toHaveText(/高|中|低/)

    // 人格量表不得出现症状量表的"等级"表述
    await expect(resultDialog).not.toContainText('总分')
    await expect(resultDialog).not.toContainText('等级')

    // 雷达图必须真正占满可用宽度。
    // 实测回归点：`.result-content` 是 grid，而 chart-container 带 margin:0 auto ⇒
    // grid 子项失去 stretch，容器塌缩到 100px，轴标签被裁成单字（「性」「神」）。
    const chartBox = await resultDialog.locator('.chart-container').boundingBox()
    expect(chartBox, '结果弹窗必须渲染图表容器').toBeTruthy()
    expect(
      Math.round(chartBox!.width),
      '图表容器宽度不得塌缩（grid 父容器 + margin:auto 曾使其只剩 100px）',
    ).toBeGreaterThan(300)

    await page.screenshot({
      path: shot('02-personality-result-radar-1280x720'),
      clip: { x: 0, y: 0, width: 1280, height: 720 },
    })
  })

  // ==================== #11 结果持久化 ====================
  test('#11 人格结果持久化（列表可读回 factorScores）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const id = await findPersonalitySurveyId(page, token)

    const answers: Record<string, number> = {}
    for (let i = 1; i <= 30; i++) answers[`q${i}`] = 5

    const submitResp = await page.request.post(`${API_BASE}/api/v1/surveys/${id}/submit`, {
      headers: authHeaders(token),
      data: { answers },
    })
    expect(submitResp.ok()).toBe(true)
    const submitted = (await submitResp.json())?.data
    const dims = submitted.factorScores as Record<string, number>
    // 全 5 分（反向题需反转）→ 逐维度值与提交响应一致即为落库正确
    expect(Object.keys(dims).length).toBe(5)

    const listResp = await page.request.get(`${API_BASE}/api/v1/surveys/results`, {
      headers: authHeaders(token),
    })
    expect(listResp.ok()).toBe(true)
    const items: any[] = (await listResp.json())?.data?.items ?? []
    const mine = items.find(
      (it) => it.riskLevel === 'dimension_profile' && it.factorScores,
    )
    expect(mine, '列表必须包含带 factorScores 的人格结果').toBeTruthy()
    expect(mine.factorScores).toEqual(dims)

    // 详情接口同样带 factorScores
    const detailResp = await page.request.get(
      `${API_BASE}/api/v1/surveys/results/${mine.resultId}`,
      { headers: authHeaders(token) },
    )
    expect(detailResp.ok()).toBe(true)
    const detail = (await detailResp.json())?.data
    expect(detail.factorScores).toEqual(dims)
  })

  // ==================== #8 user 页人格雷达图 ====================
  test('#8 我的空间展示人格维度雷达图', async ({ page }) => {
    await loginViaAPI(page)
    // 先确保有人格结果（幂等：全中立提交一次）
    const token = await loginViaAPI(page)
    const id = await findPersonalitySurveyId(page, token)
    const answers: Record<string, number> = {}
    for (let i = 1; i <= 30; i++) answers[`q${i}`] = 3
    await page.request.post(`${API_BASE}/api/v1/surveys/${id}/submit`, {
      headers: authHeaders(token),
      data: { answers },
    })

    await page.goto('/chat/user')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(3000)

    const personalityCard = page.locator('.personality-card')
    await expect(personalityCard).toBeVisible({ timeout: 10000 })
    await expect(personalityCard).toContainText('人格维度')

    // 五维度明细
    const rows = personalityCard.locator('.dimension-row')
    await expect(rows).toHaveCount(5)
    await expect(personalityCard).toContainText('开放性')
    await expect(personalityCard).toContainText('神经质')
    await expect(personalityCard.locator('.dimension-level').first()).toHaveText(/高|中|低/)

    // 雷达图必须占满卡片宽度（同弹窗：grid 父容器 + margin:auto 曾使其塌缩）
    const chartBox = await personalityCard.locator('.chart-container').boundingBox()
    expect(chartBox, '我的空间必须渲染人格雷达图').toBeTruthy()
    expect(Math.round(chartBox!.width), '雷达图宽度不得塌缩').toBeGreaterThan(300)

    // 该区块在首屏之下 —— 滚动到它再取证，否则截图只拍到上方的行为图表
    await personalityCard.scrollIntoViewIfNeeded()
    await page.waitForTimeout(600)
    await page.screenshot({
      path: shot('03-my-space-personality-radar-1280x720'),
      clip: { x: 0, y: 0, width: 1280, height: 720 },
    })
  })

  // ==================== #12 无画像降级：AI 仍正常回复 ====================
  test('#12 AI 对话正常（人格画像查询不阻断聊天链路）', async ({ page }) => {
    const token = await loginViaAPI(page)

    const resp = await page.request.post(`${API_BASE}/api/v1/ai/stream`, {
      headers: authHeaders(token),
      data: { message: '我今天有点累', conversationId: '1' },
      timeout: 30000,
    })
    expect(resp.ok(), 'ai/stream 必须 200（画像查询失败不得阻断）').toBe(true)
    const text = await resp.text()
    expect(text).toContain('[DONE]')
    expect(text.length, 'SSE 必须有实际内容').toBeGreaterThan(20)
  })
})
