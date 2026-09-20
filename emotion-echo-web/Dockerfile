# 构建阶段
# Round D: 基础镜像 digest pin（Dockerfile.digests.lock 提供 NODE_20_ALPINE_DIGEST）
# dev compose 不强制；prod CI 同步真 digest 后 build 锁版本
ARG NODE_20_ALPINE_IMAGE
ARG NODE_20_ALPINE_DIGEST
FROM ${NODE_20_ALPINE_IMAGE:-node:20-alpine}${NODE_20_ALPINE_DIGEST:+@${NODE_20_ALPINE_DIGEST}} AS builder

WORKDIR /app

# 复制 package.json 和 package-lock.json
COPY package*.json ./
# Stage 36-D Bug 9: 默认 npmjs 镜像（npmmirror.com 容器内不可达,改为可配置 BuildArg）
# P0-R2-9: 保留 lockfile + npm ci 保证构建可复现（Stage 74 的 linux-musl 兼容问题
# 通过 --omit=optional 解决，不再删除 lockfile）
# Stage 105 PR-0: 加 --ignore-scripts 跳过 postinstall (nuxt prepare 在 alpine musl 上
# 触发 oxc-parser native binding 缺报错; nuxt build 自带 prepare, 不依赖 postinstall)
ARG NPM_REGISTRY=https://registry.npmjs.org/
RUN npm config set registry ${NPM_REGISTRY} \
    && npm config set engine-strict false \
    && npm install --ignore-scripts --no-audit --no-fund \
    # Stage 105: alpine musl 上 oxc-parser/transform/minify native binding
    # npm bug #4828 偶发跳过 optional deps，显式补装保证 binary 落地。
    # 不钉版本——由 npm install 解析的实际版本决定。
    && npm ls oxc-parser --parseable 2>/dev/null | head -1 \
    && npm install --no-save --ignore-scripts --no-audit --no-fund \
        @oxc-parser/binding-linux-x64-musl \
        @oxc-transform/binding-linux-x64-musl \
        @oxc-minify/binding-linux-x64-musl \
        @rollup/rollup-linux-x64-musl

# 复制源代码
COPY . .

# 构建应用
RUN npm run build

# 运行阶段
# Round D: 基础镜像 digest pin
ARG NODE_20_ALPINE_IMAGE
ARG NODE_20_ALPINE_DIGEST
FROM ${NODE_20_ALPINE_IMAGE:-node:20-alpine}${NODE_20_ALPINE_DIGEST:+@${NODE_20_ALPINE_DIGEST}}

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