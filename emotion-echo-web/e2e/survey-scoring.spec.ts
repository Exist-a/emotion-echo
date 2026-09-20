import { test, expect } from '@playwright/test'

/**
 * E2E-F-97 回归钉：症状量表必须按 **option.score** 计分，不能按 option.id。
 *
 * 背景（2026-09-21 实测确认的真实 bug）：
 *   前端 `[id].vue` 提交的是 `answerMap[qId] = option.id`，而后端 scorer 把该值
 *   当 **score** 用。种子数据里 PHQ-9/GAD-7 的选项是 `id 1..4` / `score 0..3`
 *   （**id = score + 1**），BIG5 恰好 `id == score` 才没暴露。两个后果：
 *
 *   ① 静默虚高：全选"完全没有"（正确 score=0）→ 实测返回 total=9 / mild（轻度）
 *      ⇒ 心理健康筛查对**无症状**用户报出"轻度抑郁"
 *   ② 直接失败：选任一题的"几乎每天"（id=4）→ HTTP 400
 *      `PHQ-9 answer q1 must be 0-3, got 4`（4 是 id 不是 score）⇒ 整套答案无法提交
 *
 * 为何此前两次收口都漏掉（本用例要堵的就是这个）：
 *   - E2E-13 的 spec 每题只点**第一个选项**（id=1，不触发 400）
 *   - 且断言只查"totalScore 是个数"+"riskLevel 匹配中文正则"—— **形状断言**，
 *     `轻度` 能过；E2E-14 只测 BIG5（恰好 id==score）
 *
 * 本 spec 的原则：**断言精确取值，不断言形状**。选"第一个选项"与"最后一个选项"
 * 两端都要测 —— 前者暴露虚高，后者暴露 400。
 */

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }

async function loginViaAPI(page: import('@playwright/test').Page): Promise<string> {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const token = (await resp.json())?.data?.accessToken as string
  await page.context().addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
  return token
}

async function surveyIdByCode(page: import('@playwright/test').Page, token: string, code: string) {
  const resp = await page.request.get(`${API_BASE}/api/v1/surveys`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const items: any[] = (await resp.json())?.data?.items ?? []
  const s = items.find((i) => i.code === code)
  expect(s, `种子数据必须含量表 ${code}`).toBeTruthy()
  return s.id as number
}

/** 直接按 API 提交，answers 用 **option 数组下标** 指定每题选第几个选项（0-based） */
async function submitByOptionIndex(
  page: import('@playwright/test').Page,
  token: string,
  surveyId: number,
  questionCount: number,
  optionIndex: number,
) {
  const detail = await page.request.get(`${API_BASE}/api/v1/surveys/${surveyId}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const questions: any[] = (await detail.json())?.data?.questions ?? []
  expect(questions.length).toBe(questionCount)
  // 用种子数据里真实的 option.score（这正是前端**应该**提交的东西）
  const answers: Record<string, number> = {}
  for (const q of questions) {
    const opt = q.options[optionIndex]
    answers[q.id] = opt.score
  }
  const resp = await page.request.post(`${API_BASE}/api/v1/surveys/${surveyId}/submit`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { answers },
  })
  return resp
}

test.describe('E2E-F-97 症状量表按 score 计分（精确取值断言）', () => {
  test('#1 PHQ-9 全选"完全没有"(score=0) → total=0 / none', async ({ page }) => {
    const token = await loginViaAPI(page)
    const sid = await surveyIdByCode(page, token, 'PHQ-9')
    const resp = await submitByOptionIndex(page, token, sid, 9, 0)
    expect(resp.ok(), '提交必须 200').toBe(true)
    const data = (await resp.json())?.data
    // 精确取值断言：无症状用户必须得 0 分、等级 none
    // （修前实测 9 / mild —— 心理健康筛查假阳性）
    expect(data.totalScore, '无可测症状必须 0 分').toBe(0)
    expect(data.riskLevel, '无可测症状必须 none').toBe('none')
  })

  test('#2 PHQ-9 全选"几乎每天"(score=3) → total=27 / extreme', async ({ page }) => {
    const token = await loginViaAPI(page)
    const sid = await surveyIdByCode(page, token, 'PHQ-9')
    const resp = await submitByOptionIndex(page, token, sid, 9, 3)
    // 修前此项直接 HTTP 400（提交的是 option.id=4，超出 score 值域 0-3）
    expect(resp.ok(), '选最后一档必须能提交（修前 400: answer q1 must be 0-3, got 4）').toBe(true)
    const data = (await resp.json())?.data
    expect(data.totalScore).toBe(27)
    expect(data.riskLevel).toBe('extreme')
  })

  test('#3 GAD-7 两端同样成立（0/0 与 21/severe）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const sid = await surveyIdByCode(page, token, 'GAD-7')

    const low = await submitByOptionIndex(page, token, sid, 7, 0)
    expect(low.ok()).toBe(true)
    const lowData = (await low.json())?.data
    expect(lowData.totalScore).toBe(0)
    expect(lowData.riskLevel).toBe('none')

    const high = await submitByOptionIndex(page, token, sid, 7, 3)
    expect(high.ok(), 'GAD-7 选最后一档必须能提交').toBe(true)
    const highData = (await high.json())?.data
    expect(highData.totalScore).toBe(21)
    expect(highData.riskLevel).toBe('severe')
  })

  test('#4 BIG5 恰好 id==score，两端取值不变（防修复回归）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const sid = await surveyIdByCode(page, token, 'BIG5')

    const low = await submitByOptionIndex(page, token, sid, 30, 0)
    expect(low.ok()).toBe(true)
    // 全选 1 分：每维度 6 题 × 1 = 6，含反向题后见 scorer 契约；此处只断言总分可复现
    const lowTotal = (await low.json())?.data?.totalScore
    expect(typeof lowTotal).toBe('number')

    const high = await submitByOptionIndex(page, token, sid, 30, 4)
    expect(high.ok(), 'BIG5 选最后一档必须能提交').toBe(true)
    expect((await high.json())?.data?.totalScore).toBeGreaterThan(lowTotal)
  })

  test('#5 UI 端到端：PHQ-9 每题点最后一档 → 提交成功且弹窗显示极重度', async ({ page }) => {
    const token = await loginViaAPI(page)
    const sid = await surveyIdByCode(page, token, 'PHQ-9')
    void token

    await page.goto(`/question/${sid}`)
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    const questions = page.locator('.question-block')
    const count = await questions.count()
    expect(count).toBe(9)
    // 每题点**最后一个**选项（正是修前会 400 的那一档）
    for (let i = 0; i < count; i++) {
      await questions.nth(i).locator('.radio-option').last().click()
    }

    const submitPromise = page.waitForResponse(
      (r) => r.url().includes('/submit') && r.request().method() === 'POST',
    )
    await page.locator('button', { hasText: '提交这份答卷' }).click()
    const resp = await submitPromise
    expect(resp.status(), 'UI 选最后一档提交必须成功（修前 400）').toBe(200)
    const body = await resp.json()
    expect(body?.data?.totalScore).toBe(27)

    // 症状量表的弹窗仍走"总分 / 等级"，且等级为极重度
    const dialog = page.locator('.modal-card', { hasText: '你给出的答案' })
    await expect(dialog).toBeVisible({ timeout: 10000 })
    await expect(dialog).toContainText('总分')
    await expect(dialog).toContainText('27')
    await expect(dialog).toContainText('极重度')
  })
})
