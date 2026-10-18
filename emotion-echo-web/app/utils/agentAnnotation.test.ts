// app/utils/agentAnnotation.test.ts
//
// CL-06 · T-19：历史消息「这条是谁回的」的**标注判定**（纯函数）。
//
// 形态由用户 2026-10-18 裁定：**同一角色连续消息只标首条**（roadmap/CL-06 §N4 的原建议）。
//
// 关键语义（容易想错、必须钉住）：**"连续"是按 AI 消息之间比的，用户消息不打断**。
// 理由是聊天本身是轮流说话 —— 若每条用户消息都打断连续性，那每条 AI 回复都会被标注，
// 这个规则就等于不存在。
import { describe, it, expect } from 'vitest'
import { resolveAgentLabels } from './agentAnnotation'

const ai = (agentId?: number) => ({ sender: 'ai', agentId })
const user = () => ({ sender: 'user' })

describe('resolveAgentLabels · 基本形态', () => {
  it('空列表 → 空结果', () => {
    expect(resolveAgentLabels([])).toEqual([])
  })

  it('全是同一角色 ⇒ **只有第一条**标（连续段只标首条）', () => {
    expect(resolveAgentLabels([ai(36), ai(36), ai(36)])).toEqual([true, false, false])
  })

  it('换角色 ⇒ 每个新角色的首条都标', () => {
    expect(resolveAgentLabels([ai(36), ai(36), ai(38), ai(38)])).toEqual([true, false, true, false])
  })

  it('用户消息永远不标', () => {
    expect(resolveAgentLabels([user(), user()])).toEqual([false, false])
  })
})

describe('resolveAgentLabels · 用户消息不打断连续性（本规则的关键）', () => {
  it('AI → 用户 → 同一角色的 AI ⇒ 第二条 AI **不**重复标', () => {
    expect(resolveAgentLabels([ai(36), user(), ai(36)])).toEqual([true, false, false])
  })

  it('中间隔了多条用户消息也一样', () => {
    expect(resolveAgentLabels([ai(36), user(), user(), ai(36)])).toEqual([true, false, false, false])
  })

  it('换成另一个角色 ⇒ 隔了用户消息也要标新角色', () => {
    expect(resolveAgentLabels([ai(36), user(), ai(38)])).toEqual([true, false, true])
  })
})

describe('resolveAgentLabels · 角色功能上线前的历史消息（agentId 缺省/0）', () => {
  it('agentId 缺省 ⇒ 不标（那批消息根本没有角色，标了就是编造归属）', () => {
    expect(resolveAgentLabels([ai(undefined), ai(undefined)])).toEqual([false, false])
  })

  it('agentId=0 同样不标', () => {
    expect(resolveAgentLabels([ai(0)])).toEqual([false])
  })

  it('历史消息**不覆盖**上一个已知角色 ⇒ 夹在中间不会让同角色消息被重复标', () => {
    // 小E → 上线前旧消息 → 小E：第三段仍属"同一角色连续"，不该再标
    expect(resolveAgentLabels([ai(36), ai(undefined), ai(36)])).toEqual([true, false, false])
  })

  it('上线前旧消息之后的**新角色**仍然要标（不能因为前面是 0 就吞掉）', () => {
    expect(resolveAgentLabels([ai(undefined), ai(38)])).toEqual([false, true])
  })
})
