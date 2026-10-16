// pages/chat/conversation/cl07-emotion-chip.architecture.test.ts
//
// CL-07 前端半 · 回归钉（源码级契约）。
//
// 背景（2026-10-16 浏览器实测抓到的真 bug）：
//   `[id].vue` 的 `<article>` 带 `v-memo="[item.id, item.content, item.status, item.contentType]"`。
//   `v-memo` 按**固定依赖列表**缓存子树 —— 融合情绪角标的数据（`byMessage`）是**异步**到的，
//   不在依赖列表里 ⇒ **角标永不重渲染**（页面看起来"没实现"）。
//   单元测试测不到（没有挂载 `[id].vue` 的组件测试），只有真实浏览器能暴露。
//
// 本钉锁死三件事，防止回归：
//   ① 对话页确实挂了 `CurrentEmotionBar`；
//   ② 用户消息确实渲染 `emotion-chip`；
//   ③ `v-memo` 依赖列表**包含** `byMessage[...]`（否则角标不更新）。

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const PAGE = resolve(HERE, '[id].vue')
const src = readFileSync(PAGE, 'utf-8')

describe('CL-07 情绪回显：对话页接线契约', () => {
  it('顶部状态条已挂载（CurrentEmotionBar + state 绑定）', () => {
    expect(src).toContain('<CurrentEmotionBar')
    expect(src).toContain(':state="currentEmotionState"')
  })

  it('用户消息渲染融合情绪角标', () => {
    expect(src).toContain('class="emotion-chip"')
    expect(src).toMatch(/v-if="item\.sender === 'user' && byMessage\[item\.id\]"/)
  })

  it('v-memo 依赖列表包含 byMessage[item.id]（否则异步数据到达后角标不重渲染 —— 实测踩过）', () => {
    // 注意：依赖列表里含嵌套的 `[`/`]`（byMessage[item.id]）⇒ 不能用 [^\]]* 截取，
    // 用 [^"]* 截到属性引号为止。
    const m = src.match(/v-memo="(\[[^"]*\])"/)
    expect(m, '未找到 v-memo 依赖列表').not.toBeNull()
    expect(m![1]).toContain('byMessage[item.id]')
  })

  it('数字人情绪已接真（onFinish 走 syncEmotionToDigitalHuman）', () => {
    expect(src).toContain('syncEmotionToDigitalHuman')
  })
})
