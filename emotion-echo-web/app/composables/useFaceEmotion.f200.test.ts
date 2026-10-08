// useFaceEmotion.f200.test.ts — E2E-29 #19（F-200）契约钉
//
// 背景（账本 E2E-F-200，2026-10-07 用户实测）：点「开启摄像头」报
// 「摄像头开启失败：摄像头组件未就绪，请刷新页面或稍后重试」。落点是 `useFaceEmotion.ts`
// 的 **TypeError 宽兜底**分支——它把三种完全不同的成因混成一句文案：
//   ① 环境根本没有 `navigator.mediaDevices`（部分 webview/IAB/隐私模式）⇒ **刷新无用**，
//      文案却让用户去刷新；
//   ② 预览组件（videoRef）尚未挂载 ⇒ 稍候即可；
//   ③ `getUserMedia` 返回 null 流 ⇒ 设备/驱动问题。
// 且 `error.name` 只进了 console，**没有留痕到任何可查的地方**（用户侧无归因依据）。
//
// 契约钉（4 项，静态源码扫描 —— 与 F-119 测试同风格，因为真实 getUserMedia 在 happy-dom
// 下不可用，行为测试会失真）：
//   1. 前置条件分支必须**区分**「环境不支持 mediaDevices」与「预览未挂载」两类文案
//   2. 「环境不支持」分支不得以"刷新页面"为唯一指引（必须提到浏览器/环境）
//   3. 抛出的错误必须携带 `error.name`（真因留痕）
//   4. catch 块必须调用错误上报（`reportClientError`）——真因留痕到服务端可查处

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(resolve(__dirname, './useFaceEmotion.ts'), 'utf8')

describe('useFaceEmotion · F-200 错误透出与真因留痕契约', () => {
  it('契约 1：区分「环境不支持 mediaDevices」与「预览未挂载」两类前置条件', () => {
    expect(src, '必须显式探测 mediaDevices 能力').toMatch(/mediaDevices/)
    expect(src, '「环境不支持」文案必须提到浏览器/环境').toMatch(/浏览器环境|不支持摄像头采集|缺少 mediaDevices/)
    expect(src, '「预览未挂载」文案必须保留组件未就绪语义（F-119 契约不回退）').toMatch(/组件未就绪/)
  })

  it('契约 2：环境不支持分支不得只让用户刷新页面', () => {
    // 取「环境不支持」那句文案，断言它不是"请刷新页面"收尾
    const m = src.match(/当前浏览器环境不支持摄像头采集[^'`"]*/)
    expect(m, '未找到「环境不支持」文案').toBeTruthy()
    const text = m![0]
    expect(text, '环境不支持时刷新无用，必须给可操作指引（换浏览器/退出内嵌浏览器）').toMatch(/Chrome|Edge|内嵌|webview|隐私/)
  })

  it('契约 3：抛出的错误携带 error.name（真因留痕）', () => {
    expect(src, '错误消息必须拼接 error.name（如 ［原因：TypeError］）').toMatch(/原因：\$\{|原因：.*name/)
  })

  it('契约 4：catch 块调用客户端错误上报（真因留痕到服务端）', () => {
    expect(src, '必须 import 并调用 reportClientError').toMatch(/reportClientError/)
    expect(src, '上报必须发生在 startCamera 的 catch 路径').toMatch(/reportClientError\(\s*'error'/)
  })
})
