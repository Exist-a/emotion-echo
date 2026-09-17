import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const VERIFY_SRC = readFileSync(resolve(__dirname, 'verify.vue'), 'utf8')

// Stage 112 修复（方案 A）：forget-pwd/verify 页面之前的文案承诺"输入手机号或邮箱"，但项目
// 登录、注册、重置全用 username（user 表 phone 列从未用作登录凭据）。
// BFF /api/v1/auth/reset-password 与 user-svc ResetPassword 都按 username 查找。
// 本测试钉死契约：UI 文案、placeholder、错误信息、校验正则必须按 username 设计，
// 不能误导用户以为支持手机号/邮箱。

// 把注释和 <style> 块剥掉，只看 <template> 和 <script> 的运行时行为约束
const TEMPLATE_AND_SCRIPT = (() => {
  const tmpl = VERIFY_SRC.match(/<template>([\s\S]*?)<\/template>/)?.[1] ?? ''
  const script = VERIFY_SRC.match(/<script[^>]*>([\s\S]*?)<\/script>/)?.[1] ?? ''
  // 进一步把注释去掉
  const stripped = script.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  return tmpl + '\n' + stripped
})()

describe('forget-pwd/verify username-only copy contract (Stage 112 round 3)', () => {
  it('页面文案不允许出现"手机号"、"邮箱"、"phone / email" 字样', () => {
    expect(TEMPLATE_AND_SCRIPT).not.toMatch(/手机号/)
    expect(TEMPLATE_AND_SCRIPT).not.toMatch(/邮箱/)
    expect(TEMPLATE_AND_SCRIPT).not.toMatch(/phone\s*or\s*email/i)
  })

  it('placeholder 与错误信息应使用"用户名"措辞', () => {
    expect(TEMPLATE_AND_SCRIPT).toMatch(/placeholder=[^>]*用户名/)
    expect(TEMPLATE_AND_SCRIPT).toMatch(/请输入用户名/)
  })

  it('必须引用 username-only 校验逻辑（不再 import phoneOrEmailReg）', () => {
    expect(VERIFY_SRC).not.toContain('phoneOrEmailReg')
    expect(VERIFY_SRC).toMatch(/pattern:\s*\/[^/]+\//)
  })
})
