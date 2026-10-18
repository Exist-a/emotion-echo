// pages/chat/conversation/cl07-emotion-display.architecture.test.ts
//
// CL-07 前端半 · 对话页接线契约（源码级回归钉）。
//
// 锁死当前**已确认**的形态（2026-10-16 浏览器实测 + 用户裁定）：
//   ① 顶部「此刻的状态」状态条已挂；
//   ② 数字人情绪已接真（onFinish → syncEmotionToDigitalHuman）；
//   ③ **G2 消息级融合角标已按用户裁定移除** —— ① 页面上不得再出现 `.emotion-chip`；
//      ② 不得再引用随之删除的 `byMessage`（否则引用已移除符号 ⇒ 编译期就炸）。
//
// 为什么用源码级钉：`[id].vue` 没有组件挂载测试（页面依赖 store/router/Three.js），
// 而这两条恰是"改了别处把这里破坏掉"最容易漏的地方（v-memo 那次就吃过亏）。

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(resolve(HERE, '[id].vue'), 'utf-8')

describe('CL-07 情绪回显：对话页接线契约', () => {
  it('顶部状态条已挂载（CurrentEmotionBar + state 绑定）', () => {
    expect(src).toContain('<CurrentEmotionBar')
    expect(src).toContain(':state="currentEmotionState"')
  })

  it('数字人情绪已接真（onFinish 走 syncEmotionToDigitalHuman）', () => {
    expect(src).toContain('syncEmotionToDigitalHuman')
  })

  it('三处 onFinish（语音链 / 附件链 / 文本链）都接了数字人情绪（只接一处不算）', () => {
    const calls = src.match(/void syncEmotionToDigitalHuman\(aiEmotion\)/g) ?? []
    expect(calls.length).toBe(3)
  })

  it('G2 消息级角标已移除（用户裁定 2026-10-16）：不得再出现 emotion-chip', () => {
    expect(src).not.toContain('emotion-chip')
  })

  it('不引用已删除的 byMessage（消息级缓存随 G2 一并移除）', () => {
    expect(src).not.toContain('byMessage')
  })

  // ⚠️ 本条 2026-10-18 由 CL-06 T-19 修订（当日裁定标注形态 = "同角色连续只标首条"）：
  //    原断言是"依赖列表**恰好**这 4 项"，等于把"G2 角标依赖已回退"表达成全等。
  //    T-19 的标注需要多一项 `item.agentTagName`（**另一个特性**的依赖）⇒ 改为
  //    "4 项原依赖 + 本项白名单"的全等，**严格性不变**（任何其他多出的依赖仍会红）。
  //    上面两条（`emotion-chip` / `byMessage` 不得出现）继续承载"角标不许回来"的原意。
  it('v-memo 依赖列表 = 原有 4 项 + CL-06 T-19 的 agentTagName（不得再出现角标依赖）', () => {
    const m = src.match(/v-memo="(\[[^"]*\])"/)
    expect(m, '未找到 v-memo 依赖列表').not.toBeNull()
    expect(m![1]).toBe('[item.id, item.content, item.status, item.contentType, item.agentTagName]')
    expect(m![1], '不得包含情绪角标（G2）相关依赖').not.toMatch(/emotion/i)
  })
})
