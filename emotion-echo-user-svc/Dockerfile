# emotion-echo-user-svc Dockerfile
# HTTP (Gin :8888) + Postgres
#
# Build context 必须是仓库根

# ============ Stage 1: builder ============
FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

COPY emotion-echo-shared /workspace/emotion-echo-shared
COPY emotion-echo-user-svc /workspace/emotion-echo-user-svc

WORKDIR /workspace/emotion-echo-user-svc

RUN go mod download

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/user-svc .

# ============ Stage 2: runtime ============
FROM alpine:3.20

ENV TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    HTTP_PORT=8888

RUN apk add --no-cache tini ca-certificates tzdata wget \
    && cp /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo "${TZ}" > /etc/timezone \
    && apk del tzdata

RUN addgroup -S -g 65532 app \
    && adduser  -S -u 65532 -G app -h /app -s /sbin/nologin app

WORKDIR /app

COPY --from=builder --chown=app:app /out/user-svc /app/user-svc
COPY --from=builder --chown=app:app /workspace/emotion-echo-user-svc/etc/user-api.yaml /app/etc/user-api.yaml

USER app:app

EXPOSE 8888

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${HTTP_PORT}/health || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/user-svc", "-f", "/app/etc/user-api.yaml"]
