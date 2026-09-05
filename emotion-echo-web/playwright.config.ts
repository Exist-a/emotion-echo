import { defineConfig, devices } from '@playwright/test'

/**
 * Playwright 配置
 *
 * 默认 webServer：在 spec 跑前自动起 `pnpm dev`（Nuxt 3/4 dev server :3000）
 * 生产路径可用 `BASE_URL=http://localhost:3000` 跑（已在跑就 skip）
 *
 * 测试目录：./e2e
 * reporter：HTML（生成 playwright-report/）+ list（终端）
 *
 * 跑：
 *   pnpm playwright test                 # 全部
 *   pnpm playwright test --headed        # 开着浏览器看
 *   pnpm playwright test --grep login    # 按 name 过滤
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // Nuxt dev server 单进程，测试串行更稳
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  use: {
    baseURL: process.env.BASE_URL ?? 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    locale: 'zh-CN',
  },
  projects: [
    {
      // chromium-headless-shell is the lighter variant Playwright installs
      // by default on Windows; use it because it has no GUI deps.
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], channel: 'chromium-headless-shell' },
    },
  ],
  webServer: process.env.BASE_URL
    ? undefined
    : {
        command: 'pnpm dev --port 3000',
        url: 'http://localhost:3000',
        reuseExistingServer: true,
        timeout: 120_000,
        stdout: 'pipe',
        stderr: 'pipe',
      },
})
