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
const SERVICES_EXAMPLE = path.join(SCRIPT_DIR, 'services.env.example');

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
const infraYaml = fs.readFileSync(path.join(SCRIPT_DIR, '..', 'docker-compose.infra.yml'), 'utf8');
const checks = [
  ['seed.sh executable', isExecutable(SEED_SH)],
  // E2E-25 #11 弱断言修复：原断言 `src.includes('set -euo pipefail')` 会被
  // seed.sh:24 **注释**里的字面量满足，而脚本实际用 `set -eu`（ash 兼容，
  // 见 seed.sh 头注释）。断言必须匹配"实际生效的 set 行"，不得被注释满足。
  ['set -eu 严格模式（非注释行；ash 兼容，勿写 pipefail）',
    /^set -eu\s*$/m.test(src)],
  // E2E-F-69（2026-09-18）：原断言把**真实密钥**写进了测试文件 ⇒ 等于换个地方
  // 继续留在公开仓库里（测试文件也是仓库的一部分）。改为断言"安全性质"：
  // 默认值必须是非密钥占位符，且 seed.sh 不得内联任何长十六进制字面量。
  ['ADMIN_KEY 默认值为非密钥占位符（E2E-F-69）',
    /APISIX_ADMIN_KEY:-[a-z0-9-]*local-only/.test(src)],
  ['JWT_SECRET 默认值为非密钥占位符（E2E-F-69）',
    /BFF_JWT_SECRET:-[a-z0-9-]*local-only/.test(src)],
  ['seed.sh 不得内联真实密钥（E2E-F-69）',
    !/(APISIX_ADMIN_KEY|BFF_JWT_SECRET)[:=]\s*"?[0-9a-f]{32,}/i.test(src)],
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
  // E2E-20 #8 (Round 4.3 PR-2 后补): limit-count policy 走 Redis 避免多 APISIX 节点放大
  ['limit-count policy env-driven (E2E-20 #8 multi-instance)',
    /"policy":\s*"\$\{?LIMIT_POLICY\}?"/.test(src)],
  ['limit-count redis_host env-driven (E2E-20 #8)',
    /"redis_host":\s*"\$\{?LIMIT_REDIS_HOST\}?"/.test(src)],
  ['LIMIT_POLICY default = redis (E2E-20 #8)',
    /LIMIT_POLICY="\$\{LIMIT_POLICY:-redis\}"/.test(src)],
  // E2E-25 #17/N2：白名单路由的 limit-count 此前漏配 rejected_code ⇒ 限流拒绝走
  // APISIX 默认 503（openresty 裸页），与 catch-all 的 429 语义不一致——客户端无法
  // 区分"被限流"与"网关故障"（2026-10-02 实测 60×200 + 10×503）。
  ['E2E-25 AUTH_WHITELIST limit-count 补 rejected_code=429（N2）',
    (() => {
      const m = src.match(/AUTH_WHITELIST_PLUGINS=\$\(cat <<EOF([\s\S]*?)\nEOF\n\)/);
      return m ? /"rejected_code":\s*429/.test(m[1]) : false;
    })()],

  // === E2E-25 C1 RED：upstream 主动健康检查（F-154）===
  // 6 个 nacos-discovery upstream 此前完全没有 checks 段 ⇒ 节点健康 100% 外包给
  // Nacos 心跳 + discovery fetch_interval:30（D-30 实测摘除滞后 ≥75s）。
  // 依据 https://apisix.apache.org/docs/apisix/tutorials/health-check/ ：
  //   passive 单独无法把节点标回健康，必须与 active 组合 ⇒ checks 必须含 active。
  ['E2E-25 put_nacos_upstream JSON 含 checks 段（F-154）',
    (() => {
      const m = src.match(/put_nacos_upstream\(\) \{([\s\S]*?)\n\}/);
      return m ? /"checks"/.test(m[1]) : false;
    })()],
  ['E2E-25 checks.active 主动探测（passive 单独无法标回健康）',
    (() => {
      const m = src.match(/put_nacos_upstream\(\) \{([\s\S]*?)\n\}/);
      return m ? /"active"\s*:\s*\{/.test(m[1]) : false;
    })()],
  ['E2E-25 checks.active.http_path = /health（svc 健康端点）',
    (() => {
      const m = src.match(/put_nacos_upstream\(\) \{([\s\S]*?)\n\}/);
      return m ? /"http_path":\s*"\/health"/.test(m[1]) : false;
    })()],
  ['E2E-25 checks.active 阈值显式（healthy.successes + unhealthy.http_failures）',
    (() => {
      const m = src.match(/put_nacos_upstream\(\) \{([\s\S]*?)\n\}/);
      return m ? /"successes"/.test(m[1]) && /"http_failures"/.test(m[1]) : false;
    })()],

  // === E2E-25 C4 RED：死变量禁令（F-d）===
  // seed.sh 曾定义 PLUGINS_JSON（内含硬编码 "policy": "local" 的 limit-count）但
  // 全部路由 PUT 只引用 CATCHALL/AUTH_WHITELIST/HEALTH——改它不生效，属"配置分叉"
  // 型漂移陷阱。禁令：不得存在定义后未被引用的插件配置块。
  ['E2E-25 不存在死变量 PLUGINS_JSON（定义后必须被引用或删除，F-d）',
    !/^PLUGINS_JSON=\$\(cat <<EOF/m.test(src)],

  // === E2E-25 C3 RED：网关自身健康语义（F-f / N1）===
  // ① N1：route 205 /apisix-health 无 upstream ⇒ 命中即 openresty 503（2026-10-02
  //    实测）。修法 = echo 插件直接应答，无需 upstream。
  // ② F-f：compose healthcheck 原为纯 TCP 探 9080 ⇒ etcd 停摆时数据面 404/admin 报错
  //    而容器仍 healthy。修法 = 探 admin API（etcd 依赖的唯一直读面；镜像无 curl，
  //    用 bash /dev/tcp 发原始 HTTP GET，按状态行判 200）。
  ['E2E-25 route 205 self-health 挂 echo 插件（无 upstream 也可 200，N1）',
    (() => {
      const i = src.indexOf('"uri":"/apisix-health"');
      return i >= 0 && src.slice(i, i + 200).includes('"echo"');
    })()],
  ['E2E-25 apisix healthcheck 探 admin API 9180（F-f，etcd 依赖可观测）',
    /healthcheck:[\s\S]{0,700}?9180[\s\S]{0,300}?X-API-KEY/.test(infraYaml)],
  ['E2E-25 apisix healthcheck 不再是纯 TCP 探 9080（负向）',
    !/timeout 3 bash -c '<\/dev\/tcp\/127\.0\.0\.1\/9080'/.test(infraYaml)],
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

  // === Stage 105 RED: CORS_ALLOW_ORIGINS 默认值必须同时含 localhost:3000 与 127.0.0.1:3000 ===
  // 背景: dev 模式下 nuxt dev server 默认监听 0.0.0.0:3000。
  //   - 大多数用户用 http://localhost:3000 访问
  //   - 部分浏览器 (尤其 Windows Chrome + 沙箱 IAB) 把 localhost 当作 unsafe IP,
  //     自动重定向到 chrome-error://, 用户被迫改用 http://127.0.0.1:3000
  //   - 这两个 host 在浏览器 Origin header 看来不同源, 缺一即整页 API 请求被 CORS 拒绝
  //   - 旧 seed.sh 默认 allow_origins="http://localhost:3000" 单值, 导致后者访问时整页 Failed to fetch
  //   - Stage 103 启动 dev 模式只跑 curl smoke, 不模拟浏览器 Origin header → 漏抓
  // 修复: 默认值改为 "http://localhost:3000,http://127.0.0.1:3000"
  // 锁死: 未来不得改回单 host, 否则 dev 模式浏览器实测立刻炸
  (() => {
    const defaultMatch = src.match(/CORS_ALLOW_ORIGINS="\$\{CORS_ALLOW_ORIGINS:-\s*([^}]+)\}"/);
    const def = defaultMatch ? defaultMatch[1] : '';
    return ['Stage 105 cors 默认 allow_origins 必须包含 localhost:3000 和 127.0.0.1:3000',
      def.includes('localhost:3000') && def.includes('127.0.0.1:3000')];
  })(),

  // Stage 106: seed.sh file-logger log_format 不允许 trailing comma
  // Bug: 2026-09-16 dev 重启 web 时 apisix-seed 重跑 PUT route 100 失败,
  //   原因是 file-logger log_format 最后一个 key 带了逗号 → APISIX JSON schema 校验拒绝
  //   → catch-all /api/v1/* 路由缺失 → 所有非 auth 业务 401 unauthorized
  //   → 浏览器发消息无 AI 回复（叠加在 new.vue handleSubmit bug 上, 让人误判为前端 bug）
  // 修复: 删除 trailing comma；本断言保证未来不会再次引入
  // 扫描策略: 在 file-logger "log_format" 块内检查最后一条非空白行不以逗号结尾
  (() => {
    const blockMatch = src.match(/"log_format":\s*\{([\s\S]*?)\n\s*\}/);
    if (!blockMatch) return ['file-logger log_format 块结构 (用于 trailing comma 校验)', false];
    const block = blockMatch[1];
    // 去掉每行空白后找最后一行
    const lines = block.split('\n').map(l => l.trim()).filter(l => l.length > 0);
    const lastLine = lines[lines.length - 1];
    return ['file-logger log_format 最后一行不得以 "," 结尾 (Stage 106)',
      !lastLine.endsWith(',')];
  })(),

  // === E2E-F-114 RED (2026-09-22): dev 栈 JWT/admin secret 一致性 ===
  // 根因：HOST 直接跑 seed.sh 时环境无 BFF_JWT_SECRET，且 services.env.example
  // 用 plain 赋值 source ⇒ 占位符覆盖一切（连显式 env / compose 注入的真值都会被
  // 挪掉，正是"source .env.local 后重跑仍 401"的机制）⇒ consumer secret=占位符，
  // BFF 用 deploy/.env.local 真值签发 ⇒ 所有挂 jwt-auth 路由 401。
  ['E2E-F-114 seed.sh 自加载 ../.env.local 真值',
    src.includes('../.env.local')],
  ['E2E-F-114 加载时保留已有真值（env/services.env 优先于 .env.local）',
    src.includes('_load_env_local') && src.includes('real_admin') && src.includes('real_jwt')],
  ['E2E-F-114 JWT 仍是占位符时必须打 WARN（不许静默 401）',
    /WARN:[^"]*jwt/i.test(src)],
  ['E2E-F-114 services.env.example 用 ${VAR:-} 模式（plain 赋值会覆盖显式 env）',
    (() => {
      if (!fs.existsSync(SERVICES_EXAMPLE)) return false;
      const ex = fs.readFileSync(SERVICES_EXAMPLE, 'utf8');
      return ex.includes('${APISIX_ADMIN_KEY:-') &&
             ex.includes('${BFF_JWT_SECRET:-') &&
             ex.includes('${CORS_ALLOW_ORIGINS:-');
    })()],

  // === E2E-21 / E2E-F-13 RED (2026-09-28): X-Trace-Id 生产侧 ===
  // 缺陷：shared/pkg/middleware/gin_skywalking.go:73-77 消费 X-Trace-Id 塞进 slog ctx，
  // 但全仓**零处生产该 header**（实测 2026-09-28 前后：APISIX 只注入 X-User-Id）
  // ⇒ 生产环境每条 Go 日志的 trace_id 字段恒空，"按一次请求查全链路"做不到。
  ['E2E-F-13 seed.sh 定义 TRACE_ID_PLUGIN 共享片段', src.includes('TRACE_ID_PLUGIN=')],
  ['E2E-F-13 X-Trace-Id 取 ctx.var.request_id（与 access log 的 $apisix_request_id 同源）',
    src.split('\n').some(l =>
      l.includes('set_header(ctx,') && l.includes('X-Trace-Id') && l.includes('ctx.var.request_id'))],
  // E2E-21 实测抓到的真缺陷：file-logger 原本取 $http_x_request_id，
  // 那是**客户端传入的 header** —— 浏览器/curl 都不会带，nginx 直接把空变量
  // 整条省略，access log 里因此没有 trace_id 字段，
  // 于是"Go 日志 trace_id ↔ access log trace_id 可 join"永远不成立。
  ['E2E-F-13 file-logger trace_id 取 $apisix_request_id（网关生成值，非客户端 header）',
    src.includes('"trace_id": "$apisix_request_id"')],
  ['E2E-F-13 file-logger 不再用 $http_x_request_id（客户端不传 ⇒ 字段被整条省略）',
    !/"trace_id":\s*"\$http_x_request_id"/.test(src)],
  ['E2E-F-13 覆盖式赋值（不信任客户端自带的 X-Trace-Id）',
    src.split('\n').some(l =>
      l.includes('X-Trace-Id') && l.includes('ctx.var.request_id') && l.includes('set_header'))],

  // 关键：必须挂在**全部 3 组**在用插件变量上。
  // 曾经的坑：seed.sh 注释写「全局插件链（每个 route 共享）」，但 put_auth_route 用的是
  // AUTH_WHITELIST_PLUGINS、put_route_health 用的是 HEALTH_PLUGINS，两者都不含该片段
  // ⇒ 登录/注册/健康检查等 10 条路由仍无 trace_id（E2E-21 实测：15 条路由中仅
  // route 100 有 file-logger，其余全部没有）。
  // E2E-25（F-d）：原第 4 组 PLUGINS_JSON 是死变量，已删除（见下方死变量禁令）。
  ['E2E-F-13 TRACE_ID_PLUGIN 挂在 CATCHALL_PLUGINS_JSON（route 100 实际使用）',
    (() => {
      const m = src.match(/CATCHALL_PLUGINS_JSON=\$\(cat <<EOF([\s\S]*?)\nEOF\n\)/);
      return m ? m[1].includes('${TRACE_ID_PLUGIN}') : false;
    })()],
  ['E2E-F-13 TRACE_ID_PLUGIN 挂在 AUTH_WHITELIST_PLUGINS（登录/注册链路）',
    (() => {
      const m = src.match(/AUTH_WHITELIST_PLUGINS=\$\(cat <<EOF([\s\S]*?)\nEOF\n\)/);
      return m ? m[1].includes('${TRACE_ID_PLUGIN}') : false;
    })()],
  ['E2E-F-13 TRACE_ID_PLUGIN 挂在 HEALTH_PLUGINS（须用非引号 heredoc 才能引用变量）',
    (() => {
      const m = src.match(/HEALTH_PLUGINS=\$\(cat <<(EOF|'EOF')\n([\s\S]*?)\nEOF\n\)/);
      if (!m) return false;
      return m[1] === 'EOF' && m[2].includes('${TRACE_ID_PLUGIN}');
    })()],

  // 白名单路由此前连 access log 都没有（登录事件不可审计）
  ['E2E-F-13 AUTH_WHITELIST_PLUGINS 补上 observability 插件（file-logger/skywalking）',
    (() => {
      const m = src.match(/AUTH_WHITELIST_PLUGINS=\$\(cat <<EOF([\s\S]*?)\nEOF\n\)/);
      return m ? m[1].includes('${OBSERVABILITY_PLUGINS_JSON}') : false;
    })()],
  ['E2E-F-13 HEALTH_PLUGINS 刻意不加 file-logger（高频健康检查会冲掉有用日志）',
    (() => {
      const m = src.match(/HEALTH_PLUGINS=\$\(cat <<EOF\n([\s\S]*?)\nEOF\n\)/);
      return m ? !m[1].includes('${OBSERVABILITY_PLUGINS_JSON}') : false;
    })()],
  ['E2E-F-13 CORS allow_headers/expose_headers 含 X-Trace-Id（浏览器端可见该 ID）',
    src.includes('X-User-Id,X-Trace-Id')],

  // 上面几条只校验**源码文本**，踩过一个真实的坑，必须再校验**展开后的实际值**：
  // bash 的单引号字符串里根本无法嵌入单引号——写 '' 会被解析成"空串 + 重新开引号"，
  // 单引号直接消失。于是 Lua 变成 require(apisix.core) 与 set_header(ctx, X-Trace-Id, ...)
  // → require 收到 nil → "bad argument #1 to 'require' (string expected, got nil)"
  // → **全站所有路由 500**（2026-09-28 实测）。正确写法是 '\'' 。单靠源码断言抓不到它。
  ['E2E-F-13 展开后 lua 仍带单引号（防止 bash 吞掉引号导致全站 500）',
    (() => {
      try {
        const start = "TRACE_ID_PLUGIN='\n";
        const i = src.indexOf(start);
        if (i < 0) return false;
        const body = src.slice(i + start.length);
        // 闭合是「缩进 + }' 」，正文中 JSON 的 } 后只会跟换行/逗号，故首个 "}'" 即闭合。
        const end = body.indexOf("}'");
        if (end < 0) return false;
        // 原样包回单引号：正文里的 '\'' 已是正确的 bash 转义，再包一层反而双重转义。
        const script = "TRACE_ID_PLUGIN='" + body.slice(0, end + 2) + "\n" +
                       'printf %s "$TRACE_ID_PLUGIN"';
        const out = execSync('bash -s', { input: script, encoding: 'utf8' });
        return /require\('apisix\.core'\)/.test(out) &&
               /set_header\(ctx, 'X-Trace-Id', ctx\.var\.request_id\)/.test(out);
      } catch {
        return false;
      }
    })()],
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
