# 构建阶段
# Round D: 基础镜像 digest pin（Dockerfile.digests.lock 提供 NODE_22_ALPINE_DIGEST）
# dev compose 不强制；prod CI 同步真 digest 后 build 锁版本
#
# CL-06 Phase 0（账本 CL-F-28，2026-10-17）：**基础镜像由 node:20-alpine 升到 node:22-alpine**。
# 为什么不是"只把 npm 换成 pnpm"就完事 —— 真正的失败链有两段（实测）：
#   ① `npm install` 直接 `ERESOLVE`（package.json 要 vue-router@^5.4.0，nuxt@^4.2.2 要 ^4.6.3）；
#   ② 改用仓库权威锁（pnpm-lock.yaml）后依赖升级为 **nuxt@4.5.2 / nitropack@2.13.4 / vite@8.3.0**，
#      构建期调用 **`Set.prototype.difference`（Node 22+ 才有）** ⇒ node:20 上
#      `pnpm run build` 报 `TypeError: trustedFunctions.difference is not a function`。
#      旧的 npm 路径之所以"看着能构建"（`:clp4` 镜像 2026-10-17 建成功），是因为它用的是
#      **停更在 2026-09-24 的 package-lock.json**（nuxt@4.2.2 / nitropack@2.12.9）—— 能在 Node 20 上跑。
#      ⇒ 两条工具链解析出**不同的依赖版本**，拓扑也就不同；与权威锁对齐就必须同时升 Node。
ARG NODE_22_ALPINE_IMAGE
ARG NODE_22_ALPINE_DIGEST
FROM ${NODE_22_ALPINE_IMAGE:-node:22-alpine}${NODE_22_ALPINE_DIGEST:+@${NODE_22_ALPINE_DIGEST}} AS builder

WORKDIR /app

# 清单 + 权威锁 + registry/hoist 配置
# CL-06 Phase 0（账本 CL-F-28，2026-10-17）：本文件此前走 npm（`npm install`），
#   而仓库的权威工具链是 **pnpm** —— `package.json` 声明 `packageManager: pnpm@9`、
#   `.github/workflows/web-test.yml` 用 `pnpm install --frozen-lockfile`、
#   实时锁是 `pnpm-lock.yaml`（`package-lock.json` 停在 2026-09-24）。
#   两条工具链的解析结果不一致 ⇒ `npm install` 直接 `ERESOLVE` 退出
#   （根项目要 vue-router@^5.4.0，nuxt@^4.2.1 要 ^4.6.3）⇒ **谁都构建不出 web 镜像**。
#   现与 CI 同源：pnpm + `--frozen-lockfile`（可复现），并复制 `.npmrc`
#   （`shamefully-hoist=true` / `node-linker=hoisted`）保证容器内 node_modules 布局与 CI 一致。
COPY package.json pnpm-lock.yaml .npmrc ./

# Stage 36-D Bug 9: 默认 npmjs 镜像（npmmirror.com 容器内不可达，改为可配置 BuildArg）
# Stage 105 PR-0: --ignore-scripts 跳过 postinstall（nuxt prepare 在 alpine musl 上会触发
#   oxc-parser native binding 缺报错；nuxt build 自带 prepare，不依赖 postinstall）
ARG NPM_REGISTRY=https://registry.npmjs.org/
# pnpm 版本：与 package.json `packageManager` 声明的 **9.x** 一致（9.15.1 = 本仓开发机、也是生成本锁文件的版本）。
# ⚠️ 这里用 npm **安装包管理器本身**（`npm i -g pnpm`）—— 这与"**用 npm 装项目依赖**"是两件事，
#    后者才是 CL-F-28 的根因（npm 侧的解析结果与权威锁 pnpm-lock.yaml 不一致 ⇒ ERESOLVE）。
# ⚠️ 不用 corepack：它要求 `packageManager` 是**完整 semver**，而本仓写的是区间 `pnpm@9` ⇒
#    实测直接报 `Invalid package manager specification in package.json (pnpm@9); expected a semver version`
#    （2026-10-17，连显式 `corepack prepare pnpm@9.15.1` 也一样被拒）。修该字段要动 package.json
#    （与 CI `pnpm/action-setup@v4` 的 `version: 9` 存在冲突风险），已登账待裁，不在本 Phase 内擅自改。
ARG PNPM_VERSION=9.15.1
RUN npm i -g pnpm@${PNPM_VERSION} --registry=${NPM_REGISTRY} --no-audit --no-fund \
    && pnpm install --frozen-lockfile --ignore-scripts --registry=${NPM_REGISTRY}

# 复制源代码
COPY . .

# 构建应用
RUN pnpm run build

# 运行阶段
# Round D: 基础镜像 digest pin（与 builder 同版本：Nitro 产物同样面向 Node 22）
ARG NODE_22_ALPINE_IMAGE
ARG NODE_22_ALPINE_DIGEST
FROM ${NODE_22_ALPINE_IMAGE:-node:22-alpine}${NODE_22_ALPINE_DIGEST:+@${NODE_22_ALPINE_DIGEST}}

# P0-R2-8: 非 root 用户运行 Node.js（容器安全）
USER node

WORKDIR /app

# 从构建阶段复制构建产物
COPY --from=builder /app/.output ./.output

EXPOSE 3000

# P2-R2-16: 加 HEALTHCHECK — 之前 web 容器无探活，APISIX seed /
# depends_on 无法判定 web 是否真正就绪（node 启动慢但端口已 listen）。
# wget 是 alpine 自带，不需要装 curl。
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD wget -q --tries=1 -O /dev/null http://localhost:3000/ || exit 1

# 启动生产服务器
CMD ["node", ".output/server/index.mjs"]
