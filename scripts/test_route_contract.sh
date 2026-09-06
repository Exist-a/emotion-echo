#!/usr/bin/env bash
# scripts/test_route_contract.sh — Sprint 1 PR-3 三方路由契约脚本
#
# 用途：离线静态比对 APISIX (deploy/apisix/seed.sh) ↔ BFF (emotion-echo-web-bff) ↔ 前端
#       (Emotion-Echo-Web/app/lib/apiRoutes.ts) 三方路径集合，防止三方漂移未被发现。
#
# 三方关系：
#   - APISIX 是网关（catch-all /api/v1/* 路由到 web-bff + 5 auth 白名单）
#   - web-bff 是聚合层（27 主路径 + EmotionQ 3 条件）
#   - 前端 API_ROUTES 是 API 客户端单点真理（26 主路径 + 5 knownOrphans）
#
# 契约断言（每条 fail → exit 1）：
#   1. 每个 BFF 注册路径 ⊆ APISIX 覆盖集（被 catch-all 覆盖 或 在 5 auth 白名单内）
#   2. 每个前端 API_ROUTES 路径 ⊆ BFF 注册集（除 knownOrphans）
#   3. 反向：BFF/APISIX 注册但前端无调用 → warning（不死代码，不 fail）
#
# 退出码：
#   0 = 全契约 PASS
#   1 = 至少一项 fail
#   2 = grep 解析失败
#
# 用法：bash scripts/test_route_contract.sh
#
# 调研依据：
#   - deploy/apisix/seed.sh:417,456-478 路由清单
#   - emotion-echo-web-bff/main.go:214-246 registerRoutes（PR-1 测试已锁 27 条）
#   - Emotion-Echo-Web/app/lib/apiRoutes.ts（PR-2 新建，含 26 + 5 orphan）

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEED_SH="$REPO_ROOT/deploy/apisix/seed.sh"
BFF_MAIN="$REPO_ROOT/emotion-echo-web-bff/main.go"
BFF_HANDLERS="$REPO_ROOT/emotion-echo-web-bff/internal/handler"
WEB_API_ROUTES="$REPO_ROOT/Emotion-Echo-Web/app/lib/apiRoutes.ts"

fail_count=0
warn_count=0

log()  { echo "[route-contract] $*"; }
err()  { echo "[route-contract] FAIL: $*" >&2; fail_count=$((fail_count + 1)); }
warn() { echo "[route-contract] WARN: $*" >&2; warn_count=$((warn_count + 1)); }

# ---------- 前置：三方文件必须存在 ----------
for f in "$SEED_SH" "$BFF_MAIN" "$BFF_HANDLERS"/*.go "$WEB_API_ROUTES"; do
  if [ ! -f "$f" ]; then
    err "missing source: $f"
  fi
done
[ "$fail_count" -gt 0 ] && exit 1

# ---------- 解析 1: APISIX seed.sh 的 URI ----------
# seed.sh 用函数调用 put_route <id> "<uri>" ... 与 put_auth_route <id> "<uri>"
# 而非直接 JSON 字面量（变量引用）。抓这些调用的第 2 个参数（uri）。
log "extracting APISIX URI set from $SEED_SH"

APISIX_URIS_FILE="$(mktemp)"
python - "$SEED_SH" >"$APISIX_URIS_FILE" <<'PY'
import sys, re
src = open(sys.argv[1], encoding="utf-8").read()
uris = set()

# put_route <id> "<uri>" ['<methods>']    (line 438)
# put_route_health <id> "<uri>"           (line 391)
# put_auth_route <id> "<uri>"             (line 477-481)
# put_route <id> "<uri>" '<methods>'      (与上同)
for m in re.finditer(r'\bput_(?:route|auth_route|route_health)\s+\d+\s+"([^"]+)"', src):
    u = m.group(1)
    # 跳过 /user-health /chat-health 等容器自健康（不含 /api/v1）
    if u.startswith("/api/v1"):
        uris.add(u)

# catch-all 通配符（手工声明）
uris.add("/api/v1/*")

# health 自检（不在契约里）
for u in sorted(uris):
    print(u)
PY
[ ! -s "$APISIX_URIS_FILE" ] && { err "APISIX URI grep 返回空"; rm -f "$APISIX_URIS_FILE"; exit 2; }

# ---------- 解析 2: BFF handler 注册的路由 ----------
# main.go 直接 r.GET/POST + handler.Register() 两种
log "extracting BFF route set from $BFF_MAIN + $BFF_HANDLERS"
BFF_ROUTES_FILE="$(mktemp)"
python - "$BFF_MAIN" "$BFF_HANDLERS" >"$BFF_ROUTES_FILE" <<'PY'
import sys, re, os, glob

paths = set()  # (method, path)

def harvest(path):
    src = open(path, encoding="utf-8").read()
    # r.GET("/api/v1/xxx", ...)
    for m in re.finditer(r'\br\.(GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]+)"', src):
        paths.add((m.group(1), m.group(2)))
    # gin.Router / gin.IRouter 上 .GET 也匹配上面（regex 用 \b 兼容）
    # 显式方法注册：r.Handle("GET", ...) 也一并

main = sys.argv[1]
harvest(main)

for f in sorted(glob.glob(os.path.join(sys.argv[2], "*.go"))):
    harvest(f)

for method, path in sorted(paths):
    print(f"{method}\t{path}")
PY
[ ! -s "$BFF_ROUTES_FILE" ] && { err "BFF route grep 返回空"; rm -f "$BFF_ROUTES_FILE" "$APISIX_URIS_FILE"; exit 2; }

# ---------- 解析 3: 前端 API_ROUTES ----------
log "extracting frontend route set from $WEB_API_ROUTES"
WEB_ROUTES_FILE="$(mktemp)"
python - "$WEB_API_ROUTES" >"$WEB_ROUTES_FILE" <<'PY'
import sys, re
src = open(sys.argv[1], encoding="utf-8").read()
# 匹配 path: '/xxx' 或 path: "/xxx" 行
paths = []
for m in re.finditer(r"path:\s*['\"]([^'\"]+)['\"]", src):
    paths.append(m.group(1))
# 去重并保留 knownOrphans 标记（注释里 "Orphan" 不会匹配 path:）
for p in paths:
    print(p)
PY
[ ! -s "$WEB_ROUTES_FILE" ] && { err "前端 API_ROUTES grep 返回空"; rm -f "$BFF_ROUTES_FILE" "$APISIX_URIS_FILE"; exit 2; }

# ---------- 加载到关联数组 ----------
declare -A APISIX_URIS  # key = URI 字符串
# Git Bash heredoc python 输出含 \r，剥 CR（与 BFF 文件处理一致）
APISIX_URIS_FILE_CLEAN="$(mktemp)"
tr -d '\r' < "$APISIX_URIS_FILE" > "$APISIX_URIS_FILE_CLEAN"
rm -f "$APISIX_URIS_FILE"
APISIX_URIS_FILE="$APISIX_URIS_FILE_CLEAN"
while IFS= read -r uri; do
  APISIX_URIS["$uri"]=1
done < "$APISIX_URIS_FILE"

declare -A BFF_ROUTES  # key = "METHOD path"
# Git Bash + heredoc 临时文件可能带 \r，先剥 CR
BFF_ROUTES_FILE_CLEAN="$(mktemp)"
tr -d '\r' < "$BFF_ROUTES_FILE" > "$BFF_ROUTES_FILE_CLEAN"
rm -f "$BFF_ROUTES_FILE"
BFF_ROUTES_FILE="$BFF_ROUTES_FILE_CLEAN"

# 用 awk 安全切分（避免 Git Bash + heredoc 下 read 行为不可靠）
while IFS= read -r line; do
  method=$(printf '%s' "$line" | awk -F'\t' '{print $1}')
  path=$(printf '%s' "$line" | awk -F'\t' '{print $2}')
  BFF_ROUTES["$method $path"]=1
done < "$BFF_ROUTES_FILE"

declare -A WEB_ROUTES  # key = URI 字符串
# 同上剥 CR
WEB_ROUTES_FILE_CLEAN="$(mktemp)"
tr -d '\r' < "$WEB_ROUTES_FILE" > "$WEB_ROUTES_FILE_CLEAN"
rm -f "$WEB_ROUTES_FILE"
WEB_ROUTES_FILE="$WEB_ROUTES_FILE_CLEAN"
while IFS= read -r path; do
  WEB_ROUTES["$path"]=1
done < "$WEB_ROUTES_FILE"

# ---------- 断言 1: 每个 BFF 注册路径 ⊆ APISIX 覆盖集 ----------
log ""
log "=== contract 1: BFF 路由 ⊆ APISIX 覆盖集 ==="
APISIX_AUTH_WHITELIST="^/api/v1/auth/(login|register|verification-code|refresh|logout)\$"
while IFS=$'\t' read -r method path; do
  # 跳过基础设施路径（不在 /api/v1 下）
  if [ "$path" = "/health" ] || [ "$path" = "/metrics" ]; then
    continue
  fi
  # 检查 (1) 精确在 APISIX URI 列表 (2) 命中 catch-all /api/v1/* (3) auth 白名单
  covered=0
  if [ -n "${APISIX_URIS[$path]:-}" ]; then
    covered=1
  elif [ -n "${APISIX_URIS['/api/v1/*']:-}" ]; then
    # 业务路径必须以 /api/v1/ 起（main.go 直接注册的 /api/v1/auth/:action 等）
    # 用 bash 字符串前缀匹配，避免 glob 在 Git Bash 下不展开的坑
    case "$path" in
      /api/v1/*) covered=1 ;;
    esac
  fi
  # 也兼容 :id 动态参数（Gin trie 与 APISIX trie 等价匹配）
  if [ "$covered" -eq 0 ]; then
    base="${path%:[a-zA-Z]*}"
    if [ -n "${APISIX_URIS[$base]:-}" ]; then
      covered=1
    fi
  fi
  # auth 白名单（精确 regex）
  if [ "$covered" -eq 0 ] && [[ "$path" =~ $APISIX_AUTH_WHITELIST ]]; then
    covered=1
  fi
  if [ "$covered" -eq 0 ]; then
    err "BFF route $method $path NOT covered by APISIX (catch-all /api/v1/* 或 5 auth 白名单)"
  fi
done < "$BFF_ROUTES_FILE"

# ---------- 断言 2: 每个前端 API_ROUTES 路径 ⊆ BFF 注册集 ----------
log ""
log "=== contract 2: 前端 API_ROUTES ⊆ BFF 注册集 (除 knownOrphans) ==="

# knownOrphans 关键词（标记为前缀不检查）
# Sprint 1 PR-3 阶段包含：前端有但 BFF 未实现的孤儿；PR-4 落地后应从 orphan 转正
KNOWN_ORPHAN_PREFIXES=(
  "/upload/image"
  "/upload/video"
  "/upload/file"
  "/face/emotion"
  "/user/avatar"   # PR-4: BFF avatar_handler + MinIO + user-svc avatar_url
  "/voice/upload"  # PR-4: BFF voice_handler + ai-svc multimodal kind=audio
)

is_known_orphan() {
  local p="$1"
  for pref in "${KNOWN_ORPHAN_PREFIXES[@]}"; do
    if [ "$p" = "$pref" ]; then
      return 0
    fi
  done
  return 1
}

# 把 BFF path 转成前缀（去掉 :id / :kind 等）便于前缀匹配
declare -A BFF_PREFIXES  # key = 前缀（去掉 :xxx）
for key in "${!BFF_ROUTES[@]}"; do
  method="${key%% *}"
  path="${key#* }"
  prefix="${path%:[a-zA-Z]*}"
  BFF_PREFIXES["$method $prefix"]=1
done

while IFS= read -r path; do
  is_known_orphan "$path" && continue

  # 前端 path 不带 /api/v1 前缀（useApi 拼 baseUrl 时会加），先归一化
  case "$path" in
    /api/v1/*) normalized_path="$path" ;;
    *)         normalized_path="/api/v1$path" ;;
  esac

  # 精确匹配
  matched=0
  for key in "${!BFF_ROUTES[@]}"; do
    if [ "$key" = "POST $normalized_path" ] || [ "$key" = "GET $normalized_path" ] || \
       [ "$key" = "PUT $normalized_path" ] || [ "$key" = "PATCH $normalized_path" ] || \
       [ "$key" = "DELETE $normalized_path" ]; then
      matched=1
      break
    fi
  done
  # 前缀匹配（:id / :kind / :resultId / :messageId / :action）
  if [ "$matched" -eq 0 ]; then
    for key in "${!BFF_PREFIXES[@]}"; do
      base_path="${key#* }"
      if [[ "$normalized_path" == "$base_path"* ]]; then
        matched=1
        break
      fi
    done
  fi
  if [ "$matched" -eq 0 ]; then
    err "前端 API_ROUTES $path NOT registered in BFF (含 knownOrphans 排除)"
  fi
done < "$WEB_ROUTES_FILE"

# ---------- 反向警告：BFF 注册但前端无调用 ----------
log ""
log "=== contract 3 (warning): BFF 路由未在前端 API_ROUTES ==="
# 只检查 /api/v1/ 业务路径（跳过 /health /metrics）
while IFS=$'\t' read -r method path; do
  [[ "$path" == /health || "$path" == /metrics ]] && continue
  [[ "$path" != /api/v1/* ]] && continue
  # 跳过 catch-all / :action（前端不需要显式调用）
  [[ "$path" == */:action || "$path" == */:id ]] && continue
  # 检查前端有无调用
  found=0
  while IFS= read -r p; do
    if [ "$p" = "$path" ]; then
      found=1
      break
    fi
  done < "$WEB_ROUTES_FILE"
  if [ "$found" -eq 0 ]; then
    warn "BFF route $method $path 未被前端 API_ROUTES 调用（可能为死代码）"
  fi
done < "$BFF_ROUTES_FILE"

# ---------- 清理 ----------
rm -f "$APISIX_URIS_FILE" "$BFF_ROUTES_FILE" "$WEB_ROUTES_FILE"

# ---------- 退出码 ----------
echo ""
echo "============================================="
echo "fail_count=$fail_count  warn_count=$warn_count"
if [ "$fail_count" -eq 0 ]; then
  echo "[OK] 三方路由契约全 PASS"
  exit 0
else
  echo "[FAIL] 三方路由契约失败"
  exit 1
fi