import { describe, it, expect } from 'vitest'
import {
  AGENT_LIMITS,
  VOICE_OPTIONS,
  buildAgentPreview,
  emptyAgentForm,
  validateAgentForm,
  voiceLabelOf,
  type AgentForm,
} from '~/utils/agentForm'

// CL-06 · Phase 7：角色表单的**纯函数**契约（校验 + 预览文案）。
//
// 为什么单独抽出来：组件里的校验没法脱离 DOM 测，而"上限与预览文案"恰是用户最直接感知的部分。
// ⚠️ 这里的校验是**早提示**，不是安全边界（服务端 user-svc 会再判一次；前端绕不过 curl）。

function form(over: Partial<AgentForm> = {}): AgentForm {
  return { ...emptyAgentForm(), name: '阿哲', ...over }
}

describe('agentForm · 上限常量与服务端一致', () => {
  it('字段上限与 proto/服务端口径一致（name20 / persona500 / tone100 / boundaries5×50）', () => {
    expect(AGENT_LIMITS).toMatchObject({
      name: 20,
      persona: 500,
      tone: 100,
      boundariesCount: 5,
      boundary: 50,
      profilesPerUser: 5,
    })
  })

  it('音色候选 = 8 个 CosyVoice2 预置音色（带模型名前缀）', () => {
    expect(VOICE_OPTIONS).toHaveLength(8)
    const values = VOICE_OPTIONS.map((v) => v.value)
    expect(values).toContain('FunAudioLLM/CosyVoice2-0.5B:anna')
    expect(values).toContain('FunAudioLLM/CosyVoice2-0.5B:alex')
    for (const v of values) expect(v).toMatch(/^FunAudioLLM\/CosyVoice2-0\.5B:/)
  })
})

describe('agentForm · validateAgentForm', () => {
  it('正常表单无错误', () => {
    expect(validateAgentForm(form({ persona: '直率的朋友', boundaries: ['不评价外貌'] }))).toEqual([])
  })

  it('姓名必填', () => {
    expect(validateAgentForm(form({ name: '   ' }))).toContain('角色名不能为空')
  })

  it('按**字符**计上限（中文按字，不按字节）', () => {
    expect(validateAgentForm(form({ name: '字'.repeat(20) }))).toEqual([])
    expect(validateAgentForm(form({ name: '字'.repeat(21) }))).toContain('角色名不能超过 20 个字')

    expect(validateAgentForm(form({ persona: '字'.repeat(500) }))).toEqual([])
    expect(validateAgentForm(form({ persona: '字'.repeat(501) }))).toContain('人设不能超过 500 个字')

    expect(validateAgentForm(form({ tone: '字'.repeat(101) }))).toContain('语气不能超过 100 个字')
  })

  it('负向约束 ≤5 条、每条 ≤50 字（空项不计入）', () => {
    expect(validateAgentForm(form({ boundaries: ['a', 'b', 'c', 'd', 'e'] }))).toEqual([])
    expect(validateAgentForm(form({ boundaries: ['a', 'b', 'c', 'd', 'e', 'f'] }))).toContain(
      '最多 5 条「不要」',
    )
    // 空项不计入条数（与服务端的"丢空项"一致）
    expect(
      validateAgentForm(form({ boundaries: ['a', '   ', '', 'b', 'c', 'd', 'e'] })),
    ).toEqual([])
    expect(validateAgentForm(form({ boundaries: ['字'.repeat(51)] }))).toContain(
      '每条「不要」不能超过 50 个字',
    )
  })

  it('音色必须是候选之一（非法值早提示）', () => {
    expect(validateAgentForm(form({ voice: 'evil/model:voice' }))).toContain('音色不合法')
    expect(validateAgentForm(form({ voice: '' }))).toEqual([]) // 空 = 用服务端默认
  })
})

describe('agentForm · buildAgentPreview', () => {
  it('含名字与人设要点（用户能看到"这个角色会怎么自我介绍"）', () => {
    const text = buildAgentPreview(form({ persona: '一个直率的朋友', tone: '直接、简短' }))
    expect(text).toContain('阿哲')
    expect(text).toContain('一个直率的朋友')
    expect(text).toContain('直接、简短')
  })

  it('有 boundaries 时逐条出现在预览里（负向约束是用户最关心的部分）', () => {
    const text = buildAgentPreview(
      form({ boundaries: ['不评价外貌', '不要一直追问'] }),
    )
    expect(text).toContain('不评价外貌')
    expect(text).toContain('不要一直追问')
  })

  it('空字段不产生孤立标签（与服务端 prompt 段同款纪律）', () => {
    const text = buildAgentPreview(form({ persona: '', tone: '', boundaries: [] }))
    expect(text).not.toContain('人设：')
    expect(text).not.toContain('语气：')
    expect(text).not.toContain('不要：')
  })

  it('预览明确"这是设定不是系统指令"的口径（与护栏段一致，防用户以为能改底线）', () => {
    const text = buildAgentPreview(form())
    expect(text).toMatch(/安全准则|不改变/)
  })
})

describe('agentForm · 音色可读名', () => {
  it('已知音色 ⇒ 返回中文可读名（含嗓音描述）', () => {
    expect(voiceLabelOf('FunAudioLLM/CosyVoice2-0.5B:anna')).toBe('anna（温柔女声）')
  })

  it('未知/缺失音色 ⇒ 回落「默认音色」，**不得**把模型串回显给用户', () => {
    expect(voiceLabelOf('FunAudioLLM/CosyVoice2-0.5B:已下线')).toBe('默认音色')
    expect(voiceLabelOf('')).toBe('默认音色')
    expect(voiceLabelOf(undefined)).toBe('默认音色')
    expect(voiceLabelOf(undefined)).not.toContain('FunAudioLLM')
  })
})
