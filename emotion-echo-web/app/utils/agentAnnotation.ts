// app/utils/agentAnnotation.ts
//
// CL-06 · T-19（2026-10-18 用户裁定形态）：历史消息「这条是谁回的」的标注判定。
//
// 抽成纯函数的原因：这段判定有两条容易想错的语义（用户消息**不打断**连续性、
// agentId 缺省的历史消息**不覆盖**上一个已知角色），而它们在 UI 里既难测也难说清。
//
// 数据来源：BFF 已把每条 AI 消息的**写库时角色快照**回带为 `agentId`
// （`emotion-echo-web-bff/internal/handler/viewmodel.go:45`，`json:"agentId"`；
// chat-svc 侧 `chat_server.go:73` 映射；proto `chat.pb.go` `agent_id=9`）。
// `0` / 缺省 = 角色功能上线前的历史消息 ⇒ 不标注。

/** 判定所需的最小消息形状（不绑定具体的消息类型，便于测试与复用） */
export interface AnnotatableMessage {
  sender: string
  /** 写库时的角色 id；0 / 缺省 = 上线前历史消息 */
  agentId?: number
}

/**
 * 逐条给出「是否渲染角色标注」。
 *
 * 规则（= 用户裁定的形态「同一角色连续消息只标首条」）：
 *   ① 用户消息永不标；
 *   ② AI 消息在**与上一条有角色的 AI 消息角色不同**时标 —— 用户消息**不打断**连续性
 *      （聊天本就轮流说话，否则每条 AI 回复都会被标注，规则等于不存在）；
 *   ③ `agentId` 缺省/0 的历史消息不标，且**不覆盖**上一个已知角色 ——
 *      这样"小E → 旧消息 → 小E"不会把同一段连续拆成两段而重复标注。
 */
export function resolveAgentLabels(messages: AnnotatableMessage[]): boolean[] {
  const out: boolean[] = []
  // null = 还没有遇到过"带角色的 AI 消息"
  let lastKnownAgentId: number | null = null

  for (const m of messages) {
    if (m.sender !== 'ai') {
      out.push(false)
      continue
    }
    const id = Number(m.agentId ?? 0)
    if (!Number.isFinite(id) || id <= 0) {
      out.push(false)
      continue
    }
    out.push(id !== lastKnownAgentId)
    lastKnownAgentId = id
  }

  return out
}
