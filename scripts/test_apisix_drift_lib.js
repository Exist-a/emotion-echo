// E2E-25 C2 RED：apisix_drift_lib 契约测试（node，无依赖，风格同 deploy/apisix/seed_test.js）
//
// 覆盖漂移检测工具的四个纯函数（无网络依赖，可在 CI 离线跑）：
//   normalizeSnapshot — 剥离易变字段 + 键排序，保证快照可比
//   diffSnapshots     — 两份快照的 added/removed/changed
//   extractSeedRouteIds — 从 seed.sh 源码提取"合法路由 id 白名单"
//   findExtras        — 运行时存在但 seed 白名单外的路由（route 116 类漂移）
//
// 运行：node scripts/test_apisix_drift_lib.js
// TDD：本文件先写（RED），实现 scripts/apisix_drift_lib.js（GREEN）。

const fs = require('fs');
const path = require('path');

const libPath = path.join(__dirname, 'apisix_drift_lib.js');
if (!fs.existsSync(libPath)) {
  console.error('  ✗ apisix_drift_lib.js not found (RED 阶段：实现尚未落地)');
  process.exit(1);
}
const lib = require(libPath);

let failed = 0;
let passed = 0;
function check(name, cond) {
  if (cond) { passed++; console.log('  ✓ ' + name); }
  else { failed++; console.error('  ✗ ' + name); }
}

// ---- normalizeSnapshot ----
const raw1 = {
  routes: [
    { key: '/apisix/routes/100', createdIndex: 1, modifiedIndex: 99,
      value: { id: '100', uri: '/api/v1/*', status: 1, create_time: 111, update_time: 222,
        plugins: { 'jwt-auth': { header: 'authorization' } }, upstream_id: 6 } },
    { key: '/apisix/routes/110', createdIndex: 2, modifiedIndex: 98,
      value: { id: '110', uri: '/api/v1/auth/login', status: 1, create_time: 1, update_time: 2 } }
  ],
  upstreams: [
    { key: '/apisix/upstreams/1', createdIndex: 3, modifiedIndex: 97,
      value: { id: '1', name: 'user-svc', discovery_type: 'nacos', update_time: 5, create_time: 4 } }
  ],
  consumers: [
    { key: '/apisix/consumers/emotion_echo_bff', createdIndex: 4, modifiedIndex: 96,
      value: { username: 'emotion_echo_bff', create_time: 7, update_time: 8 } }
  ]
};
// 与 raw1 仅易变字段不同（时间戳/索引/顺序），规范化结果必须相等
const raw2 = {
  consumers: [
    { key: '/apisix/consumers/emotion_echo_bff', modifiedIndex: 555, createdIndex: 444,
      value: { update_time: 999, create_time: 888, username: 'emotion_echo_bff' } }
  ],
  upstreams: [
    { key: '/apisix/upstreams/1', modifiedIndex: 333, createdIndex: 222,
      value: { create_time: 0, update_time: 1, discovery_type: 'nacos', name: 'user-svc', id: '1' } }
  ],
  routes: [
    { key: '/apisix/routes/110', modifiedIndex: 111, createdIndex: 222,
      value: { update_time: 3, create_time: 4, status: 1, uri: '/api/v1/auth/login', id: '110' } },
    { key: '/apisix/routes/100', modifiedIndex: 11, createdIndex: 22,
      value: { upstream_id: 6, plugins: { 'jwt-auth': { header: 'authorization' } }, status: 1,
        create_time: 111, update_time: 222, uri: '/api/v1/*', id: '100' } }
  ]
};
const n1 = lib.normalizeSnapshot(raw1);
const n2 = lib.normalizeSnapshot(raw2);
check('normalize 剥离易变字段（create_time/update_time/index/key）后两份等价输入快照相等',
  JSON.stringify(n1) === JSON.stringify(n2));
check('normalize 输出按 对象类型+id 确定性排序',
  JSON.stringify(n1) === JSON.stringify([...n1].sort()) &&
  n1[0].startsWith('consumers/') && n1[1].startsWith('routes/100') && n1[2].startsWith('routes/110'));
check('normalize 是行数组且每行含类型前缀',
  Array.isArray(n1) && n1.every(l => /^(routes|upstreams|consumers)\//.test(l)));

// 实质变更必须被保留（不是把一切都抹平成相等）
const raw3 = JSON.parse(JSON.stringify(raw1));
raw3.routes[0].value.status = 0;
check('normalize 保留实质差异（status 变化可检出）',
  JSON.stringify(lib.normalizeSnapshot(raw1)) !== JSON.stringify(lib.normalizeSnapshot(raw3)));

// ---- diffSnapshots ----
const base = ['consumers/c1', 'routes/100 X', 'upstreams/1 Y'];
check('diff 无差异 → 空结果', JSON.stringify(lib.diffSnapshots(base, [...base])) === '{"added":[],"removed":[],"changed":[]}');
check('diff added', lib.diffSnapshots(base, [...base, 'routes/299 Z']).added.includes('routes/299 Z'));
check('diff removed', lib.diffSnapshots(base, base.filter(l => l !== 'routes/100 X')).removed.includes('routes/100 X'));
const changed = [...base]; changed[1] = 'routes/100 Y';
const d = lib.diffSnapshots(base, changed);
check('diff changed（同 id 内容不同）', d.changed.length === 1 && d.changed[0].id === '100');

// ---- extractSeedRouteIds ----
const fixture = [
  'put_route 100 "/api/v1/*" 6 \'["GET"]\'',
  'put_auth_route 110 "/api/v1/auth/login"',
  'put_auth_route 119 "/api/v1/client-error"',
  'put_route_health 200 "/user-health"          1',
  'if curl -sf -X PUT \\',
  '  "$ADMIN_URL/apisix/admin/routes/205" >/dev/null; then',
  'for drift_id in 116; do'
].join('\n');
const ids = lib.extractSeedRouteIds(fixture);
check('extractSeedRouteIds 提取 put_route/put_auth_route/put_route_health 调用 id',
  ids.has('100') && ids.has('110') && ids.has('119') && ids.has('200'));
check('extractSeedRouteIds 提取内联自健康路由 id（205）', ids.has('205'));
check('extractSeedRouteIds 不把漂移清理目标当白名单（116 排除）', !ids.has('116'));

// ---- findExtras ----
const runtime = [{ id: '100' }, { id: '205' }, { id: '299' }];
check('findExtras 找出白名单外路由（299）',
  JSON.stringify(lib.findExtras(runtime, ids)) === JSON.stringify(['299']));
check('findExtras 白名单内无 extras',
  lib.findExtras([{ id: '100' }, { id: '205' }], ids).length === 0);

console.log(`\nSummary: PASS=${passed} FAIL=${failed}`);
process.exit(failed ? 1 : 0);
