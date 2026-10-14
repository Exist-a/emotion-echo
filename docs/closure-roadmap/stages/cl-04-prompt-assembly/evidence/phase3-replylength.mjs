// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/phase3-replylength.mjs
//
// CL-04 · Phase 3 附带取证：核对 plan §6 R1「MaxTokens=1024 是否让回复变啰嗦」。
// 做法：直接读会话里落库的 assistant 消息全文（不再打 LLM，零额外成本）。
//
// 用法：node phase3-replylength.mjs <conversationId>

const BASE = process.env.API_BASE || 'http://localhost:19080'
const convId = process.argv[2]
if (!convId) {
  console.error('用法: node phase3-replylength.mjs <conversationId>')
  process.exit(1)
}

const token = (await (await fetch(`${BASE}/api/v1/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'echo', password: 'echo123' }),
})).json())?.data?.accessToken
if (!token) {
  console.error('FAIL: 登录未拿到 token')
  process.exit(1)
}

const body = await (await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, {
  headers: { Authorization: `Bearer ${token}` },
})).json()
const list = body?.data?.list ?? []

const len = (s) => [...s].length
for (const m of list) {
  const n = len(String(m.content || ''))
  // 注意：读路径的 VM 字段是 `sender`（"ai" | "user"），不是写路径的 `role`
  // （chat_handler.go 的 MessageItemVM）—— 探针曾因此打印 undefined。
  const sender = m.sender ?? m.role
  if (sender === 'ai') {
    // 句子数近似：按中文句末标点切
    const sentences = String(m.content).split(/[。！？!?]/).filter((x) => x.trim()).length
    console.log(`\n[ai] runes=${n} 句子数≈${sentences}`)
    console.log(m.content)
  } else {
    console.log(`\n[${sender}] runes=${n} :: ${m.content}`)
  }
}
