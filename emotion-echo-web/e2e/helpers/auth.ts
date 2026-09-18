/**
 * E2E 测试认证辅助函数（E2E-06）
 *
 * 提供共享的登录逻辑，支持环境变量注入凭据。
 * 解决演示账号硬编码问题，便于未来更换凭据。
 */
import { Page, expect } from '@playwright/test'

/** 从环境变量读取演示账号凭据，带默认值 */
export function getDemoCredentials() {
  return {
    username: process.env.E2E_USERNAME ?? 'echo',
    password: process.env.E2E_PASSWORD ?? 'echo123',
  }
}

/** 从环境变量读取网关 URL */
export function getGatewayUrl(): string {
  return process.env.E2E_GATEWAY_URL ?? 'http://localhost:19080'
}

/**
 * 通过 API 登录并返回 accessToken
 *
 * 用于需要直接调用 API 的测试（如 chat-flow happy-path-1）
 */
export async function loginViaAPI(page: Page): Promise<string> {
  const { username, password } = getDemoCredentials()
  const gatewayUrl = getGatewayUrl()

  const loginResp = await page.request.post(`${gatewayUrl}/api/v1/auth/login`, {
    data: { username, password },
  })
  expect(loginResp.ok()).toBeTruthy()
  const loginData = await loginResp.json()
  return loginData.accessToken
}

/**
 * 通过 UI 快速登录按钮登录
 *
 * 用于需要测试完整 UI 流程的测试（如 login-flow、dashboard-flow）
 * 点击"用演示账号快速体验"按钮完成登录
 */
export async function loginViaQuickButton(page: Page) {
  // 等待登录页加载
  await page.waitForLoadState('domcontentloaded')

  // 点击"用演示账号快速体验"按钮
  const quickLoginBtn = page.getByRole('button', { name: '用演示账号快速体验' })
  await expect(quickLoginBtn).toBeVisible({ timeout: 10000 })
  await quickLoginBtn.click()

  // 等待导航完成（登录后会跳转到 /chat/conversation/new）
  await page.waitForURL(/\/chat\/conversation/, { timeout: 15000 })
  await page.waitForLoadState('domcontentloaded')
}

/**
 * 设置认证 cookie（用于需要预设登录状态的测试）
 */
export async function setAuthCookie(page: Page, token: string) {
  await page.context().addCookies([
    {
      name: 'access_token',
      value: token,
      domain: 'localhost',
      path: '/',
      httpOnly: true,
      sameSite: 'Lax',
    },
  ])
}