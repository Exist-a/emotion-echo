# 构建阶段
FROM node:20-alpine AS builder

WORKDIR /app

# 复制 package.json 和 package-lock.json
COPY package*.json ./
# Stage 36-D Bug 9: 默认 npmjs 镜像（npmmirror.com 容器内不可达,改为可配置 BuildArg）
# npm ci 在 node 20 上对最新 npm 包 EBADENGINE 报错,加 engine-strict=false 跳过
# Stage 74: 仓库 lockfile 在 Windows 生成,缺 linux-x64-musl 平台 optional 原生依赖
# （npm/cli#4828）,构建期删除 lockfile 重装——官方推荐 workaround
ARG NPM_REGISTRY=https://registry.npmjs.org/
RUN npm config set registry ${NPM_REGISTRY} \
    && npm config set engine-strict false \
    && rm -f package-lock.json \
    && npm install --no-audit --no-fund --prefer-offline

# 复制源代码
COPY . .

# 构建应用
RUN npm run build

# 运行阶段
FROM node:20-alpine

WORKDIR /app

# 从构建阶段复制构建产物
COPY --from=builder /app/.output ./.output

EXPOSE 3000

# 启动生产服务器
CMD ["node", ".output/server/index.mjs"]