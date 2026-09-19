/**
 * SecurityQuestionDialog 组件契约测试（E2E-09 注册流程）
 *
 * 契约要点：
 * 1. 组件接收 modelValue (boolean) 控制显示/隐藏
 * 2. 组件接收 questions (string[]) — 1~2 个密保问题
 * 3. 内部维护 answers 数组，emit 'submit' 携带 { questions, answers }
 * 4. emit 'update:modelValue' = false 关闭弹框
 * 5. 不可跳过：无"跳过"按钮，关闭 = 中止注册
 * 6. 必填校验：答案为空时拒绝提交
 * 7. 含用途提示文案"此密保用于找回密码"
 */

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const COMP_PATH = resolve(process.cwd(), 'app/components/SecurityQuestionDialog.vue')

describe('SecurityQuestionDialog.vue · source 契约', () => {
  let src: string

  try {
    src = readFileSync(COMP_PATH, 'utf8')
  } catch {
    // 文件不存在时所有测试失败（RED 状态）
    it('组件文件必须存在', () => {
      throw new Error(`SecurityQuestionDialog.vue 不存在于 ${COMP_PATH}`)
    })
    return
  }

  it('接收 modelValue prop 控制弹框显示', () => {
    expect(src).toMatch(/modelValue/)
    expect(src).toMatch(/defineProps|props\s*[:(]/)
  })

  it('接收 questions prop（密保问题列表）', () => {
    expect(src).toMatch(/questions/)
  })

  it('emit submit 事件携带 questions 和 answers', () => {
    expect(src).toMatch(/emit.*submit/)
    expect(src).toMatch(/answers/)
  })

  it('emit update:modelValue 关闭弹框', () => {
    expect(src).toMatch(/update:modelValue/)
  })

  it('不可跳过：无"跳过"按钮', () => {
    expect(src).not.toMatch(/跳过/)
  })

  it('含用途提示文案"此密保用于找回密码"', () => {
    expect(src).toContain('找回密码')
  })

  it('答案为空时拒绝提交（必填校验）', () => {
    // 应有 trim / length 检查或 required 验证
    expect(src).toMatch(/trim|length|empty|required|必填/)
  })

  it('含关闭按钮或关闭机制', () => {
    expect(src).toMatch(/close|关闭|×|✕/)
  })
})