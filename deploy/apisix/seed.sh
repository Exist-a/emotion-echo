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

set -euo pipefail

# ---- 配置 ----
ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-WhZEPlrGviCSXlKFfALZlQWinluoGAbj}"
JWT_SECRET="${BFF_JWT_SECRET:-dev-bff-secret}"

# 业务 svc 容器名（compose 网络 DNS）
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
if [ "${SKIP_HEALTH_CHECK:-false}" = "true" ]; then
  log "  SKIP_HEALTH_CHECK=true: skipping upstream health probe (dev validation only)"
else
  for hp in \
    "$WEB_BFF_HOST:$WEB_BFF_PORT/health" \
    "$USER_SVC_HOST:$USER_SVC_PORT/health" \
    "$CHAT_SVC_HOST:$CHAT_SVC_PORT/health" \
    "$ASSESSMENT_SVC_HOST:$ASSESSMENT_SVC_PORT/health" \
    "$AI_SVC_HOST:$AI_SVC_PORT/health"; do
    if ! curl -sf --max-time 3 "http://$hp" >/dev/null 2>&1; then
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

# ID 分配：与原根目录 apisix-*-up.json 残存文件兼容
put_upstream 1  user-svc       "$USER_SVC_HOST"       "$USER_SVC_PORT"
put_upstream 2  chat-svc       "$CHAT_SVC_HOST"       "$CHAT_SVC_PORT"
put_upstream 3  assessment-svc "$ASSESSMENT_SVC_HOST" "$ASSESSMENT_SVC_PORT"
put_upstream 4  analytics-svc  "$ANALYTICS_SVC_HOST"  "$ANALYTICS_SVC_PORT"
put_upstream 5  ai-svc         "$AI_SVC_HOST"         "$AI_SVC_PORT"
put_upstream 6  web-bff        "$WEB_BFF_HOST"        "$WEB_BFF_PORT"

# ---- Step 3: 全局插件链（每个 route 共享）----
log "Step 3/4: defining shared plugins"

# jwt-auth 真正验签（替换 shared jwt_auth.go 的"信任 APISIX"模型）
# limit-count / limit-req 限流（双层：按 IP 计数 + 全局突发）
# api-breaker 下游 5xx > 50% 熔断 30s
# cors 统一 CORS（替代 BFF corsMiddleware）
# prometheus 默认配置（OAP 上报 metrics）
PLUGINS_JSON=$(cat <<EOF
{
  "jwt-auth": {
    "key": "user",
    "secret": "$JWT_SECRET",
    "algorithm": "HS256"
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
    "allow_origins": "http://localhost:3000",
    "allow_methods": "GET,POST,PUT,DELETE,OPTIONS,PATCH",
    "allow_headers": "Content-Type,Authorization,X-User-Id",
    "expose_headers": "X-User-Id",
    "allow_credentials": true,
    "max_age": 600
  },
  "prometheus": {}
}
EOF
)

# ---- Step 4: 创建 route（catch-all 主入口 + 5 健康探针）----
log "Step 4/4: creating routes"

put_route() {
  local id="$1" uri="$2" upstream_id="$3" methods="$4" extra_uri="${5:-}"
  local body
  body=$(cat <<EOF
{
  "uri": "$uri$extra_uri",
  "methods": $methods,
  "upstream_id": $upstream_id,
  "plugins": $PLUGINS_JSON,
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
    die "failed to PUT route $id" 3
  fi
}

# 健康探针（不挂鉴权/限流，monitoring 用）—— 直接放 health 端点
HEALTH_PLUGINS=$(cat <<'EOF'
{
  "prometheus": {}
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
AUTH_WHITELIST_PLUGINS=$(cat <<EOF
{
  "limit-count": {"count": 60, "time_window": 60, "key": "remote_addr", "policy": "local"},
  "cors": {"allow_origins": ["http://localhost:3000"], "allow_methods": ["GET","POST","PUT","DELETE","OPTIONS"], "allow_credentials": true, "allow_headers": ["*"]}
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
