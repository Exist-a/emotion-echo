// E2E-25 C2：APISIX seed↔admin 漂移检测纯函数库（node，无依赖）
//
// 被 scripts/check_apisix_drift.sh（网络侧 bash 包装）与
// scripts/test_apisix_drift_lib.js（契约测试）使用。
// 背景见 docs/e2e-roadmap/stages/e2e-25-apisix-gateway/plan.md §2 组 B（F-139）：
//   seed.sh 每次全量 PUT 会静默覆盖 admin 手工改动，且不清理白名单外的路由
//   （Stage 112 route 116 / E2E-25 实测 route 299 两起）。
'use strict';

const VOLATILE_KEYS = new Set(['create_time', 'update_time', 'createdIndex', 'modifiedIndex', 'key', 'checkedStatus']);

// 深拷贝并剥离易变字段；键按字典序排序，保证同构输入产出逐字节相同的 JSON。
function stableValue(v) {
  if (Array.isArray(v)) return v.map(stableValue);
  if (v && typeof v === 'object') {
    const out = {};
    for (const k of Object.keys(v).sort()) {
      if (VOLATILE_KEYS.has(k)) continue;
      out[k] = stableValue(v[k]);
    }
    return out;
  }
  return v;
}

// raw: {routes:[], upstreams:[], consumers:[]}（admin API 列表响应的合并体）
// 返回：规范化行数组，形如 "routes/100 {...json}"，按行排序（确定性）。
// admin 列表响应的列表键与包装形态（真网关 APISIX 3.18 admin v3 实测）：
//   GET /routes → {"total":N, "list":[{"key":..., "value":{...}}]}
// 兼容：裸数组 / {value:[...]} / {list:[...]}
function unwrapItems(v) {
  if (Array.isArray(v)) return v;
  if (v && Array.isArray(v.list)) return v.list;
  if (v && Array.isArray(v.value)) return v.value;
  return [];
}

function normalizeSnapshot(raw) {
  const lines = [];
  for (const kind of ['routes', 'upstreams', 'consumers']) {
    for (const item of unwrapItems(raw[kind])) {
      const v = stableValue(item.value || {});
      const id = v.id || item.key || '';
      lines.push(`${kind}/${id} ${JSON.stringify(v)}`);
    }
  }
  lines.sort();
  return lines;
}

// a/b: normalizeSnapshot 的输出。返回 {added, removed, changed}。
// changed：同 "kind/id" 前缀但内容不同（时间戳差异已被 normalize 剥离）。
function diffSnapshots(a, b) {
  const pa = new Map(a.map(l => [l.split(' ')[0], l]));
  const pb = new Map(b.map(l => [l.split(' ')[0], l]));
  const added = [], removed = [], changed = [];
  for (const [id, line] of pb) {
    if (!pa.has(id)) added.push(line);
    else if (pa.get(id) !== line) changed.push({ id, before: pa.get(id), after: line });
  }
  for (const [id, line] of pa) {
    if (!pb.has(id)) removed.push(line);
  }
  added.sort(); removed.sort(); changed.sort((x, y) => x.id < y.id ? -1 : 1);
  return { added, removed, changed };
}

// seed.sh 源码 → 合法路由 id 集合（字符串）。
// 覆盖：put_route / put_auth_route / put_route_health 调用行 + 内联 PUT 的自健康路由；
// 排除：漂移清理目标（for drift_id in 116）——它们不是白名单。
function extractSeedRouteIds(seedSrc) {
  const ids = new Set();
  for (const m of seedSrc.matchAll(/^put_(?:route|auth_route|route_health)\s+(\d+)/gm)) {
    ids.add(m[1]);
  }
  // 内联 PUT（自健康 route 205）："$ADMIN_URL/apisix/admin/routes/<id>"
  for (const m of seedSrc.matchAll(/apisix\/admin\/routes\/(\d+)/g)) {
    ids.add(m[1]);
  }
  // 漂移清理段：for drift_id in 116; do — 这些 id 会被 seed 删除，不是白名单
  for (const m of seedSrc.matchAll(/for\s+drift_id\s+in\s+([\d\s]+);/g)) {
    for (const d of m[1].trim().split(/\s+/)) ids.delete(d);
  }
  return ids;
}

// runtimeRoutes: [{id, ...}]（admin GET /routes 的 value 列表）
// seedIds: extractSeedRouteIds 输出。返回白名单外的 id（升序）。
function findExtras(runtimeRoutes, seedIds) {
  return runtimeRoutes
    .map(r => String(r.id))
    .filter(id => !seedIds.has(id))
    .sort((x, y) => Number(x) - Number(y));
}

module.exports = { normalizeSnapshot, diffSnapshots, extractSeedRouteIds, findExtras, stableValue };

// ---- CLI 入口（供 scripts/check_apisix_drift.sh 调用；不用 node -e，规避
// Git Bash/MSYS 对多行 -e 参数的静默吞改——本机实测 "node: -e requires an argument"）----
function main(argv) {
  const cmd = argv[0];
  const fs = require('fs');
  if (cmd === 'normalize') {
    const raw = JSON.parse(fs.readFileSync(0, 'utf8'));
    process.stdout.write(normalizeSnapshot(raw).join('\n') + '\n');
    return 0;
  }
  if (cmd === 'diff') {
    const a = fs.readFileSync(argv[1], 'utf8').split('\n').filter(Boolean);
    const b = fs.readFileSync(argv[2], 'utf8').split('\n').filter(Boolean);
    const d = diffSnapshots(a, b);
    if (!d.added.length && !d.removed.length && !d.changed.length) {
      console.log('[apisix-drift] no drift');
      return 0;
    }
    for (const l of d.added) console.log('[apisix-drift] ADDED   ' + l);
    for (const l of d.removed) console.log('[apisix-drift] REMOVED ' + l);
    for (const c of d.changed) {
      console.log('[apisix-drift] CHANGED ' + c.id);
      console.log('[apisix-drift]   before: ' + c.before);
      console.log('[apisix-drift]   after : ' + c.after);
    }
    return 2;
  }
  if (cmd === 'extras-from') {
    const seedSrc = fs.readFileSync(argv[1], 'utf8');
    const routes = unwrapItems(JSON.parse(fs.readFileSync(argv[2], 'utf8')))
      .map(it => ({ id: (it.value && it.value.id) || it.key }));
    const extras = findExtras(routes, extractSeedRouteIds(seedSrc));
    if (extras.length) { console.log(extras.join('\n')); return 1; }
    return 0;
  }
  console.error('usage: node apisix_drift_lib.js {normalize|diff|extras-from} ...');
  return 64;
}

if (require.main === module) {
  process.exit(main(process.argv.slice(2)));
}
