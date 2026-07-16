# emotion-echo-ai-svc Dockerfile
# 同时提供 HTTP (Gin :8891) + gRPC (Stage 19 :8892) 两套协议
#
# 注意：ai-svc 依赖 emotion-echo-shared（go.mod 里 replace ../emotion-echo-shared）
# 所以必须把 emotion-echo-shared 一同复制进 builder
#
# Build context 必须是仓库根（emotion-echo-ai-svc 的父目录）：
#   docker build -f emotion-echo-ai-svc/Dockerfile .
# 或在 docker-compose.yml 中：
#   build:
#     context: .
#     dockerfile: emotion-echo-ai-svc/Dockerfile
#
# 阶段 1: builder  - 编译 Linux 静态二进制
# 阶段 2: runtime  - alpine + tini + 二进制（非 root 运行）

# ============ Stage 1: builder ============
FROM golang:1.26-alpine AS builder

# 国内构建：必须切到 goproxy.cn，否则 proxy.golang.org 在容器内被墙
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

# 复制 shared 依赖模块（ai-svc go.mod 里 replace ../emotion-echo-shared）
# build context 必须是仓库根
COPY emotion-echo-shared /workspace/emotion-echo-shared

# 复制 ai-svc 源代码（注意：不能用 `COPY .`，会把整个 build context 复制进来，
# 反而把 emotion-echo-ai-svc 套到 /workspace/emotion-echo-ai-svc/emotion-echo-ai-svc/... 路径错位）
COPY emotion-echo-ai-svc /workspace/emotion-echo-ai-svc

# 复制 TLS 证书（Stage 18 mTLS，builder → runtime 透传）
COPY deploy /workspace/deploy

# 切换到 ai-svc 目录编译
WORKDIR /workspace/emotion-echo-ai-svc

# 下载依赖 + tidy（replace 指向 ../emotion-echo-shared 已就绪）
RUN go mod download
# 可选：RUN go mod tidy

# 编译 Linux 静态二进制（无 CGO，体积小，alpine 兼容）
# -trimpath: 去掉绝对路径，镜像可复现
# -ldflags="-s -w": 去掉符号表和调试信息，体积更小
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ai-svc .

# ============ Stage 2: runtime ============
FROM alpine:3.20

# 容器内时区
ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    # gin 端口 (HTTP)
    HTTP_PORT=8891 \
    # gRPC 端口 (Stage 19)
    GRPC_PORT=8892

# tini: PID 1 + 信号转发（用 alpine 官方包，musl 兼容）
# ca-certificates: 让 Go 访问 HTTPS 资源（OAP/Kafka 等）
# tzdata: 时区数据
# wget: 健康检查备用
RUN apk add --no-cache tini ca-certificates tzdata wget \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo "${TZ}" > /etc/timezone \
    && apk del tzdata

# 非 root 用户（UID 65532 = distroless nonroot 惯例）
RUN addgroup -S -g 65532 app \
    && adduser  -S -u 65532 -G app -h /app -s /sbin/nologin app

WORKDIR /app

# 复制编译产物 + 配置文件
COPY --from=builder --chown=app:app /out/ai-svc         /app/ai-svc
COPY --from=builder --chown=app:app /workspace/emotion-echo-ai-svc/etc/ai-api.yaml /app/etc/ai-api.yaml
# 同时把 TLS 证书也带上（Stage 18 mTLS 用，路径以 etc/tls/* 为基准）
COPY --from=builder --chown=app:app /workspace/deploy/tls/ca.crt         /app/etc/tls/ca.crt
COPY --from=builder --chown=app:app /workspace/deploy/tls/ai-client.crt  /app/etc/tls/ai-client.crt
COPY --from=builder --chown=app:app /workspace/deploy/tls/ai-client.key  /app/etc/tls/ai-client.key

# 切到非 root
USER app:app

# 暴露 HTTP + gRPC 双端口
EXPOSE 8891 8892

# 健康检查（HTTP 探活）
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${HTTP_PORT}/health || exit 1

# tini 作为 PID 1 转发 SIGTERM
ENTRYPOINT ["/sbin/tini", "--", "/app/ai-svc", "-f", "/app/etc/ai-api.yaml"]
