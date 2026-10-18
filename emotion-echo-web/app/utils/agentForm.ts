// app/utils/agentForm.ts
//
// CL-06（用户自定义 Agent）：角色表单的**纯逻辑** —— 上限常量、前端早提示校验、预览文案。
//
// 为什么抽成纯函数：组件里的表单校验脱离 DOM 测不了，而"上限"与"预览文案"恰是用户最直接
// 感知的两件事（T-15/T-17）。组件只做绑定与渲染。
//
// ⚠️ **前端校验不是安全边界**：服务端（user-svc `agentlogic.go` + `shared/pkg/agentguard`）
// 会再判一次上限与内容黑名单；绕过前端直接 curl 一样会被拒。这里只为"早提示、少一次往返"。
//
// 与后端的字段口径必须一致（改一处要改两处 —— 由 `agentForm.test.ts` 的上限断言 + 服务端
// `MaxAgent*` 常量测试共同看住）。

import type { AgentProfile, AgentProfilePayload } from '~/types/api'

/** 字段上限（与 user-svc `MaxAgent*` / 源计划 §4 对齐） */
export const AGENT_LIMITS = {
  name: 20,
  persona: 500,
  tone: 100,
  boundariesCount: 5,
  boundary: 50,
  profilesPerUser: 5,
} as const

export interface VoiceOption {
  value: string
  label: string
  gender: '女' | '男'
}

/** 8 个 CosyVoice2 预置音色（带模型名前缀；顺序即下拉展示顺序，女声在前） */
export const VOICE_OPTIONS: VoiceOption[] = [
  { value: 'FunAudioLLM/CosyVoice2-0.5B:anna', label: 'anna（温柔女声）', gender: '女' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:bella', label: 'bella（明亮女声）', gender: '女' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:claire', label: 'claire（知性女声）', gender: '女' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:diana', label: 'diana（沉稳女声）', gender: '女' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:alex', label: 'alex（清朗男声）', gender: '男' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:benjamin', label: 'benjamin（温和男声）', gender: '男' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:charles', label: 'charles（低沉男声）', gender: '男' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:david', label: 'david（年轻男声）', gender: '男' },
]

export type AgentGender = '女' | '男' | '不指定'

/**
 * 音色值的**用户可读名**（如 `anna（温柔女声）`）。
 *
 * 兜底刻意写「默认音色」而不是把原值回显：`voice` 存的是
 * `FunAudioLLM/CosyVoice2-0.5B:anna` 这类模型串，直接显示是把实现细节端到用户面前。
 * 认不出（旧数据 / 音色下线）时给一个人话，用户至少知道"它会用默认嗓音"。
 */
export function voiceLabelOf(value?: string | null): string {
  return VOICE_OPTIONS.find((v) => v.value === value)?.label ?? '默认音色'
}

export interface AgentForm {
  id?: number
  name: string
  gender: AgentGender
  persona: string
  tone: string
  boundaries: string[]
  voice: string
}

export function emptyAgentForm(): AgentForm {
  return {
    name: '',
    gender: '不指定',
    persona: '',
    tone: '',
    boundaries: [],
    voice: VOICE_OPTIONS[0]!.value,
  }
}

export function formFromProfile(p: AgentProfile): AgentForm {
  return {
    id: p.id,
    name: p.name,
    gender: (p.gender as AgentGender) || '不指定',
    persona: p.persona ?? '',
    tone: p.tone ?? '',
    boundaries: [...(p.boundaries ?? [])],
    voice: p.voice || VOICE_OPTIONS[0]!.value,
  }
}

export function formToPayload(f: AgentForm): AgentProfilePayload {
  return {
    id: f.id,
    name: f.name.trim(),
    gender: f.gender,
    persona: f.persona.trim(),
    tone: f.tone.trim(),
    boundaries: f.boundaries.map((b) => b.trim()).filter((b) => b.length > 0),
    voice: f.voice,
  }
}

const runeLen = (s: string) => Array.from(s ?? '').length

/**
 * 前端早提示校验。返回空数组 = 可提交。
 *
 * 文案刻意与后端**同类但更口语**（后端错误形如 `field=persona category=jailbreak`，
 * 前者给开发者、后者给用户）。**内容黑名单不在这里**：词表在服务端，前端不做第二份
 * （否则必然漂移，且错误信息不如服务端准确）。
 */
export function validateAgentForm(f: AgentForm): string[] {
  const errs: string[] = []
  const name = f.name?.trim() ?? ''
  if (!name) {
    errs.push('角色名不能为空')
  } else if (runeLen(name) > AGENT_LIMITS.name) {
    errs.push(`角色名不能超过 ${AGENT_LIMITS.name} 个字`)
  }

  if (runeLen(f.persona) > AGENT_LIMITS.persona) {
    errs.push(`人设不能超过 ${AGENT_LIMITS.persona} 个字`)
  }
  if (runeLen(f.tone) > AGENT_LIMITS.tone) {
    errs.push(`语气不能超过 ${AGENT_LIMITS.tone} 个字`)
  }

  const boundaries = (f.boundaries ?? []).map((b) => b.trim()).filter((b) => b.length > 0)
  if (boundaries.length > AGENT_LIMITS.boundariesCount) {
    errs.push(`最多 ${AGENT_LIMITS.boundariesCount} 条「不要」`)
  }
  if (boundaries.some((b) => runeLen(b) > AGENT_LIMITS.boundary)) {
    errs.push(`每条「不要」不能超过 ${AGENT_LIMITS.boundary} 个字`)
  }

  if (f.voice && !VOICE_OPTIONS.some((v) => v.value === f.voice)) {
    errs.push('音色不合法')
  }
  return errs
}

/**
 * 「预览你的角色会怎样自我介绍」。
 *
 * 与服务端 prompt 段的**同构纪律**：空字段不产生孤立标签（不会出现"人设："这种空行）；
 * 并明确点出"这只改怎么说话、不改变安全准则" —— 与 `safetyGuardSegmentText` 的口径一致，
 * 避免用户误以为可以改掉底线。
 */
export function buildAgentPreview(f: AgentForm): string {
  const name = f.name?.trim() || '这个角色'
  const parts: string[] = [`我是${name}。`]

  const persona = f.persona?.trim()
  if (persona) parts.push(`人设：${persona}`)
  const tone = f.tone?.trim()
  if (tone) parts.push(`语气：${tone}`)

  const boundaries = (f.boundaries ?? []).map((b) => b.trim()).filter((b) => b.length > 0)
  if (boundaries.length > 0) parts.push(`不要：${boundaries.join('；')}`)

  parts.push('（这是你为用户设定的角色：只影响怎么称呼与说话风格，不改变它的身份与安全准则。）')
  return parts.join('\n')
}
