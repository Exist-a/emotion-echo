// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/t18-title-check.mjs
//
// CL-04 · T-18 运行时验证：**会话标题是否会按第一轮对话自动概括**。
//
// 路径：登录取 token → 建会话（标题应为默认「对话 #NNN」）→ 落一条 user 消息 → 打 /ai/stream
//      （BFF 在 SSE 完成后按"消息数 ≤ 2"判定第一轮 → 生成标题）→ 回查会话列表看标题是否被改写。
//
// 用法：node t18-title-check.mjs   （需 dev 栈 + 已按 T-18 重建 web-bff）

const BASE = process.env.API_BASE || 'http://localhost:19080'
const USER_TEXT = '我最近总失眠，一躺下就开始想工作上的事，白天没精神，很焦虑'

const token = (await (await fetch(`${BASE}/api/v1/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'echo', password: 'echo123' }),
})).json())?.data?.accessToken
if (!token) {
  console.error('FAIL: 登录未拿到 token')
  process.exit(1)
}
const H = { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }

// 1) 建会话
const conv = await (await fetch(`${BASE}/api/v1/conversations`, { method: 'POST', headers: H, body: '{}' })).json()
const convId = conv?.data?.id
if (!convId) {
  console.error('FAIL: 建会话失败', JSON.stringify(conv).slice(0, 200))
  process.exit(1)
}
console.log(`conversationId=${convId}  建会话时标题=${JSON.stringify(conv?.data?.title ?? '(空)')}`)

// 2) 落 user 消息 + 打 /ai/stream（第一次交互）
await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, {
  method: 'POST',
  headers: H,
  body: JSON.stringify({ role: 'user', content: USER_TEXT, contentType: 'text', clientMsgId: crypto.randomUUID() }),
})
const t0 = Date.now()
const res = await fetch(`${BASE}/api/v1/ai/stream`, {
  method: 'POST',
  headers: H,
  body: JSON.stringify({ message: USER_TEXT, emotion: 'anxious', conversationId: String(convId) }),
})
const sse = await res.text()
let reply = ''
for (const line of sse.split('\n')) {
  const t = line.trim()
  if (!t.startsWith('data:')) continue
  const p = t.slice(5).trim()
  if (!p || p === '[DONE]') continue
  try {
    reply += JSON.parse(p)?.choices?.[0]?.delta?.content ?? ''
  } catch {}
}
console.log(`stream status=${res.status} 用时=${Date.now() - t0}ms 回复字数=${[...reply].length}`)

// 3) 等标题生成落库（best-effort，SSE 之后异步触发）
await new Promise((r) => setTimeout(r, 8000))

// 4) 回查会话标题
const list = await (await fetch(`${BASE}/api/v1/conversations?limit=5`, { headers: H })).json()
const items = list?.data?.list ?? list?.data?.conversations ?? []
const hit = items.find((c) => String(c.id) === String(convId))
if (!hit) {
  console.error('FAIL: 列表里没找到该会话', JSON.stringify(list).slice(0, 300))
  process.exit(1)
}
const title = String(hit.title ?? '')
const isDefault = /^对话\s*#?\d*$/.test(title.trim()) || title.trim() === ''
console.log(`\n回查标题 = ${JSON.stringify(title)}`)
console.log(`是否仍为默认标题 = ${isDefault}`)
console.log(isDefault ? '❌ FAIL：标题未被自动概括' : '✅ PASS：标题已按第一轮对话自动概括')
