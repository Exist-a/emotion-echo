// app/utils/agentIdentity.ts
//
// CL-06 · UI 轮：角色卡的**身份标记**（首字徽记 + 色调）—— 纯函数，无 DOM、无副作用。
//
// 设计取舍：一个用户最多 5 个角色，卡片并排时最需要一眼看出"这是几个不同的人"。
// 颜色由**名字**决定而不是由 id 决定：预览态下角色还没有 id（还没落库），
// 而名字已经在输入框里了 —— 用名字做种子，预览与保存后的正式卡片才是同一枚徽记。
//
// 色调值域只有 AGENT_TONE_COUNT 个且全部取自项目调色板（见 AgentCard.vue 的 .tone-N），
// 故意的：这不是"给每个角色配一个专属色"的彩虹板，而是一套**安静的子集**——
// 几个角色并排时彼此可辨，但整页仍是一种色彩语言。

/** 身份色调的个数（必须与 AgentCard.vue 里 .tone-0 … .tone-N 的个数一致） */
export const AGENT_TONE_COUNT = 4

const FALLBACK_INITIAL = '角'

/**
 * 徽记首字。
 *
 * 用 `Array.from` 而不是 `name[0]`：后者按 UTF-16 码元切，emoji 名字（如「🌱小青」）
 * 会被劈成半个代理对，渲染成"□"。
 */
export function agentInitial(name?: string | null): string {
  const first = Array.from((name ?? '').trim())[0]
  return first ?? FALLBACK_INITIAL
}

/**
 * 由名字算出的稳定色调下标（0 ≤ i < AGENT_TONE_COUNT）。
 *
 * 用各字符码点之和取模：实现只有一行、无需依赖，且满足我们要的两条性质 ——
 * 同一名字恒定同一色调（刷新/换设备不变），不同名字大概率不同色调。
 * 刻意**不用** id 或随机数：随机会让每次重渲染都换色（那就成了无意义的装饰）。
 */
export function agentToneIndex(name?: string | null): number {
  const seed = (name ?? '').trim()
  if (seed === '') return 0
  let sum = 0
  for (const ch of seed) sum += ch.codePointAt(0) ?? 0
  return sum % AGENT_TONE_COUNT
}
