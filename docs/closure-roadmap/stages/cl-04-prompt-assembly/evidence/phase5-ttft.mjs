// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/phase5-ttft.mjs
//
// CL-04 · Phase 5（T-11）附带取证：**WithIntent 接回对首字延迟（TTFT）的影响**。
//
// 背景：实测 8 条真实样例中 6 条规则式分类拿不准 ⇒ 接回后这些消息会在**首字之前**
// 多一次 LLM 消歧调用（≤5s）。必须用真实数据回答"到底慢多少"，不能靠猜。
//
// 跑法（A/B 同一脚本）：
//   基线：先对**未含 with_intent** 的镜像跑一次 → 保存输出
//   接回后：重建镜像 + recreate 再跑一次 → 对比 ttftMs
//
// 用法：node phase5-ttft.mjs [轮数]

const BASE = process.env.API_BASE || 'http://localhost:19080'
const ROUNDS = Number(process.argv[2] || 3)

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

const convId = (await (await fetch(`${BASE}/api/v1/conversations`, {
  method: 'POST',
  headers: H,
  body: '{}',
})).json())?.data?.id
if (!convId) {
  console.error('FAIL: 建会话失败')
  process.exit(1)
}

const questions = [
  '我下周要面试，一想到就心跳很快，有点焦虑',
  '是技术面，我最怕被问到项目细节',
  '那你觉得我该怎么准备上面那两个问题？',
  '今天天气不错，随便聊聊',
].slice(0, ROUNDS)

const rows = []
for (let i = 0; i < questions.length; i++) {
  const q = questions[i]
  await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, {
    method: 'POST',
    headers: H,
    body: JSON.stringify({ role: 'user', content: q, contentType: 'text', clientMsgId: crypto.randomUUID() }),
  })

  const t0 = Date.now()
  const res = await fetch(`${BASE}/api/v1/ai/stream`, {
    method: 'POST',
    headers: H,
    body: JSON.stringify({ message: q, emotion: 'anxious', conversationId: String(convId) }),
  })

  // 逐块读取，记录**首个非空 delta** 的时刻（= 用户看到第一个字的延迟）
  const reader = res.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  let ttft = null
  let total = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    const lines = buf.split('\n')
    buf = lines.pop() || ''
    for (const line of lines) {
      const t = line.trim()
      if (!t.startsWith('data:')) continue
      const payload = t.slice(5).trim()
      if (!payload || payload === '[DONE]') continue
      try {
        const c = JSON.parse(payload)?.choices?.[0]?.delta?.content
        if (typeof c === 'string' && c.length > 0) {
          if (ttft === null) ttft = Date.now() - t0
          total += c
        }
      } catch {
        /* 非 delta */
      }
    }
  }
  const e2e = Date.now() - t0
  rows.push({ turn: i + 1, ttftMs: ttft, e2eMs: e2e, replyChars: [...total].length })
  console.log(`turn${i + 1}: ttft=${ttft}ms e2e=${e2e}ms replyChars=${[...total].length}`)
}

const ttfts = rows.map((r) => r.ttftMs).filter((v) => typeof v === 'number')
const avg = Math.round(ttfts.reduce((a, b) => a + b, 0) / (ttfts.length || 1))
console.log(`\nconversationId=${convId}  rounds=${ttfts.length}`)
console.log(`ttft: [${ttfts.join(', ')}] ms   平均 ${avg}ms`)
