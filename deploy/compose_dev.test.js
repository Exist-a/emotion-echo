// Stage 105 PR-0: dev 模式架构调研结论文档 (Node.js, 无依赖)
// ==============================
// 目标: 让 dev 模式下修改前端 .vue 即时在浏览器看到 (不需 docker compose rebuild).
// 调研: 尝试用 Dockerfile.dev (nuxt dev HMR) + 宿主机源码 bind mount.
// 失败原因:
//   1. alpine 镜像用 musl libc, oxc-parser 的 native binding 无 linux-x64-musl 预编译 → 启动崩
//   2. 换 Debian slim (glibc, linux-x64-gnu), apt-get 装基础工具网络不稳, npm install 749 包后
//      @oxc-parser/binding-linux-x64-gnu 未安装 (npm optionalDependencies 已知 bug, 见
//      https://github.com/npm/cli/issues/4828), nuxt dev 仍崩 "Cannot find native binding"
// 结论: dev 模式继续用 Dockerfile (生产 build), 改前端代码需手动 `docker compose build
//   emotion-echo-web` 重 build image. 实操 5 分钟左右, 比 Stage 103 时少 1/2 (npm ci 偶发
//   ECONNRESET 已避坑, 用 npmmirror).
// 下一步 (Stage 106+): 调研 host npm dev (本机 node 24 已装, 项目要求 ^20.19.0 || >=22.12.0,
//   v24 满足 ≥22), 完全跳过容器化 dev, 真正实现 HMR.
//
// 本文件保留作为调研记录 + Stage 105 PR-0 状态契约: 当前架构保持生产 build.
//
// 运行: node deploy/compose_dev.test.js

const fs = require('fs');
const path = require('path');

const SCRIPT_DIR = __dirname;
const COMPOSE_APPS = path.join(SCRIPT_DIR, 'docker-compose.apps.yml');

function fail(msg) { console.error('  ✗ ' + msg); process.exitCode = 1; }
function pass(msg) { console.log('  ✓ ' + msg); }

if (!fs.existsSync(COMPOSE_APPS)) { console.error('docker-compose.apps.yml not found'); process.exit(1); }
pass('docker-compose.apps.yml exists');

const yml = fs.readFileSync(COMPOSE_APPS, 'utf8');

const checks = [
  // Stage 105 PR-0: dev 模式 web 服务仍用 Dockerfile (生产 build).
  // 等 Stage 106 host-npm-dev 落地后才能切 Dockerfile.dev.
  ['dev compose web 服务 dockerfile 必须为 Dockerfile (生产 build, 避免 alpine musl 崩 oxc-parser)',
    /emotion-echo-web:[\s\S]*?build:[\s\S]*?dockerfile:\s*Dockerfile(?!\.dev)/.test(yml)],

  // REGRARD-GUARD: 防有人未来改 Dockerfile.dev 但 alpine musl 崩问题未解.
  ['REGRARD-GUARD: dev 模式不能改用 Dockerfile.dev (alpine musl 崩 oxc-parser)',
    !/emotion-echo-web:[\s\S]*?build:[\s\S]*?dockerfile:\s*Dockerfile\.dev/.test(yml)],
];

let passCount = 0, failCount = 0;
for (const [name, ok] of checks) {
  if (ok) { pass(name); passCount++; }
  else { fail(name); failCount++; }
}

console.log('');
console.log('Summary: PASS=' + passCount + ' FAIL=' + failCount);
if (failCount > 0) process.exit(1);