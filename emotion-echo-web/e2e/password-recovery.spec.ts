import { test, expect } from '@playwright/test'

/**
 * E2E-07: 找回密码流程 Playwright 回归钉
 *
 * 覆盖 plan.md 13 个测试点：
 * 1. 登录页"忘记密码"入口可达
 * 2. 步骤 1：输入用户名 → 展示该用户的密保问题
 * 3. 不存在的用户名 → 防枚举响应
 * 4. 步骤 2：答案正确 → 进入改密页
 * 5. 步骤 2：答案错误 → 拒绝且不泄露答案
 * 6. 错误次数限制
 * 7. 步骤 3：改密成功
 * 8. 新密码可登录
 * 9. 旧密码失效
 * 10. 路由守卫：跳过步骤
 * 11. localStorage 步骤持久化
 * 12. 密码强度校验
 * 13. 页面文案无"手机号/邮箱"
 *
 * 前置条件：
 * - dev 环境运行中（docker compose up）
 * - 测试账号已设置密保（scripts/seed_security_question.sh）
 */

// 测试账号配置（需与 seed_security_question.sh 一致）
const TEST_USERNAME = 'test_user'
const TEST_PASSWORD = 'test123456'
const TEST_SECURITY_ANSWER = 'kitty'
const NEW_PASSWORD = 'newpwd123'

test.describe('password recovery flow (E2E-07)', () => {
  // 每个测试前清理 localStorage
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.evaluate(() => {
      localStorage.removeItem('forgetPwdStep')
      localStorage.removeItem('forgetPwdAccount')
      localStorage.removeItem('forgetPwdSecurityVerified')
      localStorage.removeItem('forgetPwdVerifiedUsername')
    })
  })

  // #1: 登录页"忘记密码"入口可达
  test('#1 忘记密码入口可达', async ({ page }) => {
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    const forgetLink = page.getByRole('link', { name: /忘记密码/ })
      .or(page.getByRole('button', { name: /忘记密码/ }))
      .or(page.locator('a[href*="forget"]'))
    await expect(forgetLink).toBeVisible({ timeout: 10_000 })
    await forgetLink.click()

    await expect(page).toHaveURL(/\/login\/forget/)
  })

  // #2: 步骤 1：输入用户名 → 展示该用户的密保问题
  test('#2 输入用户名后展示密保问题', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    // 输入用户名
    const usernameInput = page.locator('input[placeholder="用户名"]')
    await expect(usernameInput).toBeVisible({ timeout: 10_000 })
    await usernameInput.fill(TEST_USERNAME)

    // 点击获取密保问题
    const fetchBtn = page.getByRole('button', { name: /获取密保问题/ })
    await expect(fetchBtn).toBeVisible()
    await fetchBtn.click()

    // 等待密保问题展示
    await page.waitForTimeout(2000)

    // 断言密保问题出现（不泄露答案）
    await expect(page.locator('.question-text')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('.question-text')).toContainText(/宠物|城市|问题/)
  })

  // #3: 不存在的用户名 → 防枚举响应
  test('#3 不存在用户名防枚举', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    const usernameInput = page.locator('input[placeholder="用户名"]')
    await usernameInput.fill('nonexistent_user_xyz')

    const fetchBtn = page.getByRole('button', { name: /获取密保问题/ })
    await fetchBtn.click()

    await page.waitForTimeout(2000)

    // 断言：应有错误提示，但不泄露用户不存在
    const errorMsg = page.locator('.el-notification, .notify, [role="alert"]')
    await expect(errorMsg).toBeVisible({ timeout: 5000 })
    // 不应出现"用户不存在"字样（防枚举）
    await expect(errorMsg).not.toContainText(/不存在|not found/i)
  })

  // #4: 步骤 2：答案正确 → 进入改密页
  test('#4 答案正确进入改密页', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    // 输入用户名并获取问题
    const usernameInput = page.locator('input[placeholder="用户名"]')
    await usernameInput.fill(TEST_USERNAME)
    await page.getByRole('button', { name: /获取密保问题/ }).click()
    await page.waitForTimeout(2000)

    // 输入正确答案
    const answerInput = page.locator('input[placeholder="请输入答案"]')
    await expect(answerInput).toBeVisible({ timeout: 10_000 })
    await answerInput.fill(TEST_SECURITY_ANSWER)

    // 点击验证
    await page.getByRole('button', { name: /验证/ }).click()
    await page.waitForTimeout(2000)

    // 断言跳转到修改密码页
    await expect(page).toHaveURL(/\/login\/forget\/modify/)
  })

  // #5: 步骤 2：答案错误 → 拒绝且不泄露答案
  test('#5 答案错误被拒绝', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    const usernameInput = page.locator('input[placeholder="用户名"]')
    await usernameInput.fill(TEST_USERNAME)
    await page.getByRole('button', { name: /获取密保问题/ }).click()
    await page.waitForTimeout(2000)

    const answerInput = page.locator('input[placeholder="请输入答案"]')
    await answerInput.fill('wrong_answer_xyz')

    await page.getByRole('button', { name: /验证/ }).click()
    await page.waitForTimeout(2000)

    // 断言：仍在当前页（未跳转）
    await expect(page).toHaveURL(/\/login\/forget\/verify/)
    // 断言：有错误提示
    const errorMsg = page.locator('.el-notification, .notify, [role="alert"]')
    await expect(errorMsg).toBeVisible({ timeout: 5000 })
  })

  // #6: 错误次数限制（跳过——需后端限流配合，标记为 [M]）
  test.skip('#6 错误次数限制 [M] 需人工裁定', async ({ page }) => {
    // 此测试需后端限流配置配合，标记为需人工裁定
  })

  // #7: 步骤 3：改密成功
  test('#7 改密成功', async ({ page }) => {
    // 前置：完成密保验证
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    const usernameInput = page.locator('input[placeholder="用户名"]')
    await usernameInput.fill(TEST_USERNAME)
    await page.getByRole('button', { name: /获取密保问题/ }).click()
    await page.waitForTimeout(2000)

    const answerInput = page.locator('input[placeholder="请输入答案"]')
    await answerInput.fill(TEST_SECURITY_ANSWER)
    await page.getByRole('button', { name: /验证/ }).click()
    await page.waitForTimeout(2000)

    // 在修改密码页输入新密码
    const newPwdInput = page.locator('input[placeholder="至少 6 位"]')
    await expect(newPwdInput).toBeVisible({ timeout: 10_000 })
    await newPwdInput.fill(NEW_PASSWORD)

    const confirmPwdInput = page.locator('input[placeholder="再输入一次"]')
    await confirmPwdInput.fill(NEW_PASSWORD)

    await page.getByRole('button', { name: /保存新密码/ }).click()
    await page.waitForTimeout(2000)

    // 断言跳转到成功页
    await expect(page).toHaveURL(/\/login\/forget\/success/)
    await expect(page.getByText('密码已更新')).toBeVisible()
  })

  // #8: 新密码可登录
  test('#8 新密码可登录', async ({ page }) => {
    // 此测试依赖 #7 完成后的新密码
    // 为独立性，这里直接用新密码尝试登录
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    const usernameInput = page.locator('input[placeholder="用户名"]').first()
    await usernameInput.fill(TEST_USERNAME)

    const pwdInput = page.locator('input[type="password"]').first()
    await pwdInput.fill(NEW_PASSWORD)

    const loginBtn = page.getByRole('button', { name: /^登录$/ })
    await loginBtn.click()
    await page.waitForTimeout(3000)

    // 断言：登录成功（跳转到 /chat）
    await expect(page).toHaveURL(/\/chat/, { timeout: 15_000 })
  })

  // #9: 旧密码失效
  test('#9 旧密码失效', async ({ page }) => {
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    const usernameInput = page.locator('input[placeholder="用户名"]').first()
    await usernameInput.fill(TEST_USERNAME)

    const pwdInput = page.locator('input[type="password"]').first()
    await pwdInput.fill(TEST_PASSWORD) // 旧密码

    const loginBtn = page.getByRole('button', { name: /^登录$/ })
    await loginBtn.click()
    await page.waitForTimeout(3000)

    // 断言：登录失败（未跳转）
    await expect(page).toHaveURL(/\/login/)
  })

  // #10: 路由守卫：跳过步骤
  test('#10 路由守卫阻止跳过步骤', async ({ page }) => {
    // 直接访问修改密码页（未完成密保验证）
    await page.goto('/login/forget/modify')
    await page.waitForTimeout(2000)

    // 断言：被重定向到 verify 页
    await expect(page).toHaveURL(/\/login\/forget\/verify/)
  })

  // #11: localStorage 步骤持久化
  test('#11 localStorage 步骤持久化', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    // 设置 localStorage 模拟已验证状态
    await page.evaluate(() => {
      localStorage.setItem('forgetPwdStep', '1')
      localStorage.setItem('forgetPwdAccount', TEST_USERNAME)
      localStorage.setItem('forgetPwdSecurityVerified', 'true')
      localStorage.setItem('forgetPwdVerifiedUsername', TEST_USERNAME)
    })

    // 刷新页面
    await page.reload()
    await page.waitForTimeout(2000)

    // 断言：可以访问 modify 页（不被重定向）
    await page.goto('/login/forget/modify')
    await page.waitForTimeout(1000)
    await expect(page).toHaveURL(/\/login\/forget\/modify/)
  })

  // #12: 密码强度校验
  test('#12 密码强度校验', async ({ page }) => {
    // 前置：设置已验证状态
    await page.goto('/login/forget/verify')
    await page.evaluate(() => {
      localStorage.setItem('forgetPwdStep', '1')
      localStorage.setItem('forgetPwdSecurityVerified', 'true')
      localStorage.setItem('forgetPwdVerifiedUsername', TEST_USERNAME)
    })

    await page.goto('/login/forget/modify')
    await page.waitForLoadState('networkidle')

    // 输入弱密码
    const newPwdInput = page.locator('input[placeholder="至少 6 位"]')
    await expect(newPwdInput).toBeVisible({ timeout: 10_000 })
    await newPwdInput.fill('123') // 太短

    const confirmPwdInput = page.locator('input[placeholder="再输入一次"]')
    await confirmPwdInput.fill('123')

    await page.getByRole('button', { name: /保存新密码/ }).click()
    await page.waitForTimeout(1000)

    // 断言：仍在当前页（校验失败）
    await expect(page).toHaveURL(/\/login\/forget\/modify/)
  })

  // #13: 页面文案无"手机号/邮箱"
  test('#13 页面文案无手机号邮箱', async ({ page }) => {
    await page.goto('/login/forget/verify')
    await page.waitForLoadState('networkidle')

    const pageContent = await page.textContent('body')

    // 断言：不包含手机号/邮箱相关文案
    expect(pageContent).not.toContain('手机号')
    expect(pageContent).not.toContain('邮箱')
    expect(pageContent).not.toContain('验证码')
    expect(pageContent).not.toContain('phone')
    expect(pageContent).not.toContain('email')

    // 断言：包含密保相关文案
    expect(pageContent).toContain('密保')
  })
})