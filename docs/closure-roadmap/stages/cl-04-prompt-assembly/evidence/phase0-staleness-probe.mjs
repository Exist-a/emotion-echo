// docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/phase0-staleness-probe.mjs
//
// CL-04 · Phase 0 附带取证 —— 运行栈是否真的缺 827f654 的"游标分页"代码（2026-10-14）
//
// 背景：`scripts/check_image_freshness.sh` 报 8/8 镜像 STALE，但其判据是**纯时间比较**
//       （镜像 Created vs 该服务源码路径最近 commit 的提交时间），无法区分
//       "镜像建于工作副本已含该改动、改动稍后才被 commit" ⇒ 存在假红可能。
//       本探针用**行为**判真假：
//         - 827f654 后，BFF `data` 应为 `{list, cursor, hasMore}` 三键；旧代码只有 `list`。
//         - 旧 chat-svc 取"最早 limit 条"，新代码取"最新 limit 条" ⇒ ?limit=2 可判别方向。
//
// 用法：node phase0-staleness-probe.mjs [conversationId]

const BASE = process.env.API_BASE || 'http://localhost:19080'
const convId = process.argv[2] || '667'

const login = await fetch(`${BASE}/api/v1/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'echo', password: 'echo123' }),
})
const token = (await login.json())?.data?.accessToken
if (!token) {
  console.error('FAIL: 登录未拿到 token')
  process.exit(1)
}
const H = { Authorization: `Bearer ${token}` }

async function get(url) {
  const res = await fetch(url, { headers: H })
  const body = await res.json()
  return { status: res.status, body }
}

const full = await get(`${BASE}/api/v1/conversations/${convId}/messages`)
const dataKeys = Array.isArray(full.body?.data) ? 'array' : Object.keys(full.body?.data || {})
console.log(`GET /messages            status=${full.status} data keys=${JSON.stringify(dataKeys)}`)
console.log(`  hasMore 键存在 = ${'hasMore' in (full.body?.data || {})}  cursor 键存在 = ${'cursor' in (full.body?.data || {})}`)

const limited = await get(`${BASE}/api/v1/conversations/${convId}/messages?limit=2`)
const limList = limited.body?.data?.list || []
console.log(`GET /messages?limit=2    status=${limited.status} 返回 ${limList.length} 条 ids=[${limList.map((m) => m.id).join(',')}]`)
console.log(`  全量最后两条 ids=[${(full.body?.data?.list || []).slice(-2).map((m) => m.id).join(',')}]`)

const hasCursorContract = dataKeys.includes?.('hasMore') || dataKeys.includes('hasMore')
const newestFirst =
  limList.length === 2 &&
  limList.every((m, i) => i === 0 || Number(limList[i - 1].id) < Number(m.id)) &&
  Number(limList[limList.length - 1].id) ===
    Number((full.body?.data?.list || []).slice(-1)[0]?.id)

console.log('\n=== 判定 ===')
console.log(`运行栈含 827f654 的"游标分页"契约（data 有 hasMore）: ${hasCursorContract}`)
console.log(`?limit=2 取到的是"最新两条"(newest-first 语义): ${newestFirst}`)
console.log(
  hasCursorContract
    ? '⇒ 镜像虽按时间判 STALE，但行为已含该改动（守卫假红）'
    : '⇒ 运行镜像确实缺该改动（守卫真红），重建前不得用其运行时行为作证据',
)
