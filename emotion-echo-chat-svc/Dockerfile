# emotion-echo-chat-svc Dockerfile
# HTTP (Gin :8890) + Kafka consumer + Postgres
#
# 依赖 emotion-echo-shared (go.mod 里 replace ../emotion-echo-shared)
# 所以必须把 emotion-echo-shared 一同复制进 builder
#
# Build context 必须是仓库根:
#   docker build -f emotion-echo-chat-svc/Dockerfile .
# 或在 docker-compose.yml 中:
#   build:
#     context: .
#     dockerfile: emotion-echo-chat-svc/Dockerfile

# ============ Stage 1: builder ============
FROM golang:1.26-alpine AS builder

# 国内构建:必须切到 goproxy.cn,否则 proxy.golang.org 在容器内被墙
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

# 复制 shared 依赖模块 (chat-svc go.mod 里 replace ../emotion-echo-shared)
COPY emotion-echo-shared /workspace/emotion-echo-shared

# 复制 chat-svc 源代码
COPY emotion-echo-chat-svc /workspace/emotion-echo-chat-svc

WORKDIR /workspace/emotion-echo-chat-svc

RUN go mod download

# 编译 Linux 静态二进制
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/chat-svc .

# ============ Stage 2: runtime ============
FROM alpine:3.20

ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    HTTP_PORT=8890

# tini / ca-certificates / tzdata / wget
RUN apk add --no-cache tini ca-certificates tzdata wget \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo "${TZ}" > /etc/timezone \
    && apk del tzdata

# 非 root 用户 (UID 65532 = distroless nonroot 惯例)
RUN addgroup -S -g 65532 app \
    && adduser  -S -u 65532 -G app -h /app -s /sbin/nologin app

WORKDIR /app

COPY --from=builder --chown=app:app /out/chat-svc /app/chat-svc
COPY --from=builder --chown=app:app /workspace/emotion-echo-chat-svc/etc/chat-api.yaml /app/etc/chat-api.yaml

USER app:app

EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${HTTP_PORT}/health || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/chat-svc", "-f", "/app/etc/chat-api.yaml"]
