// Stage 32 PR-15: seed.sh 离线结构断言（Node.js，无依赖）
//
// 覆盖 seed.sh 的所有可静态校验点：bash 语法、变量定义、upstream ID、路由、插件链、退出码契约。
// 不需要 mock 网络或 APISIX 实例——纯文本与字符串包含校验。
//
// 运行：node deploy/apisix/seed_test.js
//
// 注：bash 语法检查用 `bash -n`；其他为字符串/正则匹配。
// 端到端集成验证需 docker compose up 后手动跑 ./deploy/apisix/seed.sh。

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// Git Bash on Windows: mode 位检测不可靠，用 fs.accessSync 替代
function isExecutable(filePath) {
  try {
    fs.accessSync(filePath, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

const SCRIPT_DIR = __dirname;
const SEED_SH = path.join(SCRIPT_DIR, 'seed.sh');
const CONFIG_YAML = path.join(SCRIPT_DIR, 'config.yaml');

function fail(msg) {
  console.error('  ✗ ' + msg);
  process.exitCode = 1;
}
function pass(msg) {
  console.log('  ✓ ' + msg);
}

if (!fs.existsSync(SEED_SH)) {
  console.error('seed.sh not found at ' + SEED_SH);
  process.exit(1);
}
pass('seed.sh exists');

// PR-OBS-1 RED: APISIX skywalking endpoint 必须走容器 DNS + 正确端口 + 挂上 logger 插件
if (!fs.existsSync(CONFIG_YAML)) {
  console.error('config.yaml not found at ' + CONFIG_YAML);
  process.exit(1);
}
pass('config.yaml exists');

// bash -n syntax check
try {
  execSync(`bash -n "${SEED_SH}"`, { stdio: 'pipe' });
  pass('bash -n syntax OK');
} catch (e) {
  fail('bash -n failed: ' + e.stderr.toString());
}

const src = fs.readFileSync(SEED_SH, 'utf8');
const cfg = fs.readFileSync(CONFIG_YAML, 'utf8');
const checks = [
  ['seed.sh executable', isExecutable(SEED_SH)],
  ['set -euo pipefail', src.includes('set -euo pipefail')],
  ['ADMIN_KEY default matches seed.sh (Stage 39 后的实际值)',
    src.includes('APISIX_ADMIN_KEY:-WhZEPlrGviCSXlKFfALZlQWinluoGAbj')],
  ['JWT secret from BFF_JWT_SECRET (Stage 32 过渡)',
    src.includes('BFF_JWT_SECRET:-dev-bff-secret')],
  ['catch-all route /api/v1/* → web-bff (upstream 6)',
    src.includes('put_route 100 "/api/v1/*" 6')],
  ['upstream id 1 user-svc (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 1  user-svc')],
  ['upstream id 2 chat-svc (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 2  chat-svc')],
  ['upstream id 3 assessment-svc (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 3  assessment-svc')],
  ['upstream id 4 analytics-svc (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 4  analytics-svc')],
  ['upstream id 5 ai-svc (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 5  ai-svc')],
  ['upstream id 6 web-bff (Nacos discovery, Stage 39 后)',
    src.includes('put_nacos_upstream 6  web-bff')],
  ['jwt-auth plugin (审计 S-1 修复点)', src.includes('"jwt-auth"')],
  ['limit-count plugin', src.includes('"limit-count"')],
  ['limit-req plugin', src.includes('"limit-req"')],
  ['api-breaker plugin', src.includes('"api-breaker"')],
  ['cors plugin', src.includes('"cors"')],
  ['prometheus plugin', src.includes('"prometheus"')],
  ['health route /user-health', src.includes('/user-health')],
  ['health route /chat-health', src.includes('/chat-health')],
  ['health route /assessment-health', src.includes('/assessment-health')],
  ['health route /analytics-health', src.includes('/analytics-health')],
  ['health route /ai-health', src.includes('/ai-health')],
  ['apisix self-health route', src.includes('/apisix-health')],
  ['exit code 1 for APISIX unreachable (default)',
    /die\s+"[^"]*not reachable at[^"]*"/.test(src)],
  ['exit code 2 for upstream unhealthy',
    /die\s+"[^"]*not healthy[^"]*"\s+2\b/.test(src)],
  ['exit code 3 for PUT failure',
    /die\s+"[^"]*failed to PUT[^"]*"\s+3\b/.test(src)],
  ['jwt secret HS256 algorithm',
    src.includes('HS256') || src.includes('"algorithm"')],
  ['limit count = 60', src.includes('"count": 60')],
  ['limit time_window = 60s', src.includes('"time_window": 60')],
  ['api-breaker min_requests = 20', src.includes('"min_requests": 20')],
  ['api-breaker error_threshold_ratio = 0.5',
    src.includes('"error_threshold_ratio": 0.5')],
  ['api-breaker open_time = 30s', src.includes('"open_time": 30')],

  // === PR-OBS-1 RED assertions: APISIX skywalking endpoint + logger plugins ===
  // 依据 docs/plans/observability-sprint-b.md §2.3 + Stage 35 §78 dial fail 现象
  // 修 1: skywalking endpoint 必须走容器 DNS(非 127.0.0.1)+ HTTP receiver 端口 12800
  ['PR-OBS-1 skywalking.endpoint_addr 容器 DNS (非 127.0.0.1)',
    /endpoint_addr:\s*http:\/\/emotion-echo-sw-oap/.test(cfg)],
  ['PR-OBS-1 skywalking.endpoint_addr 端口 = 12800 (HTTP/Log receiver)',
    /endpoint_addr:\s*http:\/\/emotion-echo-sw-oap:12800/.test(cfg)],
  ['PR-OBS-1 catch-all 主入口路由 plugins 含 skywalking-logger',
    src.includes('"skywalking-logger"')],
  ['PR-OBS-1 catch-all 主入口路由 plugins 含 file-logger',
    src.includes('"file-logger"')],

  // === Stage 74 RED: cors 插件字段名必须是 APISIX schema 的 allow_credential（单数） ===
  // seed.sh 曾写 allow_credentials（复数）→ 插件静默忽略 → 不发
  // Access-Control-Allow-Credentials 头 → 前端 credentials:include 请求全部
  // "Failed to fetch"（stage-74 dev 栈 web 容器实测暴露）
  ['Stage 74 cors allow_credential 字段名（单数，APISIX schema）',
    src.includes('"allow_credential": true')],
  ['Stage 74 cors 不再使用错误的 allow_credentials 复数字段',
    !src.includes('"allow_credentials"')],
];

let passCount = 0, failCount = 0;
for (const [name, ok] of checks) {
  if (ok) {
    pass(name);
    passCount++;
  } else {
    fail(name);
    failCount++;
  }
}

console.log('');
console.log('Summary: PASS=' + passCount + ' FAIL=' + failCount);
if (failCount > 0) process.exit(1);
