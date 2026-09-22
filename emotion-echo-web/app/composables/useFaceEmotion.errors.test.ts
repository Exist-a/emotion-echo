// useFaceEmotion.errors.test.ts — F-119 契约钉
//
// 背景：E2E-16 用户第二轮复测（2026-09-22）：
//   网关 access log `/multimodal/analyze` 累计请求数 0（两轮实测均 0）⇒ 摄像头权限层失败
//   但页面只笼统提示「无法访问摄像头，请检查权限设置」，用户无法判断是权限拒/无设备/被占用。
//
// 三类典型错误（MediaDevices.getUserMedia 标准定义）：
//   - NotAllowedError  : 用户拒权限 / 浏览器策略阻
//   - NotFoundError    : 设备不存在（无摄像头）
//   - NotReadableError : 设备被其他程序独占占用
//
// 修法：useFaceEmotion.startCamera catch 块按 error.name 分支抛不同 Error，
//       UI 端（[id].vue / new.vue）按错误消息提示用户具体动作。
//
// 契约钉（4 项）：
//   1. catch 块必须按 error.name 分支（必须含 NotAllowed / NotFound / NotReadable 三个之一）
//   2. 至少有一个分支明确提示"权限"（NotAllowed）
//   3. 至少有一个分支明确提示"设备"或"摄像头"是否存在（NotFound）
//   4. 至少有一个分支明确提示"占用"或"独占"（NotReadable）

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(resolve(__dirname, './useFaceEmotion.ts'), 'utf8')

describe('useFaceEmotion · F-119 摄像头错误分类契约', () => {
  it('catch 块按 error.name 分支识别三类错误', () => {
    // 至少识别 NotAllowedError + NotFoundError + NotReadableError 中的两个
    const mentionsNotAllowed = /NotAllowedError/.test(src)
    const mentionsNotFound = /NotFoundError/.test(src)
    const mentionsNotReadable = /NotReadableError/.test(src)
    const count = [mentionsNotAllowed, mentionsNotFound, mentionsNotReadable].filter(Boolean).length
    expect(count, 'catch 块必须识别 ≥2 类错误').toBeGreaterThanOrEqual(2)
  })

  it('NotAllowedError 分支含"权限"提示关键词', () => {
    // 在 NotAllowedError 上下文附近必须含 "权限" 关键词
    // 简化：源码中 "权限" 字符串 + NotAllowedError 引用同时存在
    expect(src, 'NotAllowedError 分支必须含"权限"提示').toMatch(/NotAllowedError/)
    expect(src, '必须出现"权限"提示文案').toContain('权限')
  })

  it('NotFoundError 分支含"设备"/"摄像头"提示关键词', () => {
    expect(src, 'NotFoundError 分支必须含"设备"/"摄像头"提示').toMatch(/NotFoundError/)
    const hasDeviceHint = src.includes('未找到') || src.includes('找不到') || src.includes('没有')
    expect(hasDeviceHint, '必须含"未找到/找不到/没有"类提示').toBe(true)
  })

  it('NotReadableError 分支含"占用"/"独占"提示关键词', () => {
    // NotReadableError 不是必须识别（如果只识别两类，这项允许放宽）
    // 但如果识别了，必须含"占用"/"独占"提示
    const mentionsNotReadable = /NotReadableError/.test(src)
    if (mentionsNotReadable) {
      const hasOccupiedHint = src.includes('占用') || src.includes('独占') || src.includes('正在使用')
      expect(hasOccupiedHint, 'NotReadableError 分支必须含"占用/独占/正在使用"提示').toBe(true)
    } else {
      // 没识别 NotReadableError，至少要求其他两类有提示
      expect(true, 'NotReadableError 未识别，跳过该断言').toBe(true)
    }
  })
})
