#!/usr/bin/env bash
# Stage 32 PR-15: APISIX 路由 seed 脚本
#
# 用途：APISIX 启动后向 Admin API 注入 upstream + route + 全局插件链
# 前置：etcd + apisix + 业务 svc 均已 docker-compose up
# 用法：
#   ./deploy/apisix/seed.sh                 # 默认 localhost:9180
#   APISIX_ADMIN_URL=http://apisix:9180 ./deploy/apisix/seed.sh
#   APISIX_ADMIN_KEY=<secret> ./deploy/apisix/seed.sh
#
# 设计取舍（vs 文档 stage-32-apisix-reintroduction.md §三.3）：
#   文档期望"7 upstream + 16 路由"是 Stage 30 退役前的 1:1 直连形态。
#   Stage 31 引入 web-bff 聚合层后，APISIX 入口应统一为 catch-all 到 web-bff
#   （否则 BFF 的 SSE 流式编排 / 多服务聚合 / 字段裁剪失效）。
#   本脚本落地的 6 路由 + 1 upstream（web-bff）+ 5 健康探针 upstream（直连下游），
#   比文档原计划更精简但符合架构事实。文档已在 PR-15 落地后追加"路由收敛说明"。
#
# 退出码：
#   0 - 全部成功
#   1 - APISIX 不可达
#   2 - 上游 svc 不可达（任一 health check 失败）
#   3 - route/upstream PUT 失败

# 2026-09-04：用 `set -eu` 而非 `set -euo pipefail`。
# 本脚本既在宿主 bash 跑，也由 compose 的 apisix-seed 容器（curlimages/curl，
# BusyBox ash）以 sh 执行，而 ash 不支持 `-o pipefail`。
# 脚本内仅有的 3 处管道都是 `... 2>&1 | head -c 300` 的错误信息截断，
# 退出码不参与判断，去掉 pipefail 不影响错误检测。
set -eu

# ---- Sprint 1 PR-3: services.env 单点真理 ----
# 优先级：用户传入 env > $(dirname "$0")/services.env > $(dirname "$0")/services.env.example
# services.env (git ignore) 用于 prod 覆盖；services.env.example 是 dev 默认。
_load_services() {
  local env_file
  env_file="$(dirname "$0")/services.env"
  if [ -f "$env_file" ]; then
    # shellcheck disable=SC1090
    . "$env_file"
    log "loaded services.env (prod override)"
  fi
  env_file="$(dirname "$0")/services.env.example"
  if [ -f "$env_file" ]; then
    # shellcheck disable=SC1090
    . "$env_file"
    log "loaded services.env.example (dev defaults)"
  fi
}
_load_services

# ---- 配置 ----
ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-WhZEPlrGviCSXlKFfALZlQWinluoGAbj}"
JWT_SECRET="${BFF_JWT_SECRET:-dev-bff-secret}"
# 前端来源（cors allow_origins）。dev 是 Nuxt dev server；prod 由 env 覆盖。
CORS_ALLOW_ORIGINS="${CORS_ALLOW_ORIGINS:-http://localhost:3000}"

# 业务 svc 容器名（compose 网络 DNS）。默认值由 services.env.example 提供，
# 此处仅保留 ${VAR:-default} 兜底（脚本被独立调用时仍能跑）。
USER_SVC_HOST="${USER_SVC_HOST:-emotion-echo-user-svc}"
CHAT_SVC_HOST="${CHAT_SVC_HOST:-emotion-echo-chat-svc}"
ASSESSMENT_SVC_HOST="${ASSESSMENT_SVC_HOST:-emotion-echo-assessment-svc}"
ANALYTICS_SVC_HOST="${ANALYTICS_SVC_HOST:-emotion-echo-analytics-svc}"
AI_SVC_HOST="${AI_SVC_HOST:-emotion-echo-ai-svc}"
WEB_BFF_HOST="${WEB_BFF_HOST:-emotion-echo-web-bff}"

# 端口
USER_SVC_PORT="${USER_SVC_PORT:-8888}"
CHAT_SVC_PORT="${CHAT_SVC_PORT:-8890}"
ASSESSMENT_SVC_PORT="${ASSESSMENT_SVC_PORT:-8889}"
ANALYTICS_SVC_PORT="${ANALYTICS_SVC_PORT:-8893}"
AI_SVC_PORT="${AI_SVC_PORT:-8891}"
WEB_BFF_PORT="${WEB_BFF_PORT:-8894}"

# ---- 工具 ----
log()  { echo "[seed] $*" >&2; }
die()  { echo "[seed] FATAL: $*" >&2; exit "${2:-1}"; }

# ---- Step 1: 前置 health check ----
log "Step 1/4: waiting for APISIX admin API at $ADMIN_URL"
for i in $(seq 1 30); do
  if curl -sf -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/routes" >/dev/null 2>&1; then
    log "  APISIX admin API reachable"
    break
  fi
  if [ "$i" -eq 30 ]; then
    die "APISIX admin API not reachable at $ADMIN_URL after 30s"
  fi
  sleep 1
done

log "Step 1.5/4: verifying upstream svcs are up (catch-all depends on web-bff)"
# 2026-09-04 修正：原实现从**宿主机**直接 curl 容器 DNS 名（emotion-echo-user-svc:8888），
# 宿主机解析不了 compose 网络内的服务名，于是 5 个探测必然全失败、seed 在此中止——
# 这就是 PR-3 之后"seed 从没端到端跑过"的直接原因。
# 且除 web-bff 外的 svc 自 Stage 33 PR-20 起就不再对宿主暴露 HTTP 端口，
# 所以换成 localhost:<port> 也不成立。正确做法是**在 compose 网络内**探测。
#
# DOCKER_NETWORK 留空则退化为宿主机直连（兼容非 compose 部署）。
DOCKER_NETWORK="${DOCKER_NETWORK:-emotion-echo_app-network}"
PROBE_IMAGE="${PROBE_IMAGE:-curlimages/curl:latest}"

probe() {
  local url="$1"
  if [ -n "$DOCKER_NETWORK" ] && command -v docker >/dev/null 2>&1; then
    docker run --rm --network "$DOCKER_NETWORK" "$PROBE_IMAGE" \
      -sf --max-time 3 "$url" >/dev/null 2>&1
  else
    curl -sf --max-time 3 "$url" >/dev/null 2>&1
  fi
}

if [ "${SKIP_HEALTH_CHECK:-false}" = "true" ]; then
  log "  SKIP_HEALTH_CHECK=true: skipping upstream health probe (dev validation only)"
else
  log "  probing from inside docker network '$DOCKER_NETWORK'"
  for hp in \
    "$WEB_BFF_HOST:$WEB_BFF_PORT/health" \
    "$USER_SVC_HOST:$USER_SVC_PORT/health" \
    "$CHAT_SVC_HOST:$CHAT_SVC_PORT/health" \
    "$ASSESSMENT_SVC_HOST:$ASSESSMENT_SVC_PORT/health" \
    "$AI_SVC_HOST:$AI_SVC_PORT/health"; do
    if ! probe "http://$hp"; then
      die "upstream $hp not healthy (seed will silently skip if continued, aborting per AGENTS.md RED→GREEN)" 2
    fi
    log "  upstream OK: $hp"
  done
fi

# ---- Step 2: 创建 upstream（静态节点，Stage 34+ 切 nacos-discovery）----
log "Step 2/4: creating upstreams"

put_upstream() {
  local id="$1" name="$2" host="$3" port="$4"
  local body
  body=$(cat <<EOF
{
  "name": "$name",
  "type": "roundrobin",
  "nodes": [
    {"host": "$host", "port": $port, "weight": 1}
  ]
}
EOF
)
  if curl -sf -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$ADMIN_URL/apisix/admin/upstreams/$id" >/dev/null; then
    log "  upstream OK: $name ($host:$port)"
  else
    die "failed to PUT upstream $name" 3
  fi
}

# PR-3: Nacos discovery upstream（注册到 Nacos 的 svc 走这条路径）
# 不再写死 host:port；APISIX nacos discovery 自动从 Nacos 拉实例。
#
# 2026-09-04 修正：namespace_id / group_name 必须嵌在 discovery_args 内。
# 原先写成 upstream 顶层字段，APISIX 直接拒绝：
#   {"error_msg":"invalid configuration: additional properties forbidden, found namespace_id"}
# 依据 https://apisix.apache.org/docs/apisix/discovery/nacos/ ——二者是 per-service
# 参数（属 discovery_args）；Nacos 连接信息才在 config.yaml 顶层 discovery.nacos 段。
put_nacos_upstream() {
  local id="$1" name="$2" service_name="$3"
  local body
  body=$(cat <<EOF
{
  "name": "$name",
  "type": "roundrobin",
  "discovery_type": "nacos",
  "service_name": "$service_name",
  "discovery_args": {
    "namespace_id": "$NACOS_NAMESPACE",
    "group_name": "$NACOS_GROUP"
  }
}
EOF
)
  if curl -sf -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$ADMIN_URL/apisix/admin/upstreams/$id" >/dev/null; then
    log "  nacos upstream OK: $name (svc=$service_name)"
  else
    # 打印 APISIX 的真实拒绝原因，而不是只报"失败"——原版把 schema 报错吞了，
    # 导致 PR-3 从没人知道具体错在哪个字段。
    local err
    err=$(curl -s -X PUT \
      -H "X-API-KEY: $ADMIN_KEY" \
      -H "Content-Type: application/json" \
      -d "$body" \
      "$ADMIN_URL/apisix/admin/upstreams/$id" 2>&1 | head -c 300)
    die "failed to PUT nacos upstream $name: $err" 3
  fi
}

# Nacos 配置（与业务 svc env 对齐）
NACOS_NAMESPACE="${NACOS_NAMESPACE:-emotion-echo-dev}"
NACOS_GROUP="${NACOS_GROUP:-DEFAULT_GROUP}"

# PR-3: 全部 6 个 upstream 切 nacos-discovery（与 BootNacos 注册的 serviceName 一致）
# 之前的静态 put_upstream 调用由本块替换；保留 put_upstream 函数作 Nacos 不可达时的兜底。
put_nacos_upstream 1  user-svc       "emotion-echo-user-svc"
put_nacos_upstream 2  chat-svc       "emotion-echo-chat-svc"
put_nacos_upstream 3  assessment-svc "emotion-echo-assessment-svc"
put_nacos_upstream 4  analytics-svc  "emotion-echo-analytics-svc"
put_nacos_upstream 5  ai-svc         "emotion-echo-ai-svc"
put_nacos_upstream 6  web-bff        "emotion-echo-web-bff"

# ---- Step 2.5: jwt-auth consumer ----
# 2026-09-04 新增：原 seed.sh **完全没有创建 consumer**，导致所有挂 jwt-auth 的
# 路由必然 401——jwt-auth 是靠 JWT 里的 key claim 去匹配 consumer，
# 再用该 consumer 的 secret 验签。没有 consumer 就没有 secret 来源。
#
# 依据 https://apisix.apache.org/docs/apisix/plugins/jwt-auth/ ：
#   key / secret / algorithm 属 **consumer**（凭据侧）
#   route 侧的 jwt-auth 只放传输层参数（header/query/cookie 等），不放 secret
#
# JWT_KEY 必须与 BFF 签发的 token 里的 key claim 一致
# （BFF auth_handler 签的是 key="user"，见 internal/auth）。
JWT_KEY="${BFF_JWT_KEY:-user}"

log "Step 2.5/4: creating jwt-auth consumer (key=$JWT_KEY)"
CONSUMER_BODY=$(cat <<EOF
{
  "username": "emotion_echo_bff",
  "desc": "BFF 签发的 JWT 由本 consumer 的 secret 验签",
  "plugins": {
    "jwt-auth": {
      "key": "$JWT_KEY",
      "secret": "$JWT_SECRET",
      "algorithm": "HS256"
    }
  }
}
EOF
)
if curl -sf -X PUT \
  -H "X-API-KEY: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d "$CONSUMER_BODY" \
  "$ADMIN_URL/apisix/admin/consumers/emotion_echo_bff" >/dev/null; then
  log "  consumer OK: emotion_echo_bff (jwt key=$JWT_KEY)"
else
  err=$(curl -s -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$CONSUMER_BODY" \
    "$ADMIN_URL/apisix/admin/consumers/emotion_echo_bff" 2>&1 | head -c 300)
  die "failed to PUT jwt-auth consumer: $err" 3
fi

# ---- Step 3: 全局插件链（每个 route 共享）----
log "Step 3/4: defining shared plugins"

# PR-OBS-1 REFACTOR: skywalking-logger + file-logger 在 PLUGINS_JSON 与
# CATCHALL_PLUGINS_JSON 完全相同,抽出 OBSERVABILITY_PLUGINS_JSON 共享变量。
# 保持 bash-only(不引入 python/jq),与原 seed 设计一致。

# skywalking-logger: 每个请求把 APISIX access 信息上报 OAP
#   与 config.yaml endpoint_addr=http://emotion-echo-sw-oap:12800 配套
# file-logger: 落盘到 /tmp/apisix-access.log(由 PR-OBS-5 volume mount),
#   由 promtail 采集送 Loki
OBSERVABILITY_PLUGINS_JSON='
  "skywalking-logger": {
    "endpoint_addr": "http://emotion-echo-sw-oap:12800",
    "service_name": "APISIX",
    "report_interval": 3
  },
  "file-logger": {
    "path": "/tmp/apisix-access.log",
    "log_format": {
      "client_ip": "$remote_addr",
      "user": "$remote_user",
      "timestamp": "$time_iso8601",
      "method": "$request_method",
      "url": "$request_uri",
      "status": "$status",
      "bytes_sent": "$bytes_sent",
      "bytes_received": "$bytes_received",
      "resp_time": "$request_time",
      "upstream": "$upstream_addr",
      "upstream_time": "$upstream_response_time"
    }
  }'

# jwt-auth 真正验签（替换 shared jwt_auth.go 的"信任 APISIX"模型）
#   2026-09-04：route 侧只留 {}——key/secret/algorithm 属 consumer（Step 2.5），
#   放在 route 上不会生效。见 https://apisix.apache.org/docs/apisix/plugins/jwt-auth/
# limit-count / limit-req 限流（双层：按 IP 计数 + 全局突发）
# api-breaker 下游 5xx > 50% 熔断 30s
# cors 统一 CORS（替代 BFF corsMiddleware）
# prometheus 默认配置（OAP 上报 metrics）
# skywalking-logger + file-logger 引用 OBSERVABILITY_PLUGINS_JSON (PR-OBS-1 REFACTOR)
PLUGINS_JSON=$(cat <<EOF
{
  "jwt-auth": {},
  "limit-count": {
    "count": 60,
    "time_window": 60,
    "key": "remote_addr",
    "policy": "local",
    "rejected_code": 429
  },
  "limit-req": {
    "rate": 1000,
    "burst": 100,
    "key": "remote_addr",
    "policy": "local",
    "rejected_code": 503
  },
  "api-breaker": {
    "break_response_code": 503,
    "min_requests": 20,
    "error_threshold_ratio": 0.5,
    "open_time": 30
  },
  "cors": {
    "allow_origins": "$CORS_ALLOW_ORIGINS",
    "allow_methods": "GET,POST,PUT,DELETE,OPTIONS,PATCH",
    "allow_headers": "Content-Type,Authorization,X-User-Id",
    "expose_headers": "X-User-Id",
    "allow_credentials": true,
    "max_age": 600
  },
${OBSERVABILITY_PLUGINS_JSON},
  "prometheus": {}
}
EOF
)

# 2026-09-04 新增：把已验签 JWT 的 sub claim 注入 X-User-Id header。
#
# Stage 32 §3.3 方案 A 的链路是
#   APISIX jwt-auth 验签 → **APISIX 注入 X-User-Id** → BFF/下游信任该 header
# 但 seed.sh 从来只在 cors 里把 X-User-Id 列进 allow/expose 名单，
# **从没真正注入过**。结果：jwt-auth 通过后 BFF 仍返
#   {"error":"unauthorized: missing or invalid X-User-Id"}
# 即该鉴权链路的后半截一直是断的。
#
# 实现方式的取舍（都实测过）：
#   ✗ proxy-rewrite + "$jwt_payload_sub" —— APISIX **没有**这个 nginx 变量，
#     结果 header 被设成空串，比不注入更糟（把客户端传的值也覆盖掉了）
#   ✗ X-Consumer-Username —— APISIX 验签后确实会自动加，但值是 consumer 名
#     （emotion_echo_bff），不是数字 user_id，shared middleware 的
#     parseXUserID 只接受纯数字，必然 401
#   ✓ jwt-auth.store_in_ctx=true 把 payload 放进 ctx.jwt_auth_payload，
#     再用 serverless-post-function 读 sub 写 header
#
# 用 post-function 而非 pre-function：config.yaml 里 serverless-pre-function
# 排在 jwt-auth **之前**（第 29 行 vs 第 49 行），那时还没验签、拿不到 payload；
# serverless-post-function 在第 122 行，位于 jwt-auth 之后。
#
# 覆盖式赋值：无条件覆盖客户端自带的 X-User-Id，避免外部伪造身份。
#
# 写成字面量而非用 python 改 PLUGINS_JSON：seed 要能在最小 seed 容器里跑
# （bash + curl 即可），不引入 python 依赖。与 PLUGINS_JSON 的重复部分有限，
# 换来的是零额外运行时依赖。
CATCHALL_PLUGINS_JSON=$(cat <<EOF
{
  "jwt-auth": { "store_in_ctx": true },
  "serverless-post-function": {
    "phase": "rewrite",
    "functions": [
      "return function(conf, ctx) local core = require('apisix.core'); local p = ctx.jwt_auth_payload; local uid = p and (p.sub or p.user_id); core.request.set_header(ctx, 'X-User-Id', uid ~= nil and tostring(uid) or '') end"
    ]
  },
  "limit-count": {
    "count": 60,
    "time_window": 60,
    "key": "remote_addr",
    "policy": "local",
    "rejected_code": 429
  },
  "limit-req": {
    "rate": 1000,
    "burst": 100,
    "key": "remote_addr",
    "policy": "local",
    "rejected_code": 503
  },
  "api-breaker": {
    "break_response_code": 503,
    "min_requests": 20,
    "error_threshold_ratio": 0.5,
    "open_time": 30
  },
  "cors": {
    "allow_origins": "$CORS_ALLOW_ORIGINS",
    "allow_methods": "GET,POST,PUT,DELETE,OPTIONS,PATCH",
    "allow_headers": "Content-Type,Authorization,X-User-Id",
    "expose_headers": "X-User-Id",
    "allow_credentials": true,
    "max_age": 600
  },
${OBSERVABILITY_PLUGINS_JSON},
  "prometheus": {}
}
EOF
)

# ---- Step 4: 创建 route（catch-all 主入口 + 5 健康探针）----
log "Step 4/4: creating routes"

put_route() {
  local id="$1" uri="$2" upstream_id="$3" methods="$4" extra_uri="${5:-}"
  local body
  # 用 CATCHALL_PLUGINS_JSON（= PLUGINS_JSON + proxy-rewrite 注入 X-User-Id）；
  # 仅 catch-all 走鉴权链路，故 X-User-Id 注入只需挂这里。
  body=$(cat <<EOF
{
  "uri": "$uri$extra_uri",
  "methods": $methods,
  "upstream_id": $upstream_id,
  "plugins": $CATCHALL_PLUGINS_JSON,
  "status": 1
}
EOF
)
  if curl -sf -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$ADMIN_URL/apisix/admin/routes/$id" >/dev/null; then
    log "  route OK: $id → upstream $upstream_id (uri=$uri)"
  else
    err=$(curl -s -X PUT \
      -H "X-API-KEY: $ADMIN_KEY" \
      -H "Content-Type: application/json" \
      -d "$body" \
      "$ADMIN_URL/apisix/admin/routes/$id" 2>&1 | head -c 300)
    die "failed to PUT route $id: $err" 3
  fi
}

# 健康探针（不挂鉴权/限流，monitoring 用）—— 直接放 health 端点
#
# Stage 62 PR-3.4 修复：探针路由原样转发 /<svc>-health 到下游，
# 但各 svc 的 gin_auth 中间件只豁免 /health → 实测全部返
# {"error":"unauthorized: missing or invalid X-User-Id"} 401。
# 加 proxy-rewrite 把 /<svc>-health 重写为 /health。
HEALTH_PLUGINS=$(cat <<'EOF'
{
  "prometheus": {},
  "proxy-rewrite": {
    "uri": "/health"
  }
}
EOF
)
put_route_health() {
  local id="$1" uri="$2" upstream_id="$3"
  local body
  body=$(cat <<EOF
{
  "uri": "$uri",
  "upstream_id": $upstream_id,
  "plugins": $HEALTH_PLUGINS,
  "status": 1
}
EOF
)
  if curl -sf -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$ADMIN_URL/apisix/admin/routes/$id" >/dev/null; then
    log "  health route OK: $id (uri=$uri)"
  else
    die "failed to PUT health route $id" 3
  fi
}

# 主入口：所有 /api/v1/* 走 web-bff（聚合层）
# 注意：顺序很重要—— APISIX 路由按 ID 升序匹配，catch-all 放最后
# 但 APISIX 支持 longest-prefix 优先，所以顺序无严格要求。
put_route 100 "/api/v1/*" 6 '["GET","POST","PUT","DELETE","PATCH"]'

# ---- Stage 33 PR-19b：/api/v1/auth/* 白名单（跳过 jwt-auth 插件）----
# login/register/verification-code/refresh 端点拿不到 token，不能被 jwt-auth 拦截。
# 用裸 plugins（仅保留 limit-count 全局限流 + cors，不挂 jwt-auth）。
# Stage 38-A 修正：APISIX cors 插件的 allow_origins/allow_methods/allow_headers
# 期望**字符串**（逗号分隔），不是 JSON 数组。原 heredoc 用数组导致 PUT 校验失败。
AUTH_WHITELIST_PLUGINS=$(cat <<EOF
{
  "limit-count": {"count": 60, "time_window": 60, "key": "remote_addr", "policy": "local"},
  "cors": {"allow_origins": "$CORS_ALLOW_ORIGINS", "allow_methods": "GET,POST,PUT,DELETE,OPTIONS", "allow_credentials": true, "allow_headers": "*"}
}
EOF
)
put_auth_route() {
  local id="$1" uri="$2"
  local body
  body=$(cat <<EOF
{
  "uri": "$uri",
  "upstream_id": 6,
  "methods": ["GET","POST","PUT","DELETE","PATCH","OPTIONS"],
  "plugins": $AUTH_WHITELIST_PLUGINS,
  "status": 1
}
EOF
)
  if curl -sf -X PUT \
    -H "X-API-KEY: $ADMIN_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    "$ADMIN_URL/apisix/admin/routes/$id" >/dev/null; then
    log "  auth whitelist route OK: $id (uri=$uri, no jwt-auth)"
  else
    die "failed to PUT auth whitelist route $id" 3
  fi
}

# longest-prefix 优先匹配 → 在 /api/v1/* (id 100) 之前注册也 OK，但放后面便于管理
put_auth_route 110 "/api/v1/auth/login"
put_auth_route 111 "/api/v1/auth/register"
put_auth_route 112 "/api/v1/auth/verification-code"
put_auth_route 113 "/api/v1/auth/refresh"
put_auth_route 114 "/api/v1/auth/logout"
# Sprint 1 PR-4c-4: reset-password 同样无登录态可调用，纳入白名单（fix bug #2）
put_auth_route 115 "/api/v1/auth/reset-password"

# 健康探针（直接打到下游 svc，绕开 BFF 聚合）
put_route_health 200 "/user-health"          1
put_route_health 201 "/chat-health"          2
put_route_health 202 "/assessment-health"    3
put_route_health 203 "/analytics-health"     4
put_route_health 204 "/ai-health"            5

# APISIX gateway 自身健康（不需要 upstream）
if curl -sf -X PUT \
  -H "X-API-KEY: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"uri":"/apisix-health","status":1,"plugins":{"prometheus":{}}}' \
  "$ADMIN_URL/apisix/admin/routes/205" >/dev/null; then
  log "  apisix self-health route OK: 205"
else
  die "failed to PUT apisix self-health route" 3
fi

log "seed complete: 6 upstreams + 12 routes (1 catch-all + 5 health + 5 auth-whitelist + 1 self-health)"
