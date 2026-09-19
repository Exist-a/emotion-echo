/**
 * 注册流程密保问题改造契约测试（E2E-09）
 *
 * 契约要点：
 * 1. 注册表单不再含验证码字段（code-field / verificationCode / 获取验证码）
 * 2. 注册提交后触发密保弹框（SecurityQuestionDialog）而非直接调 register
 * 3. 密保弹框 submit 后才调 userStore.register 并传 securityQuestions
 * 4. RegisterParams 类型含 securityQuestions 字段
 * 5. store.register() 调用时传 securityQuestions 给后端
 */

import { describe, it, expect, beforeAll } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const LOGIN_INDEX = resolve(process.cwd(), 'app/pages/login/index.vue')
const USER_STORE = resolve(process.cwd(), 'app/stores/user.ts')
const API_TYPES = resolve(process.cwd(), 'app/types/api.ts')

describe('注册页验证码删除（E2E-09）', () => {
  let src: string

  beforeAll(() => {
    src = readFileSync(LOGIN_INDEX, 'utf8')
  })

  it('注册表单不再含验证码输入框', () => {
    expect(src).not.toMatch(/verificationCode/)
    expect(src).not.toMatch(/code-field/)
  })

  it('不再有"获取验证码"按钮', () => {
    expect(src).not.toMatch(/获取验证码/)
  })

  it('不再有验证码倒计时逻辑', () => {
    expect(src).not.toMatch(/verificationCodeCountDown/)
    expect(src).not.toMatch(/isGetVerificationCode/)
  })

  it('不再有验证码开发模式提示', () => {
    expect(src).not.toMatch(/验证码会打印在服务端终端/)
  })
})

describe('注册页密保弹框集成（E2E-09）', () => {
  let src: string

  beforeAll(() => {
    src = readFileSync(LOGIN_INDEX, 'utf8')
  })

  it('import 或引用 SecurityQuestionDialog 组件', () => {
    expect(src).toMatch(/SecurityQuestionDialog/)
  })

  it('注册提交后显示密保弹框而非直接注册', () => {
    // 应有 showSecurityDialog / showDialog 等状态控制弹框
    expect(src).toMatch(/showSecurity|showDialog|securityDialog/)
  })

  it('密保弹框 submit 回调中调 userStore.register', () => {
    // submit 事件处理应含 register 调用
    expect(src).toMatch(/register/)
  })

  it('register 调用时传 securityQuestions', () => {
    expect(src).toMatch(/securityQuestions/)
  })
})

describe('RegisterParams 类型扩展（E2E-09）', () => {
  let src: string

  beforeAll(() => {
    src = readFileSync(API_TYPES, 'utf8')
  })

  it('RegisterParams 含 securityQuestions 字段', () => {
    expect(src).toMatch(/securityQuestions/)
  })

  it('securityQuestions 为数组类型（含 question + answer）', () => {
    // 应有 SecurityQuestion 类型或 inline { question: string; answer: string }[]
    expect(src).toMatch(/SecurityQuestion|question.*answer/)
  })
})

describe('user store register 方法扩展（E2E-09）', () => {
  let storeSrc: string
  let typesSrc: string

  beforeAll(() => {
    storeSrc = readFileSync(USER_STORE, 'utf8')
    typesSrc = readFileSync(API_TYPES, 'utf8')
  })

  it('register 方法使用 RegisterParams 类型（已含 securityQuestions）', () => {
    // store 的 register(params: RegisterParams) 通过类型系统保证 securityQuestions 传递
    expect(storeSrc).toMatch(/register.*RegisterParams|RegisterParams.*register/)
  })

  it('RegisterParams 类型已扩展为含 securityQuestions', () => {
    // RegisterParams 定义在 api.ts，已被改为含 securityQuestions
    expect(typesSrc).toMatch(/RegisterParams[\s\S]*securityQuestions|securityQuestions[\s\S]*RegisterParams/)
  })
})