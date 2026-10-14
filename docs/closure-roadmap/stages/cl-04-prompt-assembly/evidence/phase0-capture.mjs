// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/phase0-capture.mjs
//
// CL-04 · Phase 0 基线取证（2026-10-14）
//
// 目的：把「BFF 只向 llm-service 发 [system, 最后一条 user] 两条消息（多轮＝无记忆单轮）」
//       从**源码结论**升级为**运行时证据**。
//
// 手法：走 APISIX 网关（唯一业务入口，:19080）真实发 3 轮对话 —— 每轮先把 user 消息
//       落库（/conversations/:id/messages），再打 /ai/stream。跑完后对照
//       `docker logs emotion-llm-service` 的 "ChatCompletion request: messages=N" 行：
//       第 3 轮时会话里已有 6 条消息，但 N 仍应为 2 ⇒ 证伪"历史进了 prompt"。
//
// 用法：node phase0-capture.mjs
// 退出码：0 = 成功产出；1 = 前置失败（登录/建会话）

const BASE = process.env.API_BASE || 'http://localhost:19080'
const DEMO = { username: 'echo', password: 'echo123' }
const out = { base: BASE, turns: [] }

async function parse(res) {
  const text = await res.text()
  try {
    return JSON.parse(text)
  } catch {
    return { raw: text.slice(0, 300) }
  }
}

// 1) 登录（网关路由 /api/v1/auth/login）
const loginRes = await fetch(`${BASE}/api/v1/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(DEMO),
})
const loginBody = await parse(loginRes)
const token = loginBody?.data?.accessToken
if (!token) {
  console.error('PHASE0 FAIL: 登录未拿到 accessToken', loginRes.status, JSON.stringify(loginBody).slice(0, 300))
  process.exit(1)
}
out.login = { status: loginRes.status, gotToken: true }
const H = { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }

// 2) 建会话
const convRes = await fetch(`${BASE}/api/v1/conversations`, { method: 'POST', headers: H, body: '{}' })
const convBody = await parse(convRes)
const convId = convBody?.data?.id
if (!convId) {
  console.error('PHASE0 FAIL: 建会话未拿到 id', convRes.status, JSON.stringify(convBody).slice(0, 300))
  process.exit(1)
}
out.conversationId = convId

// 3) 三轮对话（内容刻意带"跨轮依赖"：第 3 轮指代第 1 轮的"面试"）
const questions = [
  '我下周要面试，一想到就心跳很快，有点焦虑',
  '是技术面，我最怕被问到项目细节',
  '那你觉得我该怎么准备上面那两个问题？',
]

for (let i = 0; i < questions.length; i++) {
  const q = questions[i]

  // 3a) user 消息落库（前端真实流程：messageStore.sendMessage → chat-svc）
  const msgRes = await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, {
    method: 'POST',
    headers: H,
    body: JSON.stringify({
      role: 'user',
      content: q,
      contentType: 'text',
      clientMsgId: crypto.randomUUID(),
    }),
  })

  // 3b) 打 /ai/stream（BFF 组装 prompt → llm-service）
  const streamRes = await fetch(`${BASE}/api/v1/ai/stream`, {
    method: 'POST',
    headers: H,
    body: JSON.stringify({ message: q, emotion: 'anxious', conversationId: String(convId) }),
  })
  const sse = await streamRes.text()
  let reply = ''
  for (const line of sse.split('\n')) {
    const t = line.trim()
    if (!t.startsWith('data:')) continue
    const payload = t.slice(5).trim()
    if (!payload || payload === '[DONE]') continue
    try {
      reply += JSON.parse(payload)?.choices?.[0]?.delta?.content ?? ''
    } catch {
      /* 非 delta 帧 */
    }
  }

  out.turns.push({
    turn: i + 1,
    messagePersistStatus: msgRes.status,
    streamStatus: streamRes.status,
    replyChars: reply.length, // ← Phase 3 落地后与本基线对照"是否变啰嗦"（plan §6 R1）
    replyHead: reply.slice(0, 60),
  })
  console.log(`turn${i + 1}: persist=${msgRes.status} stream=${streamRes.status} replyChars=${reply.length}`)
}

// 4) 会话当前消息总数（证明"库里确实有历史"，而非空会话）
const listRes = await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, { headers: H })
const listBody = await parse(listRes)
const list = listBody?.data?.messages || listBody?.data?.list || listBody?.data || []
out.conversationMessageCount = Array.isArray(list) ? list.length : null
out.listShape = Array.isArray(listBody?.data) ? 'array' : Object.keys(listBody?.data || {})

console.log('\n=== PHASE0 SUMMARY (paste into report) ===')
console.log(JSON.stringify(out, null, 1))
