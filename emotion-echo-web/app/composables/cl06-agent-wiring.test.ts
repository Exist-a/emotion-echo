// app/composables/cl06-agent-wiring.test.ts
//
// CL-06 · Phase 7：**接线契约**（"功能真的通电了没有"）。
//
// 这一层刻意用源码契约而不是挂载测试，因为要钉的正是"链路上每一环都引用到了"：
// 本仓的 `shouldGenerateTitle`（字段没进结构 ⇒ 静默丢弃）、`contentType` 三个 tag
// （snake/camel 不合 ⇒ 前端发了收不到）都是"接线漏一环、单测全绿、用户侧失效"的形态。
//
// 覆盖四条链：
//   ① 前端 → 服务端：`agentId` 从页面 → sender → AIStreamParams（BFF 直接读该字段）；
//   ② 位置：切换器在 composer 内、**语音按钮左侧**（用户 2026-10-17 明确要求）；
//   ③ 音色：会话当前角色的 voice → TTS 每请求 voice（不带则与现状同形）；
//   ④ 角色列表加载：进入对话页会拉角色（否则切换器永远只有默认项）。
import { describe, it, expect } from 'vitest'
import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'

const read = (p: string) => readFileSync(resolve(__dirname, '..', p), 'utf8')

const streamSrc = read('composables/useAIStreamHandler.ts')
const senderSrc = read('composables/useConversationSender.ts')
const managerSrc = read('composables/useTTSManager.ts')
const dhSrc = read('composables/useDigitalHumanTTS.ts')
const playerSrc = read('composables/useTTSPlayer.ts')
const pageSrc = read('pages/chat/conversation/[id].vue')

describe('① agentId：页面 → sender → AIStreamParams', () => {
  it('AIStreamParams 含 agentId 可选字段（BFF 读的就是这个名字）', () => {
    expect(streamSrc).toMatch(/agentId\?:/)
  })

  it('AIStreamParams 的 agentId 会被序列化进请求体（body 由 params 直接 JSON 化）', () => {
    expect(streamSrc).toMatch(/JSON\.stringify\(params\)/)
  })

  it('useConversationSender 把 extraParams.agentId 透传到 params', () => {
    expect(senderSrc).toMatch(/agentId:\s*extraParams\?\.agentId/)
  })

  it('sender 的 extraParams 类型里有 agentId', () => {
    const idx = senderSrc.indexOf('extraParams?: {')
    expect(idx, 'sender 应声明 extraParams 结构').toBeGreaterThan(-1)
    expect(senderSrc.slice(idx, idx + 900)).toMatch(/agentId\?:/)
  })
})

describe('② 位置：切换器在语音按钮左侧', () => {
  it('composer-actions 内同时存在切换器与语音按钮', () => {
    expect(pageSrc).toMatch(/<AgentSwitcher/)
    expect(pageSrc).toMatch(/class="voice-btn"/)
  })

  it('切换器出现在语音按钮**之前**（左侧；用户 2026-10-17 明确要求）', () => {
    const iSwitcher = pageSrc.indexOf('<AgentSwitcher')
    const iVoice = pageSrc.indexOf('class="voice-btn"')
    expect(iSwitcher).toBeGreaterThan(-1)
    expect(iVoice).toBeGreaterThan(-1)
    expect(iSwitcher, 'AgentSwitcher 必须排在 voice-btn 之前（否则跑到语音按钮右侧）').toBeLessThan(
      iVoice,
    )
  })

  it('切换器绑定本会话 id（切换写的是当前会话）', () => {
    expect(pageSrc).toMatch(/<AgentSwitcher[\s\S]{0,200}:conversation-id="conversationIdRef"/)
  })

  it('切换器不得放在设置页（用户明确否定了"设置项"形态）', () => {
    const settingSrc = read('pages/chat/setting.vue')
    expect(settingSrc).not.toMatch(/AgentSwitcher/)
  })
})

describe('③ 音色：会话当前角色的 voice 逐请求带上', () => {
  it('useTTSManager.playText 接受可选 voice 并透传给底层播放', () => {
    expect(managerSrc).toMatch(/playText\s*=\s*\([^)]*voice/)
    expect(managerSrc).toMatch(/originalPlayText\([^)]*voice/)
  })

  it('useDigitalHumanTTS.playText 接受 voice 并传给 playStream', () => {
    expect(dhSrc).toMatch(/playText\s*=\s*async\s*\([^)]*voice/);
    expect(dhSrc).toMatch(/playStream\([\s\S]{0,200}voice/)
  })

  it('useTTSPlayer.playStream 把 voice 带到请求体', () => {
    // 取 playStream 的**形参块**（到 `): Promise<void> => {` 为止）再断言含 voice ——
    // 用固定字符窗口会被中文注释长度坑到（本测试第一版就写窄了，红得对）
    const start = playerSrc.indexOf('const playStream = async (')
    const end = playerSrc.indexOf('): Promise<void> => {', start)
    expect(start).toBeGreaterThan(-1)
    expect(end).toBeGreaterThan(start)
    expect(playerSrc.slice(start, end)).toMatch(/voice\?/)
    expect(playerSrc).toMatch(/fetchPhonemes\([^)]*voice/)
  })

  it('voice 为空时请求体不得出现 voice 键（零回归：不带 = 现状）', () => {
    // 断言是"条件展开"而不是恒发 `voice`（恒发会让不带角色的会话也带上空串）
    expect(playerSrc).toMatch(/\.\.\.\(voice\s*\?|voice\s*\?[^:]*:\s*undefined/)
  })

  it('对话页把"本会话生效角色的音色"接进 sender（而不是写死默认音色）', () => {
    expect(pageSrc).toMatch(/voice/)
    expect(pageSrc).toMatch(/useAgentStore|resolveActiveAgent|activeAgent/)
  })
})

describe('④ 角色列表加载与发送', () => {
  it('对话页进入时拉角色列表（否则切换器只有默认项）', () => {
    expect(pageSrc).toMatch(/fetchAgents/)
  })

  it('对话页发送时把当前角色 id 交给 sender', () => {
    expect(pageSrc).toMatch(/agentId/)
  })
})

describe('⑤ T-19：消息「这条是谁回的」标注接线（用户 2026-10-18 裁定形态 = 同角色连续只标首条）', () => {
  it('对话页用纯函数判定标注，并把结果显示为 .agent-tag', () => {
    expect(pageSrc).toMatch(/resolveAgentLabels/)
    expect(pageSrc).toMatch(/agent-tag/)
  })

  it('标注名算进 item（不读数组下标）—— 下标+读邻居与 v-memo 冲突，会把标注卡在旧值', () => {
    expect(pageSrc, '应在 data computed 里算好 agentTagName').toMatch(/agentTagName:/)
    expect(pageSrc).toMatch(/agentNameOf\(/)
    // 断言前**剥注释**：本仓反复踩"源码钉命中注释"（我自己解释这条设计的注释里
    // 就写了 `agentLabels[i]` 这个反例），不剥就会把说明文字当成缺陷。
    const codeOnly = pageSrc.replace(/<!--[\s\S]*?-->/g, '').replace(/(?<!:)\/\/[^\n]*/g, '')
    expect(codeOnly, '实现里不得再用下标取标注位').not.toMatch(/agentLabels\[/)
  })

  it('v-memo 依赖含 agentTagName（基本类型：标注变则记忆失效）', () => {
    const memo = pageSrc.match(/v-memo="\[[^\]]*\]"/)
    expect(memo, '应能找到 v-memo').not.toBeNull()
    expect(memo![0]).toMatch(/item\.agentTagName/)
  })

  it('渲染条件是"有标注名"—— 角色已删时名字解析不出 ⇒ 不标', () => {
    expect(pageSrc).toMatch(/v-if="item\.agentTagName"/)
  })

  // 跨层契约钉（本仓已有 CL-F-48「两端字段名不一致 ⇒ 静默丢字段」先例）：
  // 前端读的键名必须与 BFF 实际下发的 json tag 逐字一致。
  it('前端 MessageItem.agentId ⇄ BFF MessageItemVM 的 json tag 一致（字段名不得漂移）', () => {
    const vmPath = resolve(__dirname, '../../../emotion-echo-web-bff/internal/handler/viewmodel.go')
    if (!existsSync(vmPath)) {
      throw new Error(`读不到 BFF 视图模型（路径变了？）：${vmPath} —— 本钉不得静默跳过`)
    }
    const vmSrc = readFileSync(vmPath, 'utf8')
    expect(vmSrc, 'BFF 必须以 json:"agentId" 回带该字段').toMatch(/AgentID\s+int64\s+`json:"agentId"`/)

    const typesSrc = readFileSync(resolve(__dirname, '../types/api.ts'), 'utf8')
    const item = typesSrc.slice(typesSrc.indexOf('export interface MessageItem'))
    expect(item.slice(0, 1200), '前端 MessageItem 必须声明同名可选字段').toMatch(/agentId\?: number/)
  })
})
