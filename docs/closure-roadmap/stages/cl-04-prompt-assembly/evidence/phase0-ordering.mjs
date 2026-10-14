// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/phase0-ordering.mjs
//
// CL-04 · Phase 0 附带取证（2026-10-14）
//
// 目的：运行时确认 chat-svc ListMessages 的**排序语义**（plan §A.3 假设表：源码读为
//       id DESC 取最新 limit 条后**反转为时间升序**；§6 R5 记录 `ai_stream_handler.go:318`
//       的陈旧注释写成"倒序"）。Phase 2 的历史注入直接依赖这个顺序。
//
// 用法：node phase0-ordering.mjs [conversationId]
// 退出码：0 = 成功产出

const BASE = process.env.API_BASE || 'http://localhost:19080'
const convId = process.argv[2] || process.env.CONV_ID
if (!convId) {
  console.error('用法: node phase0-ordering.mjs <conversationId>')
  process.exit(1)
}

const loginRes = await fetch(`${BASE}/api/v1/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'echo', password: 'echo123' }),
})
const token = (await loginRes.json())?.data?.accessToken
if (!token) {
  console.error('FAIL: 登录未拿到 token')
  process.exit(1)
}

const res = await fetch(`${BASE}/api/v1/conversations/${convId}/messages`, {
  headers: { Authorization: `Bearer ${token}` },
})
const body = await res.json()
const list = body?.data?.list ?? []

console.log(`conversationId=${convId} status=${res.status} count=${list.length}`)
console.log('顺序（数组下标 → id/role/contentType/内容头）：')
list.forEach((m, i) => {
  console.log(`  [${i}] id=${m.id} role=${m.role} contentType=${m.contentType} head=${String(m.content || '').slice(0, 18)}`)
})

const ids = list.map((m) => Number(m.id))
const ascending = ids.every((v, i) => i === 0 || ids[i - 1] < v)
console.log(`\n判定: ids 升序(旧→新) = ${ascending}   ids=[${ids.join(',')}]`)
