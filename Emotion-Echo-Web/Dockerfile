# 构建阶段
FROM node:20-alpine AS builder

WORKDIR /app

# 复制 package.json 和 package-lock.json
COPY package*.json ./
# 国内网络：用 npmmirror 镜像加速 npm ci（默认 npmjs.org 在容器内被墙/超时）
RUN npm config set registry https://registry.npmmirror.com \
    && npm ci

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